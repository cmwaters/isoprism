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
  github_access_status?: "authorized" | "revoked";
  authorized_at?: string;
  revoked_at?: string;
  indexed_at?: string;
  selected_at?: string;
  unused_at?: string;
  purge_after?: string;
  is_selected?: boolean;
  user_class?: "pilot" | "regular";
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
  graph_status: "pending" | "running" | "ready" | "skipped" | "failed";
  created_at: string;
}

// ── Queue ─────────────────────────────────────────────────────────────────────

export interface QueuePR extends PullRequest {
  summary?: string;
  nodes_changed: number;
  additions: number;
  deletions: number;
  risk_score?: number;
  risk_label?: "low" | "medium" | "high";
  urgency_score: number;
}

// QueueResponse describes an outbound response for web API utilities.
export interface QueueResponse {
  prs: QueuePR[];
}

// ── Graph ─────────────────────────────────────────────────────────────────────

export interface GraphNode {
  id: string;
  full_name: string;
  file_path: string;
  package_path?: string;
  line_start: number;
  line_end: number;
  inputs: GraphNodeTypeRef[];
  outputs: GraphNodeTypeRef[];
  language: string;
  kind: string;
  is_test: boolean;
  is_entrypoint: boolean;
  node_type: "changed" | "caller" | "callee" | "entrypoint" | "context";
  doc_comment?: string;
  summary?: string;
  change_summary?: string;
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted" | "renamed";
  old_full_name?: string;
  old_file_path?: string;
  lines_added: number;
  lines_removed: number;
  weight: number;
  degree: number;
  graph_depth: number;
  boundary: boolean;
  tests: GraphNodeTest[];
}

// GraphNodeTypeRef describes a graph node used by web API utilities.
export interface GraphNodeTypeRef {
  name?: string;
  type: string;
  node_id?: string;
}

// GraphNodeTest describes a graph node used by web API utilities.
export interface GraphNodeTest {
  name: string;
  full_name: string;
  file_path: string;
  line_start: number;
  line_end: number;
}

// GraphEdge describes a graph edge used by web API utilities.
export interface GraphEdge {
  source_id: string;
  destination_id: string;
  edge_kind: "calls" | "owns_method" | "uses_type";
  change_type?: "added" | "deleted";
  weight?: number;
  changed_weight?: number;
  underlying_edge_count?: number;
  sample_edges?: GraphEdgeSample[];
}

// GraphEdgeSample describes a graph edge used by web API utilities.
export interface GraphEdgeSample {
  source_id: string;
  destination_id: string;
  source_name: string;
  destination_name: string;
}

// GraphPR describes pull request data used by web API utilities.
export interface GraphPR {
  id: string;
  number: number;
  title: string;
  html_url: string;
  base_branch?: string;
  head_branch?: string;
  base_commit_sha: string;
  head_commit_sha: string;
  body: string;
  summary?: string;
  author_login: string;
}

// GitHubIssueReference defines the interface required by web API utilities.
export interface GitHubIssueReference {
  owner: string;
  repo: string;
  number: number;
}

// GitHubIssueDescription defines the interface required by web API utilities.
export interface GitHubIssueDescription {
  owner: string;
  repo: string;
  number: number;
  title: string;
  body: string;
  html_url: string;
  state: "open" | "closed" | string;
  author_login: string;
}

// PRFileDiff describes pull request data used by web API utilities.
export interface PRFileDiff {
  filename: string;
  previous_filename?: string;
  status: "added" | "modified" | "removed" | "renamed" | string;
  additions: number;
  deletions: number;
  changes: number;
  patch?: string;
}

// GraphResponse describes an outbound response for web API utilities.
export interface GraphResponse {
  pr: GraphPR;
  nodes: GraphNode[];
  edges: GraphEdge[];
  files: PRFileDiff[];
  test_changes: GraphNode[];
  test_context: GraphNode[];
}

// RepoGraphResponse describes an outbound response for web API utilities.
export interface RepoGraphResponse {
  repo: Repository;
  programs?: GraphProgram[];
  nodes: GraphNode[];
  edges: GraphEdge[];
}

// RepoProgramsResponse describes an outbound response for web API utilities.
export interface RepoProgramsResponse {
  repo: Repository;
  programs: GraphProgram[];
}

// GraphProgram defines the interface required by web API utilities.
export interface GraphProgram {
  id: string;
  full_name: string;
  file_path: string;
  package_path?: string;
  line_start: number;
  line_end: number;
  language: string;
  kind: string;
  is_entrypoint: boolean;
  summary?: string;
}

// GraphExpansionContext defines the interface required by web API utilities.
export interface GraphExpansionContext {
  mode: "repo" | "pr";
  pr_id?: string;
}

// GraphExpansionRequest describes an inbound request for web API utilities.
export interface GraphExpansionRequest {
  node_id: string;
  visible_node_ids: string[];
  graph_context: GraphExpansionContext;
}

// GraphExpansionResponse describes an outbound response for web API utilities.
export interface GraphExpansionResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
  expanded_node_id: string;
  has_more: boolean;
  hidden_neighbor_count: number;
}

// ── Node code ────────────────────────────────────────────────────────────────

export interface NodeCodeSegment {
  commit_sha: string;
  start_line: number;
  end_line: number;
  source: string;
}

// NodeCodeResponse describes an outbound response for web API utilities.
export interface NodeCodeResponse {
  node_id: string;
  file_path: string;
  language: string;
  base?: NodeCodeSegment;
  head?: NodeCodeSegment;
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted" | "renamed";
}

// ── Repo status ───────────────────────────────────────────────────────────────

export interface RepoStatus {
  index_status: "pending" | "running" | "ready" | "failed";
  pr_count: number;
  ready_count: number;
}

// ── Beta feedback ────────────────────────────────────────────────────────────

export interface BetaFeedbackPayload {
  type: "bug" | "feature";
  title: string;
  details: string;
  user_id?: string;
  repo_full_name: string;
  repo_id: string;
  pr_number?: number;
  pr_title?: string;
  node_full_name?: string;
  node_file_path?: string;
  browser_path: string;
  app_commit_sha: string;
  source_commit_sha?: string;
}

// BetaFeedbackResponse describes an outbound response for web API utilities.
export interface BetaFeedbackResponse {
  status: "submitted";
  issue: number;
  html_url: string;
}
