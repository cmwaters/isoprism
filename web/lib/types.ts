export interface Organization {
  id: string;
  name: string;
  slug: string;
  github_account_login: string;
  github_account_type: "Organization" | "User";
  github_account_id?: number;
  avatar_url?: string;
  created_at: string;
}

export interface Team {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  github_team_id?: number;
  created_at: string;
}

export interface Repository {
  id: string;
  org_id: string;
  installation_id: string;
  github_repo_id: number;
  full_name: string;
  default_branch: string;
  is_active: boolean;
  created_at: string;
}

export interface PRAnalysis {
  id: string;
  pull_request_id: string;
  commit_sha: string;
  summary?: string;
  why?: string;
  impacted_areas: string[];
  key_files: string[];
  size_label?: "small" | "medium" | "large";
  risk_score?: number;
  risk_label?: "low" | "medium" | "high";
  risk_reasons: string[];
}

export interface PullRequest {
  id: string;
  org_id: string;
  repo_id: string;
  github_pr_id: number;
  number: number;
  title: string;
  body?: string;
  author_github_login: string;
  author_avatar_url?: string;
  base_branch: string;
  head_branch: string;
  state: "open" | "closed" | "merged";
  draft: boolean;
  additions: number;
  deletions: number;
  changed_files: number;
  html_url: string;
  opened_at: string;
  closed_at?: string;
  merged_at?: string;
  last_activity_at?: string;
  repo_full_name?: string;
  analysis?: PRAnalysis;
}

/**
 * Author-bucket state reasons  → the PR author needs to act.
 * Reviewer-bucket state reasons → a reviewer needs to act.
 */
export type StateReason =
  // Author bucket
  | "ci_failing"         // CI is broken — fix the build
  | "merge_conflict"     // Merge conflicts — resolve them
  | "changes_requested"  // Reviewer requested changes
  | "unresolved_threads" // Reviewer asked a question; no reply yet
  | "ready_to_merge"     // Approved — merge it
  // Reviewer bucket
  | "re_review"          // Author pushed new commits after changes_requested
  | "review_requested"   // Explicitly asked to review
  | "needs_review";      // General review needed (assumed codeowner)

export type ActionBucket = "author" | "reviewer";
export type PriorityTier = "critical" | "high" | "medium";

export interface QueueItem extends PullRequest {
  waiting_hours: number;
  state_reason: StateReason;
  action_bucket: ActionBucket;
  priority_tier: PriorityTier;
}

export interface QueueResponse {
  items: QueueItem[];
  total: number;
}

export interface OrgJoinRequest {
  id: string;
  org_id: string;
  user_id: string;
  status: "pending" | "approved" | "rejected";
  created_at: string;
  github_username?: string;
  avatar_url?: string;
}
