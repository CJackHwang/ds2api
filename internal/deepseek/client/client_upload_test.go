package client

import (
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	powpkg "ds2api/pow"
)

func TestBuildUploadMultipartBodyOmitsPurposeAndIncludesFilePart(t *testing.T) {
	body, contentType, err := buildUploadMultipartBody(`../demo.txt`, "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("buildUploadMultipartBody error: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	payload := string(body)
	if strings.Contains(payload, `name="purpose"`) || strings.Contains(payload, "assistants") {
		t.Fatalf("expected purpose to be omitted from payload: %q", payload)
	}
	if !strings.Contains(payload, `name="file"; filename="demo.txt"`) {
		t.Fatalf("expected sanitized filename in payload: %q", payload)
	}
	if !strings.Contains(payload, "Content-Type: text/plain") {
		t.Fatalf("expected file content type in payload: %q", payload)
	}
	if !strings.Contains(payload, "hello") {
		t.Fatalf("expected file content in payload: %q", payload)
	}
}

func TestDoUploadDoesNotFallbackForNonIdempotentUpload(t *testing.T) {
	var fallbackCalled bool
	client := &Client{}
	_, err := client.doUpload(
		context.Background(),
		doerFunc(func(req *http.Request) (*http.Response, error) {
			_, _ = io.ReadAll(req.Body)
			return nil, errors.New("ambiguous upload write failure")
		}),
		doerFunc(func(*http.Request) (*http.Response, error) {
			fallbackCalled = true
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}),
		dsprotocol.DeepSeekUploadFileURL,
		map[string]string{"Content-Type": "multipart/form-data"},
		[]byte("body"),
	)
	if err == nil {
		t.Fatal("expected upload error")
	}
	if fallbackCalled {
		t.Fatal("upload fallback should not be called for a non-idempotent request")
	}
}

func TestExtractUploadFileResultSupportsNestedShapes(t *testing.T) {
	got := extractUploadFileResult(map[string]any{
		"data": map[string]any{
			"biz_data": map[string]any{
				"file": map[string]any{
					"file_id":   "file_123",
					"file_name": "report.pdf",
					"file_size": 99,
					"status":    "processed",
					"purpose":   "assistants",
					"is_image":  true,
				},
			},
		},
	})
	if got.ID != "file_123" {
		t.Fatalf("expected id file_123, got %#v", got)
	}
	if got.Filename != "report.pdf" {
		t.Fatalf("expected filename report.pdf, got %#v", got)
	}
	if got.Bytes != 99 {
		t.Fatalf("expected bytes 99, got %#v", got)
	}
	if got.Status != "processed" {
		t.Fatalf("expected status processed, got %#v", got)
	}
	if got.Purpose != "assistants" {
		t.Fatalf("expected purpose assistants, got %#v", got)
	}
	if !got.IsImage {
		t.Fatalf("expected image flag true, got %#v", got)
	}
}

func TestUploadFileUsesUploadTargetPowAndMultipartHeaders(t *testing.T) {
	challengeHash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
	powResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"challenge":{"algorithm":"DeepSeekHashV1","challenge":"` + hex.EncodeToString(challengeHash[:]) + `","salt":"salt","expire_at":1712345678,"difficulty":1000,"signature":"sig","target_path":"` + dsprotocol.DeepSeekUploadTargetPath + `"}}}}`
	uploadResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"file":{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"processed","purpose":"assistants","is_image":false}}}}`
	var seenPow string
	var seenTargetPath string
	var seenContentType string
	var seenFileSize string
	var seenModelType string
	var seenBody string
	call := 0
	client := &Client{
		regular: doerFunc(func(req *http.Request) (*http.Response, error) {
			call++
			bodyBytes, _ := io.ReadAll(req.Body)
			switch call {
			case 1:
				seenTargetPath = string(bodyBytes)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(powResponse)), Request: req}, nil
			case 2:
				seenPow = req.Header.Get("x-ds-pow-response")
				seenContentType = req.Header.Get("Content-Type")
				seenFileSize = req.Header.Get("x-file-size")
				seenModelType = req.Header.Get("x-model-type")
				seenBody = string(bodyBytes)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(uploadResponse)), Request: req}, nil
			default:
				t.Fatalf("unexpected request count %d", call)
				return nil, nil
			}
		}),
		fallback: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, nil
		})},
		maxRetries: 1,
	}
	result, err := client.UploadFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token", TriedAccounts: map[string]bool{}}, UploadFileRequest{
		Filename:    "demo.txt",
		ContentType: "text/plain",
		Purpose:     "assistants",
		ModelType:   "vision",
		Data:        []byte("hello"),
	}, 1)
	if err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}
	if result.ID != "file_789" {
		t.Fatalf("expected uploaded file id file_789, got %#v", result)
	}
	if !strings.Contains(seenTargetPath, `"target_path":"`+dsprotocol.DeepSeekUploadTargetPath+`"`) {
		t.Fatalf("expected upload target_path in pow request, got %q", seenTargetPath)
	}
	if strings.TrimSpace(seenPow) == "" {
		t.Fatal("expected x-ds-pow-response header")
	}
	rawPow, err := base64.StdEncoding.DecodeString(seenPow)
	if err != nil {
		t.Fatalf("decode pow header failed: %v", err)
	}
	var powHeader map[string]any
	if err := json.Unmarshal(rawPow, &powHeader); err != nil {
		t.Fatalf("unmarshal pow header failed: %v", err)
	}
	if powHeader["target_path"] != dsprotocol.DeepSeekUploadTargetPath {
		t.Fatalf("expected pow target_path %q, got %#v", dsprotocol.DeepSeekUploadTargetPath, powHeader["target_path"])
	}
	if seenFileSize != "5" {
		t.Fatalf("expected x-file-size=5, got %q", seenFileSize)
	}
	if seenModelType != "vision" {
		t.Fatalf("expected x-model-type=vision, got %q", seenModelType)
	}
	if !strings.HasPrefix(seenContentType, "multipart/form-data; boundary=") {
		t.Fatalf("expected multipart content type, got %q", seenContentType)
	}
	if !strings.Contains(seenBody, `name="file"; filename="demo.txt"`) {
		t.Fatalf("expected file part in upload body: %q", seenBody)
	}
}

func TestUploadFileWaitsForProcessedFetchFiles(t *testing.T) {
	oldSleep := fileReadySleep
	fileReadySleep = func(time.Duration) {}
	defer func() { fileReadySleep = oldSleep }()

	challengeHash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
	powResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"challenge":{"algorithm":"DeepSeekHashV1","challenge":"` + hex.EncodeToString(challengeHash[:]) + `","salt":"salt","expire_at":1712345678,"difficulty":1000,"signature":"sig","target_path":"` + dsprotocol.DeepSeekUploadTargetPath + `"}}}}`
	uploadResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"file":{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"PENDING","purpose":"assistants","is_image":false}}}}`
	pendingFetchResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"files":[{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"PENDING","purpose":"assistants","is_image":false}]}}}`
	processedFetchResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"files":[{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"processed","purpose":"assistants","is_image":true}]}}}`

	var call int
	client := &Client{
		regular: doerFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				bodyBytes, _ := io.ReadAll(req.Body)
				if !strings.Contains(string(bodyBytes), `"target_path":"`+dsprotocol.DeepSeekUploadTargetPath+`"`) {
					t.Fatalf("expected pow target path request, got %s", string(bodyBytes))
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(powResponse)), Request: req}, nil
			case 2:
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(uploadResponse)), Request: req}, nil
			case 3, 4:
				if req.Method != http.MethodGet {
					t.Fatalf("expected GET fetch request, got %s", req.Method)
				}
				if req.URL.Path != "/api/v0/file/fetch_files" {
					t.Fatalf("expected fetch files path /api/v0/file/fetch_files, got %q", req.URL.Path)
				}
				if got := req.URL.Query().Get("file_ids"); got != "file_789" {
					t.Fatalf("expected file_ids=file_789, got %q", got)
				}
				respBody := pendingFetchResponse
				if call == 4 {
					respBody = processedFetchResponse
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(respBody)), Request: req}, nil
			default:
				t.Fatalf("unexpected request count %d", call)
				return nil, nil
			}
		}),
		fallback:   &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) { return nil, nil })},
		maxRetries: 1,
	}

	result, err := client.UploadFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token", TriedAccounts: map[string]bool{}}, UploadFileRequest{
		Filename:    "demo.txt",
		ContentType: "text/plain",
		Purpose:     "assistants",
		Data:        []byte("hello"),
	}, 1)
	if err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}
	if result.ID != "file_789" {
		t.Fatalf("expected uploaded file id file_789, got %#v", result)
	}
	if result.Status != "processed" {
		t.Fatalf("expected final status processed, got %#v", result.Status)
	}
	if call != 4 {
		t.Fatalf("expected 4 requests, got %d", call)
	}
}
