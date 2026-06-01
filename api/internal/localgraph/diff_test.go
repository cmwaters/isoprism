package localgraph

import (
	"bytes"
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

func TestLoadTreeGraphSkipsFilesOverSemanticSizeCap(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runGitTestCommand(t, root, "init", "-b", "main")
	runGitTestCommand(t, root, "config", "user.email", "test@example.com")
	runGitTestCommand(t, root, "config", "user.name", "Test User")
	writeTestBytes(t, root, "large.go", append([]byte("package main\n\n"), bytes.Repeat([]byte("x"), int(maxSemanticFileBytes))...))
	runGitTestCommand(t, root, "add", "large.go")
	runGitTestCommand(t, root, "commit", "-m", "large file")

	g := gitClient{root: root}
	sha, err := g.resolveCommit(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	graph, err := loadTreeGraph(ctx, g, t.TempDir(), "HEAD", sha)
	if err != nil {
		t.Fatal(err)
	}

	if len(graph.tree) != 1 {
		t.Fatalf("tree has %d files, want 1", len(graph.tree))
	}
	if len(graph.nodes) != 0 {
		t.Fatalf("graph has %d nodes, want 0", len(graph.nodes))
	}
}

func TestGenerateDiffIncludesUntrackedWorktreeFiles(t *testing.T) {
	ctx := context.Background()
	root := initLocalGraphTestRepo(t)
	writeTestFile(t, root, "new.go", "package main\n\nfunc untracked() {}\n")

	payload, err := GenerateDiff(ctx, Options{RepoDir: root, Args: []string{"HEAD", "worktree"}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	foundFile := false
	for _, file := range payload.Graph.Files {
		if file.Filename == "new.go" {
			foundFile = true
			if file.Status != "added" {
				t.Fatalf("status = %q, want added", file.Status)
			}
			if file.Additions != 3 || file.Deletions != 0 {
				t.Fatalf("stats = +%d/-%d, want +3/-0", file.Additions, file.Deletions)
			}
		}
	}
	if !foundFile {
		t.Fatal("new.go not included in worktree diff files")
	}
	foundNode := false
	for _, node := range payload.Graph.Nodes {
		if node.FilePath == "new.go" && node.ChangeType != nil && *node.ChangeType == "added" {
			foundNode = true
		}
	}
	if !foundNode {
		t.Fatal("untracked function node not included as added")
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

	writeTestBytes(t, root, path, []byte(content))
}

func writeTestBytes(t *testing.T, root, path string, content []byte) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
