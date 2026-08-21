package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSPAFileServer checks the two branches that matter: a real static
// asset is served as-is, and a client-side route (e.g. the diner table
// link /{slug}/t/{token}, which has no matching file) falls back to
// index.html so app.js can parse the path itself.
func TestSPAFileServer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("shell"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("js"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := spaFileServer(dir)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if got := rec.Body.String(); got != "js" {
		t.Errorf("GET /app.js: got body %q, want %q (existing file should be served as-is)", got, "js")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/demo-bistro/t/abc123", nil))
	if got := rec.Body.String(); got != "shell" {
		t.Errorf("GET /demo-bistro/t/abc123: got body %q, want %q (unknown path should fall back to index.html)", got, "shell")
	}
}
