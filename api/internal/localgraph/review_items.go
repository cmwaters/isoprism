package localgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/isoprism/api/internal/models"
)

const (
	localUncommittedReviewID = "local-uncommitted"
	localWorktreePRReviewID  = "local-worktree-pr"
)

// listReviewItems lists review items for the local CLI graph runtime.
func listReviewItems(ctx context.Context, opts Options) ([]models.QueuePR, error) {
	var items []models.QueuePR
	localItems, err := listLocalReviewItems(ctx, opts)
	if err != nil {
		return nil, err
	}
	items = append(items, localItems...)

	ghItems, err := listGHReviewItems(ctx, opts)
	if err != nil {
		ghItems = []models.QueuePR{}
	}
	items = append(items, ghItems...)
	return items, nil
}

// listLocalReviewItems lists local review items for the local CLI graph runtime.
func listLocalReviewItems(ctx context.Context, opts Options) ([]models.QueuePR, error) {
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return nil, err
	}
	g := gitClient{root: root}
	defaultBranch, err := g.resolveDefaultBranch(ctx)
	if err != nil {
		return nil, err
	}
	defaultBranchRef := g.defaultBranchRef(ctx, defaultBranch)
	headSHA, err := g.resolveCommit(ctx, "HEAD")
	if err != nil {
		return nil, err
	}
	currentBranch := g.currentBranch(ctx)
	mergeBase, err := g.mergeBase(ctx, defaultBranchRef, "HEAD")
	if err != nil {
		return nil, err
	}

	var items []models.QueuePR
	if mergeBase != headSHA {
		if item, ok := localReviewItem(ctx, g, localUncommittedReviewID, "Uncommitted changes", "HEAD", worktreeTreeRef, "HEAD", currentBranch, headSHA, "worktree-"+headSHA); ok {
			items = append(items, item)
		}
	}
	if item, ok := localReviewItem(ctx, g, localWorktreePRReviewID, "Worktree", mergeBase, worktreeTreeRef, defaultBranchRef, currentBranch, mergeBase, "worktree-"+headSHA); ok {
		items = append(items, item)
	}
	return items, nil
}

// loadLocalReviewItemGraph loads local review item graph for the local CLI graph runtime.
func loadLocalReviewItemGraph(ctx context.Context, opts Options, reviewItemID string) (ReviewGraphPayload, error) {
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	g := gitClient{root: root}
	defaultBranch, err := g.resolveDefaultBranch(ctx)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	defaultBranchRef := g.defaultBranchRef(ctx, defaultBranch)
	headSHA, err := g.resolveCommit(ctx, "HEAD")
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	currentBranch := g.currentBranch(ctx)

	title := ""
	baseRef := ""
	headRef := worktreeTreeRef
	baseBranch := ""
	headBranch := currentBranch
	switch reviewItemID {
	case localUncommittedReviewID:
		title = "Uncommitted changes"
		baseRef = "HEAD"
		baseBranch = "HEAD"
	case localWorktreePRReviewID:
		title = "Worktree"
		mergeBase, err := g.mergeBase(ctx, defaultBranchRef, "HEAD")
		if err != nil {
			return ReviewGraphPayload{}, err
		}
		baseRef = mergeBase
		baseBranch = defaultBranchRef
	default:
		return ReviewGraphPayload{}, fmt.Errorf("unknown local review item %q", reviewItemID)
	}

	diffOpts := opts
	diffOpts.RepoDir = root
	diffOpts.Args = []string{baseRef, headRef}
	payload, err := GenerateDiff(ctx, diffOpts)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	payload.Mode = "local_review"
	payload.Graph.PR.ID = reviewItemID
	payload.Graph.PR.Number = 0
	payload.Graph.PR.Title = title
	payload.Graph.PR.HTMLURL = ""
	payload.Graph.PR.BaseBranch = baseBranch
	payload.Graph.PR.HeadBranch = headBranch
	payload.Graph.PR.BaseCommitSHA = payload.Diff.BaseSHA
	payload.Graph.PR.HeadCommitSHA = "worktree-" + headSHA
	payload.Graph.PR.Body = fmt.Sprintf("%s: %s -> %s", title, headBranch, baseBranch)
	payload.Graph.PR.AuthorLogin = "local"
	payload.Metadata["local_review_item"] = map[string]any{
		"id":          reviewItemID,
		"title":       title,
		"base_branch": baseBranch,
		"head_branch": headBranch,
	}
	return payload, nil
}

// localReviewItem builds a queue-compatible card for a local diff review.
func localReviewItem(ctx context.Context, g gitClient, id, title, baseRef, headRef, baseBranch, headBranch, baseSHA, headSHA string) (models.QueuePR, bool) {
	stats, err := g.diffNumstat(ctx, baseRef, headRef)
	if err != nil {
		return models.QueuePR{}, false
	}
	additions, deletions := sumDiffStats(stats)
	if additions == 0 && deletions == 0 {
		return models.QueuePR{}, false
	}
	now := time.Now().UTC()
	return models.QueuePR{
		PullRequest: models.PullRequest{
			ID:              id,
			RepoID:          "local",
			GitHubPRID:      0,
			Number:          0,
			Title:           title,
			AuthorLogin:     "local",
			BaseBranch:      baseBranch,
			HeadBranch:      headBranch,
			BaseCommitSHA:   localStringPtr(baseSHA),
			HeadCommitSHA:   localStringPtr(headSHA),
			State:           "open",
			Draft:           false,
			OpenedAt:        now,
			LastActivityAt:  &now,
			GraphStatus:     "ready",
			ProcessingStats: json.RawMessage(`{}`),
			CreatedAt:       now,
		},
		Additions:    additions,
		Deletions:    deletions,
		UrgencyScore: 0,
	}, true
}

// sumDiffStats totals additions and deletions across file stats.
func sumDiffStats(stats map[string][2]int) (int, int) {
	additions := 0
	deletions := 0
	for _, stat := range stats {
		additions += stat[0]
		deletions += stat[1]
	}
	return additions, deletions
}

// isLocalReviewItem reports whether local review item matches the expected condition.
func isLocalReviewItem(id string) bool {
	return id == localUncommittedReviewID || id == localWorktreePRReviewID
}
