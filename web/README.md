# Isoprism Web

Isoprism is a PR review interface that turns code changes into a semantic call graph. Reviewers can inspect changed functions, structs, and nearby callers/callees, then switch between summary-first review and source/diff review for a selected node.

## Local development

```bash
npm install
npm run dev
```

The web app runs on [http://localhost:3000](http://localhost:3000). It talks to the Go API at `NEXT_PUBLIC_API_URL`, defaulting to `http://localhost:8080`.

Useful checks:

```bash
npm run lint
npm run build
```

## PR graph view

The PR page at `/repos/[repoID]/pr/[prID]` fetches `GET /api/v1/repos/{repoID}/prs/{prID}/graph` and renders:

- A call graph of changed nodes plus nearby caller/callee context.
- A left side panel that defaults to the semantic overview.
- A side panel that reviewers can resize between bounded minimum and maximum widths.
- Node cards with package/type labels, signatures, and added/removed/deleted pills.

The side panel has two modes:

- `Overview`: semantic PR or node summary, change explanation, diff stats, calls, and callers.
- `Code`: a lazy-loaded source viewer for the selected function or struct. Changed nodes automatically show the PR diff. Unchanged context nodes automatically show plain source.

The overview/code icon controls switch the side panel mode without changing the selected graph node.

## API contract used by the code panel

The code panel lazily fetches:

```http
GET /api/v1/repos/{repoID}/prs/{prID}/nodes/{nodeID}/code
```

Response:

```ts
interface NodeCodeResponse {
  node_id: string;
  file_path: string;
  language: string;
  base?: {
    commit_sha: string;
    start_line: number;
    end_line: number;
    source: string;
  };
  head?: {
    commit_sha: string;
    start_line: number;
    end_line: number;
    source: string;
  };
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted";
}
```

Expected behavior:

- `modified`: may include both `base` and `head`, plus `diff_hunk`.
- `added`: includes `head` when source can be fetched, plus `diff_hunk`.
- `deleted`: includes `base` when source can be fetched, plus `diff_hunk`.
- Caller/callee context nodes are not PR changes; they show the head version when available.
- If GitHub source fetching fails, the UI still uses `diff_hunk` from graph processing when present.

## Development debug endpoints

The API includes unauthenticated debug endpoints for rebuilding graph data:

```http
POST /debug/repos/{repoID}/reindex
POST /debug/prs/{prID}/reprocess
```

Both endpoints are idempotent and safe to call during development. `reindex` rebuilds `code_nodes` and `code_edges` from main branch HEAD. `reprocess` rebuilds `pr_node_changes` and PR call edges for one PR.
