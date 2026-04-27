// ── Repository ────────────────────────────────────────────────────────────────

export interface Repository {
  id: string;
  user_id: string;
  installation_id: string;
  github_repo_id: number;
  full_name: string;
  default_branch: string;
  main_commit_sha?: string;
  index_status: "pending" | "running" | "ready" | "failed";
  is_active: boolean;
  created_at: string;
}

// ── Pull Request ──────────────────────────────────────────────────────────────

export interface PullRequest {
  id: string;
  repo_id: string;
  github_pr_id: number;
  number: number;
  title: string;
  body?: string;
  author_login: string;
  author_avatar_url?: string;
  base_branch: string;
  head_branch: string;
  base_commit_sha?: string;
  head_commit_sha?: string;
  state: "open" | "closed" | "merged";
  draft: boolean;
  html_url: string;
  opened_at: string;
  merged_at?: string;
  last_activity_at?: string;
  graph_status: "pending" | "running" | "ready" | "failed";
  created_at: string;
}

// ── Queue ─────────────────────────────────────────────────────────────────────

export interface QueuePR extends PullRequest {
  summary?: string;
  nodes_changed: number;
  risk_score?: number;
  risk_label?: "low" | "medium" | "high";
  urgency_score: number;
}

export interface QueueResponse {
  prs: QueuePR[];
}

// ── Graph ─────────────────────────────────────────────────────────────────────

export interface GraphNode {
  id: string;
  name: string;
  full_name: string;
  file_path: string;
  line_start: number;
  line_end: number;
  signature: string;
  language: string;
  kind: string;
  node_type: "changed" | "caller" | "callee" | "entrypoint" | "context";
  summary?: string;
  change_summary?: string;
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted";
  lines_added: number;
  lines_removed: number;
  tests: GraphNodeTest[];
}

export interface GraphNodeTest {
  name: string;
  full_name: string;
  file_path: string;
  line_start: number;
  line_end: number;
}

export interface GraphEdge {
  caller_id: string;
  callee_id: string;
}

export interface GraphPR {
  id: string;
  number: number;
  title: string;
  html_url: string;
  base_commit_sha: string;
  head_commit_sha: string;
  body: string;
  author_login: string;
}

export interface GraphResponse {
  pr: GraphPR;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface RepoGraphResponse {
  repo: Repository;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

// ── Node code ────────────────────────────────────────────────────────────────

export interface NodeCodeSegment {
  commit_sha: string;
  start_line: number;
  end_line: number;
  source: string;
}

export interface NodeCodeResponse {
  node_id: string;
  file_path: string;
  language: string;
  base?: NodeCodeSegment;
  head?: NodeCodeSegment;
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted";
}

// ── Repo status ───────────────────────────────────────────────────────────────

export interface RepoStatus {
  index_status: "pending" | "running" | "ready" | "failed";
  pr_count: number;
  ready_count: number;
}
