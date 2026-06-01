package localgraph

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveDiffRefsSupportsLocalReviewAliases(t *testing.T) {
	ctx := context.Background()
	root := initLocalGraphTestRepo(t)
	g := gitClient{root: root}

	tests := []struct {
		name     string
		args     []string
		wantBase string
		wantHead string
	}{
		{
			name:     "unstaged defaults to default branch against worktree",
			args:     []string{"unstaged"},
			wantBase: "main",
			wantHead: worktreeTreeRef,
		},
		{
			name:     "worktree head",
			args:     []string{"main", "worktree"},
			wantBase: "main",
			wantHead: worktreeTreeRef,
		},
		{
			name:     "working tree head",
			args:     []string{"main", "working-tree"},
			wantBase: "main",
			wantHead: worktreeTreeRef,
		},
		{
			name:     "staged head",
			args:     []string{"main", "staged"},
			wantBase: "main",
			wantHead: indexTreeRef,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBase, gotHead, err := resolveDiffRefs(ctx, g, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if gotBase != tt.wantBase || gotHead != tt.wantHead {
				t.Fatalf("refs = %q, %q; want %q, %q", gotBase, gotHead, tt.wantBase, tt.wantHead)
			}
		})
	}
}

func initLocalGraphTestRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	runGitTestCommand(t, root, "init", "-b", "main")
	runGitTestCommand(t, root, "config", "user.email", "test@example.com")
	runGitTestCommand(t, root, "config", "user.name", "Test User")
	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	runGitTestCommand(t, root, "add", "main.go")
	runGitTestCommand(t, root, "commit", "-m", "initial")
	return root
}

func runGitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeTestFile(t *testing.T, root, path, content string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
