package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListAccountsPageSizeCapIs5000(t *testing.T) {
	accounts := make([]string, 0, 150)
	for i := range 150 {
		accounts = append(accounts, fmt.Sprintf(`{"email":"u%d@example.com","password":"pwd"}`, i))
	}
	raw := fmt.Sprintf(`{"accounts":[%s]}`, strings.Join(accounts, ","))
	router := newHTTPAdminHarness(t, raw, &testingDSMock{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts?page=1&page_size=200", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, _ := payload["items"].([]any)
	if len(items) != 150 {
		t.Fatalf("expected all 150 accounts with page_size=200, got %d", len(items))
	}
	if ps, _ := payload["page_size"].(float64); ps != 200 {
		t.Fatalf("expected page_size=200 in response, got %v", payload["page_size"])
	}
}

func TestListAccountsPageSizeAbove5000ClampedTo5000(t *testing.T) {
	router := newHTTPAdminHarness(t, `{"accounts":[{"email":"u@example.com","password":"pwd"}]}`, &testingDSMock{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts?page=1&page_size=9999", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ps, _ := payload["page_size"].(float64); ps != 5000 {
		t.Fatalf("expected page_size clamped to 5000, got %v", payload["page_size"])
	}
}

func TestAddAccountValidatesBeforePersist(t *testing.T) {
	ds := &testingDSMock{}
	router := newHTTPAdminHarness(t, `{"accounts":[]}`, ds)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts", []byte(`{"email":"new@example.com","password":"pwd"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ok, _ := payload["success"].(bool); !ok {
		t.Fatalf("expected success response, got %#v", payload)
	}
	account, _ := payload["account"].(map[string]any)
	if identifier, _ := account["identifier"].(string); identifier != "new@example.com" {
		t.Fatalf("unexpected account payload: %#v", account)
	}
	if testStatus, _ := account["test_status"].(string); testStatus != "ok" {
		t.Fatalf("expected validated account status ok, got %#v", account)
	}
	if ds.loginCalls != 1 || ds.createSessionCalls != 1 {
		t.Fatalf("expected validation login/create session once, got login=%d createSession=%d", ds.loginCalls, ds.createSessionCalls)
	}
}

func TestAddAccountRejectsUnavailableCredentials(t *testing.T) {
	ds := &testingDSMock{loginError: fmt.Errorf("invalid credentials")}
	router := newHTTPAdminHarness(t, `{"accounts":[]}`, ds)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts", []byte(`{"email":"bad@example.com","password":"pwd"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if ds.createSessionCalls != 0 {
		t.Fatalf("expected create session to be skipped when login fails, got %d", ds.createSessionCalls)
	}
	if !strings.Contains(rec.Body.String(), "account login failed") {
		t.Fatalf("expected validation error in response, got %s", rec.Body.String())
	}

	readRec := httptest.NewRecorder()
	router.ServeHTTP(readRec, adminReq(http.MethodGet, "/accounts?page=1&page_size=10", nil))
	if readRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", readRec.Code, readRec.Body.String())
	}
	var listPayload map[string]any
	if err := json.Unmarshal(readRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	items, _ := listPayload["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected account not to be persisted after failed validation, got %#v", items)
	}
}
