package history

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type currentInputCacheTestStore struct{}

func (currentInputCacheTestStore) CurrentInputFileEnabled() bool { return true }
func (currentInputCacheTestStore) CurrentInputFileMinChars() int { return 0 }
func (currentInputCacheTestStore) ContextEngineMode() string     { return "off" }

type currentInputCacheTestDS struct {
	uploads []dsclient.UploadFileRequest
}

func (d *currentInputCacheTestDS) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	d.uploads = append(d.uploads, req)
	return &dsclient.UploadFileResult{ID: fmt.Sprintf("file-%d", len(d.uploads))}, nil
}

func TestApplyCurrentInputFileCachesToolsFileByStableHash(t *testing.T) {
	ResetCurrentInputToolsFileCacheForTesting()
	ds := &currentInputCacheTestDS{}
	svc := Service{Store: currentInputCacheTestStore{}, DS: ds}
	authReq := &auth.RequestAuth{CallerID: "caller-1", DeepSeekToken: "token"}
	stdReq := cacheTestStandardRequest()

	first, err := svc.ApplyCurrentInputFile(context.Background(), authReq, stdReq)
	if err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	second, err := svc.ApplyCurrentInputFile(context.Background(), authReq, stdReq)
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}

	if len(ds.uploads) != 3 {
		t.Fatalf("expected two history uploads and one cached tools upload, got %d", len(ds.uploads))
	}
	if ds.uploads[0].Filename != "DS2API_HISTORY.txt" || ds.uploads[1].Filename != "DS2API_TOOLS.txt" || ds.uploads[2].Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("unexpected upload sequence: %#v", ds.uploads)
	}
	if first.CurrentToolsFileCacheHit {
		t.Fatalf("first request should miss tools file cache")
	}
	if !second.CurrentToolsFileCacheHit {
		t.Fatalf("second request should hit tools file cache")
	}
	if first.CurrentToolsHash == "" || first.CurrentToolsHash != second.CurrentToolsHash {
		t.Fatalf("expected stable tools hash, got first=%q second=%q", first.CurrentToolsHash, second.CurrentToolsHash)
	}
	if first.CurrentInputHash == "" || first.CurrentInputHash != second.CurrentInputHash {
		t.Fatalf("expected stable history hash, got first=%q second=%q", first.CurrentInputHash, second.CurrentInputHash)
	}
	if first.CurrentPromptHash == "" || first.CurrentPromptHash != second.CurrentPromptHash {
		t.Fatalf("expected stable prompt hash, got first=%q second=%q", first.CurrentPromptHash, second.CurrentPromptHash)
	}
	if len(second.RefFileIDs) < 2 || second.RefFileIDs[0] != "file-3" || second.RefFileIDs[1] != "file-2" {
		t.Fatalf("expected second request to reuse cached tools file after fresh history, got %#v", second.RefFileIDs)
	}
}

func TestApplyCurrentInputFileToolsCacheIsAccountScoped(t *testing.T) {
	ResetCurrentInputToolsFileCacheForTesting()
	ds := &currentInputCacheTestDS{}
	svc := Service{Store: currentInputCacheTestStore{}, DS: ds}
	stdReq := cacheTestStandardRequest()

	first, err := svc.ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{UseConfigToken: true, AccountID: "acc1@test.com"}, stdReq)
	if err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	second, err := svc.ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{UseConfigToken: true, AccountID: "acc2@test.com"}, stdReq)
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	third, err := svc.ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{UseConfigToken: true, AccountID: "acc1@test.com"}, stdReq)
	if err != nil {
		t.Fatalf("third apply failed: %v", err)
	}

	if first.CurrentToolsFileCacheHit || second.CurrentToolsFileCacheHit {
		t.Fatalf("first use per account should miss cache: first=%v second=%v", first.CurrentToolsFileCacheHit, second.CurrentToolsFileCacheHit)
	}
	if !third.CurrentToolsFileCacheHit {
		t.Fatalf("same account and same tools content should hit cache")
	}
	if len(ds.uploads) != 5 {
		t.Fatalf("expected 3 history uploads and 2 account-scoped tools uploads, got %d", len(ds.uploads))
	}
	if len(third.RefFileIDs) < 2 || third.RefFileIDs[1] != first.CurrentToolsFileID {
		t.Fatalf("expected third request to reuse acc1 tools file id %q, got %#v", first.CurrentToolsFileID, third.RefFileIDs)
	}
	if second.CurrentToolsFileID == first.CurrentToolsFileID {
		t.Fatalf("different accounts must not share cached tools file id")
	}
}

func TestApplyCurrentInputFileGoldenTranscriptStability(t *testing.T) {
	ResetCurrentInputToolsFileCacheForTesting()
	ds := &currentInputCacheTestDS{}
	svc := Service{Store: currentInputCacheTestStore{}, DS: ds}
	authReq := &auth.RequestAuth{CallerID: "caller-golden", DeepSeekToken: "token"}

	out, err := svc.ApplyCurrentInputFile(context.Background(), authReq, cacheTestStandardRequest())
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(ds.uploads) != 2 {
		t.Fatalf("expected history and tools uploads, got %d", len(ds.uploads))
	}
	const wantHistory = "# DS2API_HISTORY.txt\nPrior conversation history and tool progress.\n\n=== 1. SYSTEM ===\nsystem rule\n\n=== 2. USER ===\nsearch docs\n"
	if got := string(ds.uploads[0].Data); got != wantHistory {
		t.Fatalf("history transcript mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantHistory)
	}
	const wantTools = "# DS2API_TOOLS.txt\nAvailable tool descriptions and parameter schemas for this request.\n\nYou have access to these tools:\n\nTool: search\nDescription: Search docs\nParameters: {\"properties\":{\"query\":{\"type\":\"string\"}},\"type\":\"object\"}\n"
	if got := string(ds.uploads[1].Data); got != wantTools {
		t.Fatalf("tools transcript mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantTools)
	}
	if !strings.HasPrefix(out.PromptTokenText, wantHistory+"\n"+wantTools+"\n") {
		t.Fatalf("prompt token text should start with history, tools, prompt in order, got %q", out.PromptTokenText)
	}
	if !strings.Contains(out.FinalPrompt, "DS2API_TOOLS.txt") || !strings.Contains(out.FinalPrompt, "TOOL CALL FORMAT") {
		t.Fatalf("final prompt should reference tools file and retain format instructions, got %q", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, "Description: Search docs") {
		t.Fatalf("final prompt should not inline tool descriptions, got %q", out.FinalPrompt)
	}
}

func cacheTestStandardRequest() promptcompat.StandardRequest {
	return promptcompat.StandardRequest{
		RequestedModel: "deepseek-v4-flash",
		ResolvedModel:  "deepseek-v4-flash",
		ResponseModel:  "deepseek-v4-flash",
		Messages: []any{
			map[string]any{"role": "system", "content": "system rule"},
			map[string]any{"role": "user", "content": "search docs"},
		},
		ToolsRaw: []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": "Search docs",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}
