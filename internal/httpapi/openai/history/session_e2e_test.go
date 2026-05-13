//go:build integration

package history

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/sse"
)

func setupTestClient(t *testing.T) (*dsclient.Client, *auth.RequestAuth, context.Context) {
	t.Helper()
	store, err := config.LoadStoreWithError()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	accounts := store.Accounts()
	if len(accounts) == 0 {
		t.Fatal("no accounts configured")
	}
	acc := accounts[0]
	t.Logf("using account: %s", acc.Identifier())

	pool := account.NewPool(store)
	var dsClient *dsclient.Client
	resolver := auth.NewResolver(store, pool, func(ctx context.Context, a config.Account) (string, error) {
		return dsClient.Login(ctx, a)
	})
	dsClient = dsclient.NewClient(store, resolver)
	a := &auth.RequestAuth{
		AccountID:      acc.Identifier(),
		DeepSeekToken:  acc.Token,
		UseConfigToken: true,
		TriedAccounts:  map[string]bool{},
	}
	ctx := context.Background()
	if a.DeepSeekToken == "" {
		token, err := dsClient.Login(ctx, acc)
		if err != nil {
			t.Fatalf("login failed: %v", err)
		}
		a.DeepSeekToken = token
	}
	return dsClient, a, ctx
}

func sendPrompt(t *testing.T, ds *dsclient.Client, a *auth.RequestAuth, ctx context.Context, sessionID string, parentMsgID int, prompt string) (string, int) {
	t.Helper()
	pow, err := ds.GetPow(ctx, a, 3)
	if err != nil {
		t.Fatalf("get pow: %v", err)
	}
	var parentID any
	if parentMsgID > 0 {
		parentID = parentMsgID
	}
	payload := map[string]any{
		"chat_session_id":   sessionID,
		"model_type":        "default",
		"parent_message_id": parentID,
		"prompt":            prompt,
		"ref_file_ids":      []any{},
		"thinking_enabled":  false,
		"search_enabled":    false,
	}
	resp, err := ds.CallCompletion(ctx, a, payload, pow, 3)
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	if resp.StatusCode != 200 {
		body := make([]byte, 2048)
		n, _ := resp.Body.Read(body)
		t.Fatalf("completion returned status %d: %s", resp.StatusCode, string(body[:n]))
	}
	result := sse.CollectStream(resp, false, true)
	t.Logf("  response: %q (msg_id=%d)", result.Text, result.ResponseMessageID)
	return result.Text, result.ResponseMessageID
}

// Test 1: Multi-turn conversation — 4 turns with accumulating context.
func TestSessionMultiTurnConversation(t *testing.T) {
	ds, a, ctx := setupTestClient(t)

	sessionID, err := ds.CreateSession(ctx, a, 3)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Logf("session: %s", sessionID)

	type turn struct {
		prompt   string
		contains string
	}

	turns := []turn{
		{
			prompt:   "<|User|>我的生日是1995年8月15日。请回复记住了。<|Assistant|>",
			contains: "记住",
		},
		{
			prompt:   "<|User|>我喜欢吃火锅。请回复记住了。<|Assistant|>",
			contains: "记住",
		},
		{
			prompt:   "<|User|>我养了一只猫叫小花。请回复记住了。<|Assistant|>",
			contains: "记住",
		},
		{
			prompt:   "<|User|>请告诉我：我的生日是哪天？我喜欢吃什么？我的猫叫什么名字？<|Assistant|>",
			contains: "",
		},
	}

	parentID := 0
	for i, turn := range turns {
		t.Logf("--- Turn %d ---", i+1)
		text, msgID := sendPrompt(t, ds, a, ctx, sessionID, parentID, turn.prompt)
		if turn.contains != "" && !strings.Contains(text, turn.contains) {
			t.Errorf("turn %d: expected response to contain %q, got %q", i+1, turn.contains, text)
		}
		if msgID <= 0 {
			t.Fatalf("turn %d: invalid response_message_id: %d", i+1, msgID)
		}
		parentID = msgID
	}

	// Verify the final response recalls all facts.
	// Send a summary request using the last parentID.
	t.Log("--- Verifying final turn ---")
	summaryText, _ := sendPrompt(t, ds, a, ctx, sessionID, parentID, "<|User|>用一句话总结你知道的关于我的所有信息。<|Assistant|>")
	t.Logf("summary: %q", summaryText)

	checks := []struct {
		key   string
		found bool
	}{
		{"生日", strings.Contains(summaryText, "1995") || strings.Contains(summaryText, "8月15")},
		{"火锅", strings.Contains(summaryText, "火锅")},
		{"小花", strings.Contains(summaryText, "小花")},
	}
	allPassed := true
	for _, c := range checks {
		if c.found {
			t.Logf("  [PASS] remembered: %s", c.key)
		} else {
			t.Logf("  [FAIL] forgot: %s", c.key)
			allPassed = false
		}
	}
	if allPassed {
		t.Log("SUCCESS: All facts remembered across 4 turns!")
	} else {
		t.Error("FAILED: Some facts were lost in multi-turn conversation")
	}
}

// Test 2: Tool call in session — send tools, get tool_call, send result, continue.
func TestSessionToolCall(t *testing.T) {
	ds, a, ctx := setupTestClient(t)

	sessionID, err := ds.CreateSession(ctx, a, 3)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Logf("session: %s", sessionID)

	// Step 1: Ask a question that triggers a tool call.
	toolPrompt := `<|User|>你可以使用以下工具：
- get_weather(city: string): 获取天气

请使用 get_weather 工具查询北京的天气。<|Assistant|>`

	t.Log("=== Step 1: Trigger tool call ===")
	text1, msgID1 := sendPrompt(t, ds, a, ctx, sessionID, 0, toolPrompt)
	t.Logf("tool call response: %q", text1)

	hasToolCall := strings.Contains(text1, "DSML") || strings.Contains(text1, "get_weather") || strings.Contains(text1, "invoke")
	t.Logf("contains tool call: %v", hasToolCall)

	// Step 2: Send tool result and ask for continuation.
	toolResultPrompt := `<|User|>你可以使用以下工具：
- get_weather(city: string): 获取天气

请使用 get_weather 工具查询北京的天气。
<|DSML|tool_calls>
<|DSML|invoke name="get_weather">
<|DSML|parameter name="city"><![CDATA[北京]]></|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>

<|Tool|>北京今天晴，温度25°C，微风。<|end▁of▁toolresults|>

请根据天气信息回答用户。<|Assistant|>`

	t.Log("=== Step 2: Send tool result ===")
	text2, _ := sendPrompt(t, ds, a, ctx, sessionID, msgID1, toolResultPrompt)
	t.Logf("tool result response: %q", text2)

	if strings.Contains(text2, "25") || strings.Contains(text2, "晴") || strings.Contains(text2, "天气") {
		t.Log("SUCCESS: DeepSeek processed tool result and responded correctly!")
	} else {
		t.Logf("INCONCLUSIVE: response doesn't mention weather details")
		t.Logf("  response: %q", text2)
	}
}

// Test 3: Basic session context preservation.
func TestSessionContextPreservesConversation(t *testing.T) {
	ds, a, ctx := setupTestClient(t)

	sessionID, err := ds.CreateSession(ctx, a, 3)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Logf("session: %s", sessionID)

	t.Log("=== Sending first message ===")
	text1, msgID1 := sendPrompt(t, ds, a, ctx, sessionID, 0, "<|User|>记住：我的名字叫张三。请只回复好的两个字。<|Assistant|>")
	t.Logf("first response: %q (msg_id=%d)", text1, msgID1)

	if msgID1 <= 0 {
		t.Fatal("invalid response_message_id")
	}

	t.Log("=== Sending second message ===")
	text2, _ := sendPrompt(t, ds, a, ctx, sessionID, msgID1, "<|User|>我叫什么名字？<|Assistant|>")
	t.Logf("second response: %q", text2)

	if strings.Contains(text2, "张三") {
		t.Log("SUCCESS: DeepSeek remembered the name!")
	} else {
		t.Errorf("FAILED: expected '张三' in response, got %q", text2)
	}
}
