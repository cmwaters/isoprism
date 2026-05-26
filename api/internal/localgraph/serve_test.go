package localgraph

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalMuxRootRedirectsToWebViewer(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3000/local")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "http://127.0.0.1:3000/local" {
		t.Fatalf("Location = %q, want local viewer", got)
	}
}

func TestLocalMuxRootReportsAPIWhenWebViewerDisabled(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResolveWebDirUsesExplicitPathInsteadOfRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "package.json"), []byte(`{"name":"web"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveWebDir(repoRoot, webDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != webDir {
		t.Fatalf("web dir = %q, want %q", got, webDir)
	}
}

func TestResolveWebDirUsesEnvPathInsteadOfRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "package.json"), []byte(`{"name":"web"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ISOPRISM_WEB_DIR", webDir)

	got, err := resolveWebDir(repoRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != webDir {
		t.Fatalf("web dir = %q, want %q", got, webDir)
	}
}
