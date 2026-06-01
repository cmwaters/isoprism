package localgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/isoprism/api/internal/models"
)

const ghPRJSONFields = "number,title,body,url,author,baseRefName,baseRefOid,headRefName,headRefOid,additions,deletions,state,isDraft,createdAt,updatedAt"

type ghAuthor struct {
	Login string `json:"login"`
}

type ghPullRequest struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	Author      ghAuthor  `json:"author"`
	BaseRefName string    `json:"baseRefName"`
	BaseRefOID  string    `json:"baseRefOid"`
	HeadRefName string    `json:"headRefName"`
	HeadRefOID  string    `json:"headRefOid"`
	Additions   int       `json:"additions"`
	Deletions   int       `json:"deletions"`
	State       string    `json:"state"`
	IsDraft     bool      `json:"isDraft"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func listGHReviewItems(ctx context.Context, opts Options) ([]models.QueuePR, error) {
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return nil, err
	}
	if !ghAvailable() {
		return []models.QueuePR{}, nil
	}
	out, err := runGH(ctx, root, "pr", "list", "--state", "open", "--limit", "50", "--json", ghPRJSONFields)
	if err != nil {
		return nil, err
	}
	var prs []ghPullRequest
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list: %w", err)
	}
	items := make([]models.QueuePR, 0, len(prs))
	for _, pr := range prs {
		items = append(items, ghPullRequestQueueItem(pr))
	}
	return items, nil
}

func loadGHPullRequestGraph(ctx context.Context, opts Options, reviewItemID string) (ReviewGraphPayload, error) {
	number, ok := ghReviewItemNumber(reviewItemID)
	if !ok {
		return ReviewGraphPayload{}, fmt.Errorf("unknown review item %q", reviewItemID)
	}
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	if !ghAvailable() {
		return ReviewGraphPayload{}, fmt.Errorf("gh CLI is not installed or is not on PATH")
	}
	pr, err := loadGHPullRequest(ctx, root, number)
	if err != nil {
		return ReviewGraphPayload{}, err
	}

	g := gitClient{root: root}
	baseRef, headRef, err := ensureGHPullRequestRefs(ctx, g, pr)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	diffOpts := opts
	diffOpts.RepoDir = root
	diffOpts.Args = []string{baseRef, headRef}
	payload, err := GenerateDiff(ctx, diffOpts)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	applyGHPullRequestMetadata(&payload, pr, reviewItemID)
	return payload, nil
}

func loadGHPullRequest(ctx context.Context, root string, number int) (ghPullRequest, error) {
	out, err := runGH(ctx, root, "pr", "view", strconv.Itoa(number), "--json", ghPRJSONFields)
	if err != nil {
		return ghPullRequest{}, err
	}
	var pr ghPullRequest
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return ghPullRequest{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	if pr.Number == 0 {
		pr.Number = number
	}
	return pr, nil
}

func ensureGHPullRequestRefs(ctx context.Context, g gitClient, pr ghPullRequest) (string, string, error) {
	headRef := ghPullRequestHeadRef(pr.Number)
	if err := g.fetch(ctx, fmt.Sprintf("+pull/%d/head:%s", pr.Number, headRef)); err != nil {
		return "", "", fmt.Errorf("fetch PR #%d head from origin: %w", pr.Number, err)
	}

	headSHA, err := g.resolveCommit(ctx, headRef)
	if err != nil {
		return "", "", err
	}
	if pr.HeadRefOID != "" && headSHA != pr.HeadRefOID {
		return "", "", fmt.Errorf("fetched PR #%d head %s, expected %s", pr.Number, headSHA, pr.HeadRefOID)
	}

	baseRef := pr.BaseRefName
	if pr.BaseRefOID != "" {
		if !g.commitExists(ctx, pr.BaseRefOID) && pr.BaseRefName != "" {
			_ = g.fetch(ctx, pr.BaseRefName)
		}
		if g.commitExists(ctx, pr.BaseRefOID) {
			baseRef = pr.BaseRefOID
		}
	}
	if strings.TrimSpace(baseRef) == "" {
		return "", "", fmt.Errorf("PR #%d has no base ref", pr.Number)
	}
	mergeBase, err := g.mergeBase(ctx, baseRef, headRef)
	if err != nil {
		return "", "", fmt.Errorf("resolve PR #%d merge base: %w", pr.Number, err)
	}
	return mergeBase, headRef, nil
}

func runGH(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func ghPullRequestQueueItem(pr ghPullRequest) models.QueuePR {
	body := localStringPtr(strings.TrimSpace(pr.Body))
	updatedAt := pr.UpdatedAt
	return models.QueuePR{
		PullRequest: models.PullRequest{
			ID:              ghReviewItemID(pr.Number),
			RepoID:          "local",
			GitHubPRID:      int64(pr.Number),
			Number:          pr.Number,
			Title:           pr.Title,
			Body:            body,
			AuthorLogin:     pr.Author.Login,
			BaseBranch:      pr.BaseRefName,
			HeadBranch:      pr.HeadRefName,
			BaseCommitSHA:   localStringPtr(pr.BaseRefOID),
			HeadCommitSHA:   localStringPtr(pr.HeadRefOID),
			State:           normalizeGHPRState(pr.State),
			Draft:           pr.IsDraft,
			HTMLURL:         pr.URL,
			OpenedAt:        pr.CreatedAt,
			LastActivityAt:  &updatedAt,
			GraphStatus:     "ready",
			ProcessingStats: json.RawMessage(`{}`),
			CreatedAt:       pr.CreatedAt,
		},
		NodesChanged: 0,
		Additions:    pr.Additions,
		Deletions:    pr.Deletions,
		UrgencyScore: 0,
	}
}

func applyGHPullRequestMetadata(payload *ReviewGraphPayload, pr ghPullRequest, reviewItemID string) {
	payload.Mode = "pull_request"
	payload.Graph.PR = models.GraphPR{
		ID:            reviewItemID,
		Number:        pr.Number,
		Title:         pr.Title,
		HTMLURL:       pr.URL,
		BaseBranch:    pr.BaseRefName,
		HeadBranch:    pr.HeadRefName,
		BaseCommitSHA: payload.Diff.BaseSHA,
		HeadCommitSHA: firstNonEmpty(pr.HeadRefOID, payload.Diff.HeadSHA),
		Body:          pr.Body,
		AuthorLogin:   pr.Author.Login,
	}
	payload.Metadata["github_pr"] = map[string]any{
		"number":           pr.Number,
		"url":              pr.URL,
		"base_ref_name":    pr.BaseRefName,
		"base_ref_oid":     pr.BaseRefOID,
		"merge_base_oid":   payload.Diff.BaseSHA,
		"head_ref_name":    pr.HeadRefName,
		"head_ref_oid":     pr.HeadRefOID,
		"additions":        pr.Additions,
		"deletions":        pr.Deletions,
		"state":            pr.State,
		"is_draft":         pr.IsDraft,
		"created_at":       pr.CreatedAt,
		"last_activity_at": pr.UpdatedAt,
	}
}

func ghReviewItemID(number int) string {
	return fmt.Sprintf("gh-pr-%d", number)
}

func ghReviewItemNumber(id string) (int, bool) {
	value, ok := strings.CutPrefix(id, "gh-pr-")
	if !ok {
		return 0, false
	}
	number, err := strconv.Atoi(value)
	return number, err == nil && number > 0
}

func ghPullRequestHeadRef(number int) string {
	return fmt.Sprintf("refs/isoprism/pr/%d/head", number)
}

func normalizeGHPRState(state string) string {
	switch strings.ToLower(state) {
	case "closed":
		return "closed"
	case "merged":
		return "merged"
	default:
		return "open"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func localStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
