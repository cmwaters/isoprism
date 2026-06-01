package localgraph

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isoprism/api/internal/models"
)

// TestLocalMuxRootServesEmbeddedViewer renders the test local mux root serves embedded viewer for the local CLI graph runtime.
func TestLocalMuxRootServesEmbeddedViewer(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3717", true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
}

// TestLocalMuxRootReportsAPIWhenWebViewerDisabled verifies local mux root reports API when web viewer disabled.
func TestLocalMuxRootReportsAPIWhenWebViewerDisabled(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "", false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestLocalMuxServesEmbeddedViewer renders the test local mux serves embedded viewer for the local CLI graph runtime.
func TestLocalMuxServesEmbeddedViewer(t *testing.T) {
	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3717", true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
}

// TestLocalMuxServesEmbeddedViewerAssets verifies local mux serves embedded viewer assets.
func TestLocalMuxServesEmbeddedViewerAssets(t *testing.T) {
	matches, err := fs.Glob(embeddedViewer, "viewer/assets/*.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no embedded viewer JavaScript asset found")
	}

	mux := localMux(t.TempDir(), t.TempDir(), "http://127.0.0.1:3717", true)
	req := httptest.NewRequest(http.MethodGet, strings.TrimPrefix(matches[0], "viewer"), nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestLocalMuxReviewItemsUsesGH verifies local mux review items uses GitHub.
func TestLocalMuxReviewItemsUsesGH(t *testing.T) {
	root := initLocalGraphTestRepo(t)
	installFakeGH(t, `[{"number":7,"title":"Improve graph","body":"PR body","url":"https://github.com/acme/repo/pull/7","author":{"login":"octo"},"baseRefName":"main","baseRefOid":"base","headRefName":"feature","headRefOid":"head","additions":12,"deletions":3,"state":"OPEN","isDraft":false,"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-03T03:04:05Z"}]`, `{}`)

	mux := localMux(root, t.TempDir(), "", false)
	req := httptest.NewRequest(http.MethodGet, "/api/review-items", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		ReviewItems []models.QueuePR `json:"review_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ReviewItems) != 1 {
		t.Fatalf("review_items len = %d, want 1", len(got.ReviewItems))
	}
	pr := got.ReviewItems[0]
	if pr.ID != "gh-pr-7" || pr.Number != 7 || pr.Title != "Improve graph" || pr.AuthorLogin != "octo" {
		t.Fatalf("unexpected PR item: %+v", pr)
	}
	if pr.Additions != 12 || pr.Deletions != 3 {
		t.Fatalf("stats = +%d/-%d, want +12/-3", pr.Additions, pr.Deletions)
	}
}

// TestLoadGHPullRequestGraphFetchesHiddenPRRef verifies load GitHub pull request graph fetches hidden PR ref.
func TestLoadGHPullRequestGraphFetchesHiddenPRRef(t *testing.T) {
	root := initLocalGraphTestRepo(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	runGitTestCommand(t, root, "init", "--bare", bare)
	runGitTestCommand(t, root, "remote", "add", "origin", bare)
	runGitTestCommand(t, root, "push", "-u", "origin", "main")
	mergeBaseSHA := gitOutputTestCommand(t, root, "rev-parse", "HEAD")

	runGitTestCommand(t, root, "checkout", "-b", "feature")
	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n\nfunc reviewed() string { return \"yes\" }\n")
	runGitTestCommand(t, root, "add", "main.go")
	runGitTestCommand(t, root, "commit", "-m", "feature")
	headSHA := gitOutputTestCommand(t, root, "rev-parse", "HEAD")
	runGitTestCommand(t, root, "push", "origin", "HEAD:refs/pull/7/head")
	runGitTestCommand(t, root, "checkout", "main")
	writeTestFile(t, root, "unrelated.go", "package main\n\nfunc unrelated() {}\n")
	runGitTestCommand(t, root, "add", "unrelated.go")
	runGitTestCommand(t, root, "commit", "-m", "advance main")
	runGitTestCommand(t, root, "push", "origin", "main")
	currentBaseSHA := gitOutputTestCommand(t, root, "rev-parse", "HEAD")

	prJSON := `{"number":7,"title":"Visualise PR","body":"Adds reviewed().","url":"https://github.com/acme/repo/pull/7","author":{"login":"octo"},"baseRefName":"main","baseRefOid":"` + currentBaseSHA + `","headRefName":"feature","headRefOid":"` + headSHA + `","additions":2,"deletions":0,"state":"OPEN","isDraft":false,"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-03T03:04:05Z"}`
	installFakeGH(t, `[`+prJSON+`]`, prJSON)

	payload, err := loadGHPullRequestGraph(t.Context(), Options{RepoDir: root, CacheDir: t.TempDir()}, "gh-pr-7")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Graph.PR.ID != "gh-pr-7" || payload.Graph.PR.Number != 7 || payload.Graph.PR.Title != "Visualise PR" {
		t.Fatalf("unexpected graph PR metadata: %+v", payload.Graph.PR)
	}
	if payload.Graph.PR.BaseCommitSHA != mergeBaseSHA || payload.Graph.PR.HeadCommitSHA != headSHA {
		t.Fatalf("graph SHAs = %s..%s, want %s..%s", payload.Graph.PR.BaseCommitSHA, payload.Graph.PR.HeadCommitSHA, mergeBaseSHA, headSHA)
	}
	if payload.Graph.PR.HeadBranch != "feature" || payload.Graph.PR.BaseBranch != "main" {
		t.Fatalf("graph branches = %s -> %s, want feature -> main", payload.Graph.PR.HeadBranch, payload.Graph.PR.BaseBranch)
	}
	for _, file := range payload.Graph.Files {
		if file.Filename == "unrelated.go" {
			t.Fatal("PR graph included a base-branch-only file; expected merge-base comparison")
		}
	}
	if len(payload.Graph.Nodes) == 0 {
		t.Fatal("expected PR graph nodes")
	}
}

// TestResolveWebDirUsesExplicitPath verifies resolve web dir uses explicit path.
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

// installFakeGH installs a fake GitHub CLI for local daemon tests.
func installFakeGH(t *testing.T, listJSON, viewJSON string) {
	t.Helper()

	binDir := t.TempDir()
	path := filepath.Join(binDir, "gh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n" +
		"  printf '%s\\n' '" + listJSON + "'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n" +
		"  printf '%s\\n' '" + viewJSON + "'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo unexpected gh command: \"$@\" >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// gitOutputTestCommand builds a shell command that proxies git output in tests.
func gitOutputTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestResolveWebDirUsesEnvPath verifies resolve web dir uses env path.
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
