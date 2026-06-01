# CLI API

## Boundary

The CLI daemon API is the boundary between the local Go process and the browser UI displayed at `/local`. It is separate from the cloud API. It should use local concepts directly instead of pretending that the local checkout is a cloud repository.

The current implementation still exposes some cloud-shaped compatibility routes under `/api/v1/repos/local/...` because the first local viewer reused cloud graph components directly. New work should move toward local-specific routes and React adapters.

## Current Local API

Base URL while serving locally:

```text
http://127.0.0.1:3717
```

Viewer/static routes:

```text
GET /                         redirects to /local when the viewer is enabled
GET /local                    embedded local viewer
GET /local/                   embedded local viewer
GET /local/assets/*           embedded viewer assets
```

Data routes:

```text
GET  /api/diff
GET  /api/v1/local/repo
POST /api/v1/local/review/compare
GET  /api/v1/repos/local/queue
GET  /api/v1/repos/local/programs
GET  /api/v1/repos/local/programs/{nodeID}/graph
POST /api/v1/repos/local/graph/expand
GET  /api/v1/repos/local/nodes/{nodeID}/code
```

`POST /api/v1/repos/local/graph/expand` accepts:

```json
{
  "node_id": "node-id",
  "visible_node_ids": ["node-id"],
  "graph_context": { "mode": "repo" }
}
```

`POST /api/v1/local/review/compare` accepts:

```json
{
  "base_ref": "main",
  "head_ref": "worktree"
}
```

Both fields are optional. If `base_ref` is empty, the daemon uses the detected default branch. If `head_ref` is empty, the daemon uses `worktree`. The daemon indexes any uncached trees or working-tree blobs before returning the semantic review graph.

Supported `head_ref` aliases:

- `worktree`, `working-tree`, or `unstaged` compare the base ref against the current working tree.
- `staged` compares the base ref against the git index.
- Any other value is resolved as a branch, tag, or commit ref.

## Target Local API

The target API should be honest about being local:

```text
GET  /api/local/session
GET  /api/local/repo
GET  /api/local/programs
GET  /api/local/review-items
POST /api/local/review/compare
GET  /api/local/programs/{programID}/graph
GET  /api/local/review-items/{reviewItemID}/graph
POST /api/local/graph/expand
GET  /api/local/nodes/{nodeID}/code
GET  /api/local/diff
```

`review-items` may return an empty list. The UI must treat that as normal and omit review-specific sections.

## Session Response

```ts
type LocalSession = {
  id: string;
  mode: "local";
  repo: RepoInfo;
  capabilities: {
    canRefreshReviewItems?: boolean;
    canOpenExternalReview?: boolean;
  };
};
```

## Review Items

Review items are UI data, not a cloud queue. A local repo may have no review items. Future `gh` integration can populate this list with pull requests discovered from the current checkout.

```ts
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
```

## Graph Client Contract

Shared React should talk to a local adapter implementing:

```ts
interface GraphClient {
  getProgramGraph(programID: string): Promise<RepoGraphResponse>;
  getReviewGraph(reviewItemID: string): Promise<GraphResponse>;
  expand(req: GraphExpansionRequest): Promise<GraphExpansionResponse>;
  getNodeCode(nodeID: string, context?: CodeContext): Promise<NodeCodeResponse>;
}
```

The adapter maps that interface to the local API. The shared components should not know whether data came from git, `gh`, Postgres, or GitHub webhooks.

## Performance Rule

The daemon should not rebuild the full semantic tree separately for every metadata endpoint. Session/repo/program/review requests should share a generated snapshot keyed by repo root, cache directory, and git SHA. This is especially important for `/local`, where startup currently performs multiple graph-building requests.
