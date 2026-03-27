package models

import "time"

// ---- Org model ----

type Organization struct {
	ID                 string    `json:"id" db:"id"`
	Name               string    `json:"name" db:"name"`
	Slug               string    `json:"slug" db:"slug"`
	GitHubAccountLogin string    `json:"github_account_login" db:"github_account_login"`
	GitHubAccountType  string    `json:"github_account_type" db:"github_account_type"`
	GitHubAccountID    *int64    `json:"github_account_id" db:"github_account_id"`
	AvatarURL          *string   `json:"avatar_url" db:"avatar_url"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

type OrgMember struct {
	ID        string    `json:"id" db:"id"`
	OrgID     string    `json:"org_id" db:"org_id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Role      string    `json:"role" db:"role"` // org_admin | member
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type OrgJoinRequest struct {
	ID         string     `json:"id" db:"id"`
	OrgID      string     `json:"org_id" db:"org_id"`
	UserID     string     `json:"user_id" db:"user_id"`
	Status     string     `json:"status" db:"status"` // pending | approved | rejected
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at" db:"resolved_at"`
	ResolvedBy *string    `json:"resolved_by" db:"resolved_by"`
}

// ---- Team model ----

type Team struct {
	ID           string    `json:"id" db:"id"`
	OrgID        string    `json:"org_id" db:"org_id"`
	Name         string    `json:"name" db:"name"`
	Slug         string    `json:"slug" db:"slug"`
	GitHubTeamID *int64    `json:"github_team_id" db:"github_team_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type TeamMember struct {
	ID        string    `json:"id" db:"id"`
	TeamID    string    `json:"team_id" db:"team_id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Role      string    `json:"role" db:"role"` // team_admin | member
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ---- User model ----

type User struct {
	ID             string    `json:"id" db:"id"`
	Email          string    `json:"email" db:"email"`
	DisplayName    *string   `json:"display_name" db:"display_name"`
	AvatarURL      *string   `json:"avatar_url" db:"avatar_url"`
	GitHubUserID   *int64    `json:"github_user_id" db:"github_user_id"`
	GitHubUsername *string   `json:"github_username" db:"github_username"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// ---- GitHub integration ----

type GitHubInstallation struct {
	ID               string    `json:"id" db:"id"`
	OrgID            string    `json:"org_id" db:"org_id"`
	InstallationID   int64     `json:"installation_id" db:"installation_id"`
	AccountLogin     string    `json:"account_login" db:"account_login"`
	AccountType      string    `json:"account_type" db:"account_type"`
	AccountAvatarURL *string   `json:"account_avatar_url" db:"account_avatar_url"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

type Repository struct {
	ID             string    `json:"id" db:"id"`
	OrgID          string    `json:"org_id" db:"org_id"`
	InstallationID string    `json:"installation_id" db:"installation_id"`
	GitHubRepoID   int64     `json:"github_repo_id" db:"github_repo_id"`
	FullName       string    `json:"full_name" db:"full_name"`
	DefaultBranch  string    `json:"default_branch" db:"default_branch"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// ---- Pull Requests ----

type PullRequest struct {
	ID                string     `json:"id" db:"id"`
	OrgID             string     `json:"org_id" db:"org_id"`
	RepoID            string     `json:"repo_id" db:"repo_id"`
	GitHubPRID        int64      `json:"github_pr_id" db:"github_pr_id"`
	Number            int        `json:"number" db:"number"`
	Title             string     `json:"title" db:"title"`
	Body              *string    `json:"body" db:"body"`
	AuthorGitHubLogin string     `json:"author_github_login" db:"author_github_login"`
	AuthorAvatarURL   *string    `json:"author_avatar_url" db:"author_avatar_url"`
	BaseBranch        string     `json:"base_branch" db:"base_branch"`
	HeadBranch        string     `json:"head_branch" db:"head_branch"`
	State             string     `json:"state" db:"state"`
	Draft             bool       `json:"draft" db:"draft"`
	Additions         int        `json:"additions" db:"additions"`
	Deletions         int        `json:"deletions" db:"deletions"`
	ChangedFiles      int        `json:"changed_files" db:"changed_files"`
	HTMLURL           string     `json:"html_url" db:"html_url"`
	OpenedAt          time.Time  `json:"opened_at" db:"opened_at"`
	ClosedAt          *time.Time `json:"closed_at" db:"closed_at"`
	MergedAt          *time.Time `json:"merged_at" db:"merged_at"`
	LastActivityAt    *time.Time `json:"last_activity_at" db:"last_activity_at"`
	LastSyncedAt      *time.Time `json:"last_synced_at" db:"last_synced_at"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
	HeadSHA           *string    `json:"head_sha,omitempty" db:"head_sha"`
	Mergeable         *bool      `json:"mergeable,omitempty" db:"mergeable"`

	// Joined fields
	RepoFullName string      `json:"repo_full_name,omitempty" db:"repo_full_name"`
	Analysis     *PRAnalysis `json:"analysis,omitempty" db:"-"`
	Reviews      []PRReview  `json:"reviews,omitempty" db:"-"`
}

type PRAnalysis struct {
	ID             string      `json:"id" db:"id"`
	PullRequestID  string      `json:"pull_request_id" db:"pull_request_id"`
	CommitSHA      string      `json:"commit_sha" db:"commit_sha"`
	Summary        *string     `json:"summary" db:"summary"`
	Why            *string     `json:"why" db:"why"`
	ImpactedAreas  []string    `json:"impacted_areas" db:"impacted_areas"`
	KeyFiles       []string    `json:"key_files" db:"key_files"`
	SizeLabel      *string     `json:"size_label" db:"size_label"`
	RiskScore      *int        `json:"risk_score" db:"risk_score"`
	RiskLabel      *string     `json:"risk_label" db:"risk_label"`
	RiskReasons    []string    `json:"risk_reasons" db:"risk_reasons"`
	SemanticGroups interface{} `json:"semantic_groups" db:"semantic_groups"`
	AIProvider     *string     `json:"ai_provider" db:"ai_provider"`
	AIModel        *string     `json:"ai_model" db:"ai_model"`
	GeneratedAt    *time.Time  `json:"generated_at" db:"generated_at"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
}

type PRReview struct {
	ID                string    `json:"id" db:"id"`
	PullRequestID     string    `json:"pull_request_id" db:"pull_request_id"`
	GitHubReviewID    int64     `json:"github_review_id" db:"github_review_id"`
	ReviewerLogin     string    `json:"reviewer_login" db:"reviewer_login"`
	ReviewerAvatarURL *string   `json:"reviewer_avatar_url" db:"reviewer_avatar_url"`
	State             string    `json:"state" db:"state"`
	CommitSHA         *string   `json:"commit_sha,omitempty" db:"commit_sha"` // SHA when review was submitted
	SubmittedAt       time.Time `json:"submitted_at" db:"submitted_at"`
}

// QueueItem is a PR enriched with personalised ranking signals.
//
// ActionBucket: "author" = you need to act | "reviewer" = you need to review.
//
// StateReason values:
//
//	Author bucket:   "ci_failing" | "merge_conflict" | "changes_requested" | "unresolved_threads" | "ready_to_merge"
//	Reviewer bucket: "re_review" | "review_requested" | "needs_review"
//
// PriorityTier: "critical" | "high" | "medium"
type QueueItem struct {
	PullRequest
	WaitingHours float64 `json:"waiting_hours"` // hours since last review or PR open
	StateReason  string  `json:"state_reason"`  // why this PR is in the queue
	ActionBucket string  `json:"action_bucket"` // "author" | "reviewer"
	PriorityTier string  `json:"priority_tier"` // "critical" | "high" | "medium"
	// TierScore is an internal sort key — not exposed to the client.
	TierScore int `json:"-"`
}
