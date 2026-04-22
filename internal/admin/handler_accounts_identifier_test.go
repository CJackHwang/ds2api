package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

func newAdminTestHandler(t *testing.T, raw string) *Handler {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", raw)
	store := config.LoadStore()
	return &Handler{
		Store: store,
		Pool:  account.NewPool(store),
		DS:    &testingDSMock{},
	}
}

func TestListAccountsUsesEmailIdentifier(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"u@example.com","password":"pwd"}]
	}`)

	req := httptest.NewRequest(http.MethodGet, "/admin/accounts?page=1&page_size=10", nil)
	rec := httptest.NewRecorder()
	h.listAccounts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	items, _ := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	identifier, _ := first["identifier"].(string)
	if identifier != "u@example.com" {
		t.Fatalf("expected email identifier, got %q", identifier)
	}
}

func TestDeleteAccountSupportsMobileAlias(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"u@example.com","mobile":"13800138000","password":"pwd"}]
	}`)

	r := chi.NewRouter()
	r.Delete("/admin/accounts/{identifier}", h.deleteAccount)
	req := httptest.NewRequest(http.MethodDelete, "/admin/accounts/13800138000", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := len(h.Store.Accounts()); got != 0 {
		t.Fatalf("expected account removed, remaining=%d", got)
	}
}

func TestDeleteAccountSupportsEncodedPlusMobile(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"mobile":"+8613800138000","password":"pwd"}]
	}`)

	r := chi.NewRouter()
	r.Delete("/admin/accounts/{identifier}", h.deleteAccount)
	req := httptest.NewRequest(http.MethodDelete, "/admin/accounts/"+url.PathEscape("+8613800138000"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := len(h.Store.Accounts()); got != 0 {
		t.Fatalf("expected account removed, remaining=%d", got)
	}
}

func TestAddAccountRejectsCanonicalMobileDuplicate(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"mobile":"+8613800138000","password":"pwd"}]
	}`)

	r := chi.NewRouter()
	r.Post("/admin/accounts", h.addAccount)
	body := []byte(`{"mobile":"13800138000","password":"pwd2"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/accounts", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := len(h.Store.Accounts()); got != 1 {
		t.Fatalf("expected no duplicate insert, got=%d", got)
	}
}

func TestFindAccountByIdentifierSupportsMobile(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[
			{"email":"u@example.com","mobile":"13800138000","password":"pwd"}
		]
	}`)

	accByMobile, ok := findAccountByIdentifier(h.Store, "13800138000")
	if !ok {
		t.Fatal("expected find by mobile")
	}
	if accByMobile.Email != "u@example.com" {
		t.Fatalf("unexpected account by mobile: %#v", accByMobile)
	}
	accByMobileWithCountryCode, ok := findAccountByIdentifier(h.Store, "+8613800138000")
	if !ok {
		t.Fatal("expected find by +86 mobile")
	}
	if accByMobileWithCountryCode.Email != "u@example.com" {
		t.Fatalf("unexpected account by +86 mobile: %#v", accByMobileWithCountryCode)
	}

}
