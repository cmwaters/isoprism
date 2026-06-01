# CLI API

## Boundary

The CLI daemon API is the boundary between the local Go process and the browser UI served by the daemon. It is separate from the cloud API. It should use local checkout concepts directly instead of pretending that the local checkout is a cloud repository.

Canonical CLI daemon routes live under `/api/...`. They should not include `local` in the path.

Base URL while serving locally:

```text
http://127.0.0.1:3717
```

Viewer/static routes:

```text
GET /                         embedded viewer
GET /assets/*                 embedded viewer assets
```

## Data Routes

```text
GET  /api/session
GET  /api/repo
GET  /api/programs
GET  /api/review-items
POST /api/review/compare
GET  /api/programs/{programID}/graph
GET  /api/review-items/{reviewItemID}/graph
POST /api/graph/expand
GET  /api/nodes/{nodeID}/code
GET  /api/diff
```

## Shared Types

```ts
type LocalSession = {
  id: string;
  mode: "local";
  repo: Repository;
  capabilities: {
    canCompareRefs: boolean;
    canReadWorkingTree: boolean;
    canReadGitIndex: boolean;
    canUseGh?: boolean;
  };
};

type ReviewItem = {
  id: string;
  kind: "pull_request" | "local_diff" | "staged_diff" | "unstaged_diff";
  source: "gh" | "git";
  title: string;
  number?: number;
  summary?: string;
  url?: string;
  baseRef?: string;
  headRef?: string;
  additions?: number;
  deletions?: number;
  status?: string;
};

type ReviewCompareRequest = {
  base_ref?: string;
  head_ref?: string;
};

type GraphExpansionRequest = {
  node_id: string;
  visible_node_ids: string[];
  graph_context: {
    mode: "repo" | "pr";
    pr_id?: string;
  };
};

type CodeContext = {
  graph_context?: {
    mode: "repo" | "review";
    review_item_id?: string;
  };
};
```

`Repository`, `GraphProgram`, `RepoGraphResponse`, `GraphResponse`, `GraphExpansionResponse`, `NodeCodeResponse`, `GraphNode`, and `GraphEdge` use the shared graph contracts in `docs/spec/common/graph.md` and the common React adapter contracts in `docs/spec/common/ui.md`.

## Endpoint Reference

### `GET /api/session`

Returns daemon session metadata for the current checkout.

Input: none.

Response: `LocalSession`.

Use this to discover the active repo, local capabilities, and whether optional integrations such as `gh` are available.

### `GET /api/repo`

Returns lightweight repository metadata for the current git checkout.

Input: none.

Response: `Repository`.

This endpoint must not load, build, or refresh the semantic graph. It may use cheap git operations, such as resolving the repo root, default branch, and current `HEAD` commit SHA. Graph loading happens in later program, graph expansion, node code, and review comparison calls.

### `GET /api/programs`

Loads the semantic graph for the current `HEAD` commit and returns repo metadata plus indexed program entrypoints.

Input: none.

Response:

```ts
type RepoProgramsResponse = {
  repo: Repository;
  programs: GraphProgram[];
};
```

The embedded viewer should use this as the primary graph-building boot request because it contains both repo metadata and the program list.

### `GET /api/review-items`

Returns available local review contexts.

Input: none.

Response:

```ts
type ReviewItemsResponse = {
  review_items: ReviewItem[];
};
```

`review_items` may be empty. That is a normal local repo browsing state.

The daemon may include up to two local review items before external PRs:

- `local-uncommitted`: current uncommitted checkout changes, shown as `HEAD -> worktree`, only when that diff is non-empty.
- `local-worktree-pr`: the current worktree as it would appear if opened as a GitHub PR against the default branch, shown as `worktree -> origin/<default-branch>`, only when that merge-base diff is non-empty.

When `gh` is installed and authenticated, the daemon also populates this list from `gh pr list --state open --limit 50 --json ...`. The returned review cards are intentionally compatible with the shared cloud queue UI so the local browser can show title, author, URL, base/head refs, and diff stats without a GitHub App or cloud database.

### `POST /api/review/compare`

Builds a semantic review graph by comparing two local git states.

Input: `ReviewCompareRequest`.

Example:

```json
{
  "base_ref": "main",
  "head_ref": "worktree"
}
```

Response: `GraphResponse`.

Both fields are optional. If `base_ref` is empty, the daemon uses the detected default branch. If `head_ref` is empty, the daemon uses `worktree`. The daemon indexes any uncached branches, commits, staged blobs, or working-tree files before calculating the semantic diff.

Supported `head_ref` aliases:

- `worktree`, `working-tree`, or `unstaged` compare the base ref against the current working tree.
- `staged` compares the base ref against the git index.
- Any other value is resolved as a branch, tag, or commit ref.

### `GET /api/programs/{programID}/graph`

Returns a bounded repo graph focused on one program entrypoint.

Input:

- `programID`: a `GraphProgram.id` returned by `GET /api/programs`.

Response: `RepoGraphResponse`.

The returned graph should include the selected program node and nearby semantic context. The UI should select and center the requested program node after loading this graph.

### `GET /api/review-items/{reviewItemID}/graph`

Returns the semantic graph for a previously discovered review item.

Input:

- `reviewItemID`: a `ReviewItem.id` returned by `GET /api/review-items`.

Response: `GraphResponse`.

For local git diffs this graph is equivalent to a cached compare result. For `local-uncommitted`, the daemon compares `HEAD -> worktree`. For `local-worktree-pr`, it resolves the merge base between the default branch and `HEAD`, then compares that merge base against `worktree`.

For `gh` PRs, the daemon runs `gh pr view <number> --json ...`, fetches the PR head into `refs/isoprism/pr/<number>/head`, resolves the merge base between the PR base ref and the hidden head ref, compares that merge base against the hidden head ref, and returns the same `GraphResponse` shape as the cloud PR graph. This matches GitHub's PR file list semantics and avoids counting unrelated base-branch changes.

The daemon must not check out the PR branch or mutate the working tree while loading a PR graph.

### `POST /api/graph/expand`

Expands graph context around a visible node.

Input: `GraphExpansionRequest`.

Example:

```json
{
  "node_id": "node-id",
  "visible_node_ids": ["already-visible-node-id"],
  "graph_context": {
    "mode": "repo"
  }
}
```

Response: `GraphExpansionResponse`.

`nodes` and `edges` must always be arrays. If nothing new is available, return empty arrays rather than `null`.

### `GET /api/nodes/{nodeID}/code`

Returns source code and/or diff hunks for a node.

Input:

- `nodeID`: a `GraphNode.id`.
- Optional `review_item_id` query parameter when a node belongs to a specific review graph.

Response: `NodeCodeResponse`.

For repo graphs, this returns the source segment at the indexed commit. For review graphs, this may return `base`, `head`, `diff_hunk`, and `change_type` depending on whether the node was added, removed, or modified.

### `GET /api/diff`

Returns the full static diff payload used by CLI diff/export flows.

Input: currently none on the daemon route. CLI command flags provide refs and output behavior for static generation.

Response: `ReviewGraphPayload`.

This route is primarily for CLI/debug usage. The interactive viewer should prefer `POST /api/review/compare`.

## Graph Client Contract

Shared React should talk to a local adapter implementing:

```ts
interface GraphClient {
  getProgramGraph(programID: string): Promise<RepoGraphResponse>;
  getReviewGraph(reviewItemID: string): Promise<GraphResponse>;
  compareReview(req: ReviewCompareRequest): Promise<GraphResponse>;
  expand(req: GraphExpansionRequest): Promise<GraphExpansionResponse>;
  getNodeCode(nodeID: string, context?: CodeContext): Promise<NodeCodeResponse>;
}
```

The adapter maps that interface to the CLI daemon API. The shared components should not know whether data came from git, `gh`, Postgres, or GitHub webhooks.

## Performance Rule

The daemon should not rebuild the full semantic tree for lightweight metadata endpoints. Program, graph, node-code, and review requests should share a generated commit graph snapshot keyed by repo root, cache directory, and git SHA.

The embedded viewer startup path should fetch `GET /api/programs` as the single graph-building boot request because that response includes both repo metadata and programs. Cheap ancillary requests, such as `GET /api/review-items`, may run in parallel.
