package localgraph

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalMuxRootRedirectsToWebViewer(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3717/local", true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "http://127.0.0.1:3717/local" {
		t.Fatalf("Location = %q, want local viewer", got)
	}
}

func TestLocalMuxRootReportsAPIWhenWebViewerDisabled(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "", false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLocalMuxServesEmbeddedViewer(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3717/local", true)
	req := httptest.NewRequest(http.MethodGet, "/local", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
}

func TestLocalMuxServesEmbeddedViewerAssets(t *testing.T) {
	matches, err := fs.Glob(embeddedViewer, "viewer/assets/*.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no embedded viewer JavaScript asset found")
	}

	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3717/local", true)
	req := httptest.NewRequest(http.MethodGet, "/local"+strings.TrimPrefix(matches[0], "viewer"), nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResolveWebDirUsesExplicitPath(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "package.json"), []byte(`{"name":"web"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveWebDir(webDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != webDir {
		t.Fatalf("web dir = %q, want %q", got, webDir)
	}
}

func TestResolveWebDirUsesEnvPath(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "package.json"), []byte(`{"name":"web"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ISOPRISM_WEB_DIR", webDir)

	got, err := resolveWebDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != webDir {
		t.Fatalf("web dir = %q, want %q", got, webDir)
	}
}
