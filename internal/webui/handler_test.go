package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeFromDiskPinsContentType ensures static admin assets are returned
// with an explicit, RFC-compliant Content-Type that does not depend on
// mime.TypeByExtension. On Windows mime.TypeByExtension consults the registry
// (HKEY_CLASSES_ROOT) which third-party software can corrupt — for example
// installing certain editors rewrites .css to application/xml — and Chrome
// then refuses to apply a stylesheet whose Content-Type is not text/css,
// breaking the /admin page entirely. Pinning the type by file extension makes
// the response deterministic across operating systems and machine state.
func TestServeFromDiskPinsContentType(t *testing.T) {
	staticDir := t.TempDir()
	assetsDir := filepath.Join(staticDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}

	files := map[string]string{
		"index.html":           "<!doctype html><html></html>",
		"assets/index.css":     "body{}",
		"assets/index.js":      "console.log(1)",
		"assets/icon.svg":      `<svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		"assets/source.js.map": `{"version":3}`,
	}
	for rel, body := range files {
		full := filepath.Join(staticDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	h := &Handler{StaticDir: staticDir}

	cases := []struct {
		urlPath      string
		wantPrefix   string
		wantCacheCtl string
	}{
		{"/admin/assets/index.css", "text/css", "public, max-age=31536000, immutable"},
		{"/admin/assets/index.js", "text/javascript", "public, max-age=31536000, immutable"},
		{"/admin/assets/icon.svg", "image/svg+xml", "public, max-age=31536000, immutable"},
		{"/admin/assets/source.js.map", "application/json", "public, max-age=31536000, immutable"},
		// "/admin/index.html" is intentionally omitted: http.ServeFile redirects
		// requests for index.html to "./", matching Go's net/http behavior. The
		// route the SPA actually lands on is "/admin/" below.
		{"/admin/", "text/html", "no-store, must-revalidate"},
	}

	for _, tc := range cases {
		t.Run(tc.urlPath, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.urlPath, nil)
			rec := httptest.NewRecorder()
			h.serveFromDisk(rec, req, staticDir)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, tc.wantPrefix) {
				t.Fatalf("Content-Type = %q, want prefix %q", ct, tc.wantPrefix)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCacheCtl {
				t.Fatalf("Cache-Control = %q, want %q", got, tc.wantCacheCtl)
			}
		})
	}
}

func TestServeFromDiskRejectsSiblingDirectoryWithSharedPrefix(t *testing.T) {
	parent := t.TempDir()
	staticDir := filepath.Join(parent, "admin")
	siblingDir := filepath.Join(parent, "admin-leak")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("mkdir static dir: %v", err)
	}
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatalf("mkdir sibling dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(siblingDir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write sibling secret: %v", err)
	}

	h := &Handler{StaticDir: staticDir}
	req := httptest.NewRequest(http.MethodGet, "/admin/../admin-leak/secret.txt", nil)
	rec := httptest.NewRecorder()
	h.serveFromDisk(rec, req, staticDir)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "secret") {
		t.Fatal("served content from sibling directory")
	}
}

func TestIsPathInsideRootAllowsFilesystemRootChildren(t *testing.T) {
	root := filepath.VolumeName(os.TempDir()) + string(os.PathSeparator)
	child := filepath.Join(root, "assets", "index.css")

	if !isPathInsideRoot(child, root) {
		t.Fatalf("expected filesystem-root child %q inside %q", child, root)
	}
}

func TestIsPathInsideRootRejectsSharedPrefixSibling(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "admin")
	sibling := filepath.Join(parent, "admin-leak", "secret.txt")

	if isPathInsideRoot(sibling, root) {
		t.Fatalf("expected shared-prefix sibling %q outside %q", sibling, root)
	}
}

// TestSetStaticContentTypeUnknownExtensionFallsThrough verifies that unknown
// extensions leave the Content-Type header unset, so http.ServeFile can apply
// its own detection (sniffing or mime.TypeByExtension) for cases the pinned
// table does not cover.
func TestSetStaticContentTypeUnknownExtensionFallsThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	setStaticContentType(rec, "/tmp/data.unknownext")
	if got := rec.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty for unknown extension", got)
	}
}

// TestSetStaticContentTypeIsCaseInsensitive guards against a regression where
// uppercase extensions (e.g. STYLE.CSS shipped from some build pipelines)
// would bypass the pinned table and fall back to the registry on Windows.
func TestSetStaticContentTypeIsCaseInsensitive(t *testing.T) {
	rec := httptest.NewRecorder()
	setStaticContentType(rec, "/tmp/STYLE.CSS")
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("Content-Type = %q, want text/css prefix", got)
	}
}
