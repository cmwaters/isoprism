package models

import "time"

// ── User ──────────────────────────────────────────────────────────────────────

type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	DisplayName    *string   `json:"display_name"`
	AvatarURL      *string   `json:"avatar_url"`
	GitHubUserID   *int64    `json:"github_user_id"`
	GitHubUsername *string   `json:"github_username"`
	CreatedAt      time.Time `json:"created_at"`
}

// ── GitHub integration ────────────────────────────────────────────────────────

type GitHubInstallation struct {
	ID               string    `json:"id"`
	InstallationID   int64     `json:"installation_id"`
	AccountLogin     string    `json:"account_login"`
	AccountType      string    `json:"account_type"`
	AccountAvatarURL *string   `json:"account_avatar_url"`
	CreatedAt        time.Time `json:"created_at"`
}

// ── Repository ────────────────────────────────────────────────────────────────

type Repository struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	InstallationID string    `json:"installation_id"`
	GitHubRepoID   int64     `json:"github_repo_id"`
	FullName       string    `json:"full_name"`
	DefaultBranch  string    `json:"default_branch"`
	MainCommitSHA  *string   `json:"main_commit_sha"`
	IndexStatus    string    `json:"index_status"` // pending | running | ready | failed
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
}

// ── Pull Request ──────────────────────────────────────────────────────────────

type PullRequest struct {
	ID              string     `json:"id"`
	RepoID          string     `json:"repo_id"`
	GitHubPRID      int64      `json:"github_pr_id"`
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	Body            *string    `json:"body"`
	AuthorLogin     string     `json:"author_login"`
	AuthorAvatarURL *string    `json:"author_avatar_url"`
	BaseBranch      string     `json:"base_branch"`
	HeadBranch      string     `json:"head_branch"`
	BaseCommitSHA   *string    `json:"base_commit_sha"`
	HeadCommitSHA   *string    `json:"head_commit_sha"`
	State           string     `json:"state"` // open | closed | merged
	Draft           bool       `json:"draft"`
	HTMLURL         string     `json:"html_url"`
	OpenedAt        time.Time  `json:"opened_at"`
	MergedAt        *time.Time `json:"merged_at"`
	LastActivityAt  *time.Time `json:"last_activity_at"`
	GraphStatus     string     `json:"graph_status"` // pending | running | ready | failed
	CreatedAt       time.Time  `json:"created_at"`
}

// ── Code graph ────────────────────────────────────────────────────────────────

type CodeNode struct {
	ID        string    `json:"id"`
	RepoID    string    `json:"repo_id"`
	CommitSHA string    `json:"commit_sha"`
	Name      string    `json:"name"`
	FullName  string    `json:"full_name"`
	FilePath  string    `json:"file_path"`
	LineStart int       `json:"line_start"`
	LineEnd   int       `json:"line_end"`
	Signature string    `json:"signature"`
	Language  string    `json:"language"`
	Kind      string    `json:"kind"`
	BodyHash  string    `json:"body_hash"`
	Summary   *string   `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

type CodeEdge struct {
	ID        string    `json:"id"`
	RepoID    string    `json:"repo_id"`
	CommitSHA string    `json:"commit_sha"`
	CallerID  string    `json:"caller_id"`
	CalleeID  string    `json:"callee_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ── PR delta ──────────────────────────────────────────────────────────────────

type PRNodeChange struct {
	ID            string    `json:"id"`
	PullRequestID string    `json:"pull_request_id"`
	NodeID        string    `json:"node_id"`
	ChangeType    string    `json:"change_type"` // added | modified | deleted
	ChangeSummary *string   `json:"change_summary"`
	DiffHunk      *string   `json:"diff_hunk"`
	CreatedAt     time.Time `json:"created_at"`
}

type PRAnalysis struct {
	ID            string     `json:"id"`
	PullRequestID string     `json:"pull_request_id"`
	Summary       *string    `json:"summary"`
	NodesChanged  int        `json:"nodes_changed"`
	RiskScore     *int       `json:"risk_score"`
	RiskLabel     *string    `json:"risk_label"`
	AIModel       *string    `json:"ai_model"`
	GeneratedAt   *time.Time `json:"generated_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ── Queue ─────────────────────────────────────────────────────────────────────

// QueuePR is a PR enriched with urgency data for queue display.
type QueuePR struct {
	PullRequest
	Summary      *string `json:"summary"`
	NodesChanged int     `json:"nodes_changed"`
	RiskScore    *int    `json:"risk_score"`
	RiskLabel    *string `json:"risk_label"`
	UrgencyScore float64 `json:"urgency_score"`
}

// ── Graph API response ────────────────────────────────────────────────────────

// GraphNode is a node in the PR graph response, tagged with its computed role.
type GraphNode struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	FullName      string  `json:"full_name"`
	FilePath      string  `json:"file_path"`
	LineStart     int     `json:"line_start"`
	LineEnd       int     `json:"line_end"`
	Signature     string  `json:"signature"`
	Language      string  `json:"language"`
	Kind          string  `json:"kind"`
	NodeType      string  `json:"node_type"` // changed | caller | callee
	Summary       *string `json:"summary"`
	ChangeSummary *string `json:"change_summary"`
	DiffHunk      *string `json:"diff_hunk"`
	ChangeType    *string `json:"change_type"` // added | modified | deleted | nil for unchanged
	// Diff stats (only for changed nodes)
	LinesAdded   int `json:"lines_added"`
	LinesRemoved int `json:"lines_removed"`
}

type GraphEdge struct {
	CallerID string `json:"caller_id"`
	CalleeID string `json:"callee_id"`
}

type GraphResponse struct {
	PR    GraphPR     `json:"pr"`
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphPR struct {
	ID            string `json:"id"`
	Number        int    `json:"number"`
	Title         string `json:"title"`
	HTMLURL       string `json:"html_url"`
	BaseCommitSHA string `json:"base_commit_sha"`
	HeadCommitSHA string `json:"head_commit_sha"`
	Body          string `json:"body"`
	AuthorLogin   string `json:"author_login"`
}

// ── Node code API response ───────────────────────────────────────────────────

type NodeCodeSegment struct {
	CommitSHA string `json:"commit_sha"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Source    string `json:"source"`
}

type NodeCodeResponse struct {
	NodeID     string           `json:"node_id"`
	FilePath   string           `json:"file_path"`
	Language   string           `json:"language"`
	Base       *NodeCodeSegment `json:"base,omitempty"`
	Head       *NodeCodeSegment `json:"head,omitempty"`
	DiffHunk   *string          `json:"diff_hunk,omitempty"`
	ChangeType *string          `json:"change_type,omitempty"`
}
