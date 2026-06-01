package localgraph

import (
	"context"
	"testing"
)

func TestListLocalReviewItemsCollapsesDuplicateMainWorktree(t *testing.T) {
	root := initLocalGraphTestRepo(t)
	writeTestFile(t, root, "new.go", "package main\n\nfunc localChange() {}\n")

	items, err := listLocalReviewItems(context.Background(), Options{RepoDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	item := items[0]
	if item.ID != localWorktreePRReviewID {
		t.Fatalf("item id = %q, want %q", item.ID, localWorktreePRReviewID)
	}
	if item.Title != "Worktree" {
		t.Fatalf("title = %q, want Worktree", item.Title)
	}
	if item.HeadBranch != "main" || item.BaseBranch != "main" {
		t.Fatalf("branches = %s -> %s, want main -> main", item.HeadBranch, item.BaseBranch)
	}
}

func TestListLocalReviewItemsLabelsRemoteDefaultBranch(t *testing.T) {
	root := initLocalGraphTestRepo(t)
	bare := t.TempDir() + "/origin.git"
	runGitTestCommand(t, root, "init", "--bare", bare)
	runGitTestCommand(t, root, "remote", "add", "origin", bare)
	runGitTestCommand(t, root, "push", "-u", "origin", "main")
	writeTestFile(t, root, "new.go", "package main\n\nfunc localChange() {}\n")

	items, err := listLocalReviewItems(context.Background(), Options{RepoDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	item := items[0]
	if item.HeadBranch != "main" || item.BaseBranch != "origin/main" {
		t.Fatalf("branches = %s -> %s, want main -> origin/main", item.HeadBranch, item.BaseBranch)
	}
}

func TestListLocalReviewItemsShowsUncommittedWhenBranchDiffersFromDefault(t *testing.T) {
	root := initLocalGraphTestRepo(t)
	runGitTestCommand(t, root, "checkout", "-b", "feature")
	writeTestFile(t, root, "committed.go", "package main\n\nfunc committedChange() {}\n")
	runGitTestCommand(t, root, "add", "committed.go")
	runGitTestCommand(t, root, "commit", "-m", "feature change")
	writeTestFile(t, root, "uncommitted.go", "package main\n\nfunc uncommittedChange() {}\n")

	items, err := listLocalReviewItems(context.Background(), Options{RepoDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	if items[0].ID != localUncommittedReviewID || items[1].ID != localWorktreePRReviewID {
		t.Fatalf("item ids = %q, %q; want uncommitted then worktree", items[0].ID, items[1].ID)
	}
	for _, item := range items {
		if item.HeadBranch != "feature" {
			t.Fatalf("item %s head branch = %q, want feature", item.ID, item.HeadBranch)
		}
	}
}
