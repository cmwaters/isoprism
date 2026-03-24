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

export interface QueueItem extends PullRequest {
  urgency_score: number;
  review_state: "needs_review" | "needs_author" | "stalled" | "draft";
  waiting_hours: number;
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
