# Isoprism Project Reference

Use this reference when the task needs current Isoprism behavior, repo maintenance context, or implementation constraints beyond the main skill workflow.

## Product Shape

Isoprism validates whether a semantic graph representation of software changes helps reviewers understand PRs faster than raw diffs. The hosted prototype is an invite-only pilot: users register at `/pilot/register`, admins approve and send `/pilot/{token}` invites, users connect GitHub, select one repository, review PRs for one week, submit feedback from the app, and complete `/pilot/review/{token}`.

The local-first direction is the CLI:

```bash
isoprism diff
isoprism serve
```

The CLI should work in any git repo without GitHub permissions, Supabase credentials, Node.js, or an Isoprism source checkout. The current source implementation lives under `api/cmd/isoprism`.

## Current CLI Contract

Supported examples from the source checkout:

```bash
cd api
go run ./cmd/isoprism diff
go run ./cmd/isoprism diff <ref>
go run ./cmd/isoprism diff <ref1> <ref2>
go run ./cmd/isoprism diff staged
go run ./cmd/isoprism diff <ref1> <ref2> --json
go run ./cmd/isoprism diff <ref1> <ref2> --output /tmp/isoprism.html --no-open
go run ./cmd/isoprism serve --port 3717
go run ./cmd/isoprism serve --web-dir ../web --web-port 3000
go run ./cmd/isoprism serve --no-web
go run ./cmd/isoprism annotate diff --reason-for-change "..." --expected-outcome "..."
go run ./cmd/isoprism annotate node <node-sha256> --description "..." --reasoning "..." --confidence high
go run ./cmd/isoprism annotate test --node <node-sha256> --description "..."
```

Implemented behavior:

- `diff` resolves the repo root from local git.
- Default branch detection tries `origin/HEAD`, `git remote show origin`, local `main`, then local `master`.
- `diff <ref>` and `diff <ref1> <ref2>` work for committed refs.
- `diff staged` compares `HEAD` to the git index without `git write-tree`.
- `--json` emits a review graph payload that wraps the web `GraphResponse` shape.
- `--output` writes a self-contained static HTML artifact with embedded payload and file patches.
- `.isoprism/objects/nodes` and `.isoprism/objects/index/blob_to_nodes` cache parsed semantic nodes by git blob SHA.
- `--rebuild-cache` removes parser-derived objects without deleting annotations.
- `serve` binds to `127.0.0.1:3717` by default and serves the local viewer at `/local`.
- `serve` does not require Node.js or the Isoprism source checkout.
- `--web-dir` and `ISOPRISM_WEB_DIR` are frontend-development overrides.
- `annotate` writes JSON under `.isoprism/annotations/<base-sha>..<head-sha>/`.

Intentional gaps:

- `diff unstaged` is not implemented.
- Static diff HTML is readable and self-contained, but not yet visually identical to the hosted React graph UI.
- `serve` does not yet watch files or push SSE/WebSocket refreshes.
- Incoming link indexes are not persisted yet; context is computed from active graphs.

## Semantic Graph Model

The canonical graph is function/type-level. The parser supports Go, TypeScript, TSX, JavaScript, and JSX. Rust and Python are not currently indexed.

Primary node kinds include functions, methods, structs/types, interfaces, and tests. Test code is stored as first-class nodes but default repo and PR graph responses filter tests out of the production graph. PR graph responses return changed tests separately in `test_changes[]`.

Primary edge kinds:

- `calls`: semantic call relationship.
- `owns_method`: Go receiver type to method.
- `uses_type`: struct/type dependency from fields or references.
- `imports` and `references`: available for local CLI object links and future language relationships.

Resolution is conservative. Unknown external calls, ambiguous suffix matches, selector-name-only matches, unknown receiver types, and unresolved field chains should be omitted instead of guessed.

## PR Graph Behavior

PR graphs seed from changed production nodes and initially load direct semantic context. The UI groups PR overview changes after the PR description:

1. Graph changes: changed production functions/types represented in the graph.
2. Test changes: changed test entrypoints from `test_changes[]`.
3. Documentation changes: Markdown files from GitHub file diffs.
4. Other changes: remaining file diffs not represented by graph, tests, or docs.

GitHub PR files are the source of truth for file-level diffs, additions, deletions, docs, config, generated files, package metadata, and other non-node changes. Semantic changes are stored separately in `pr_node_changes`.

Rename detection is intentionally conservative. Pair by rename metadata or matching body hash. Line overlap alone must not create a rename.

Type/class nodes initially show a `Contents` panel: owned methods first, used field types next, then type references from inputs/outputs. Contents materialize into the graph only when selected from the panel.

## AI Contract

AI is PR-only. The frontend must not call model providers directly.

| Area | Contract |
| --- | --- |
| Provider | Google Gemini API |
| Model | `gemini-2.5-flash` |
| Entrypoint | `api/internal/ai/enricher.go` |
| Runtime config | `GEMINI_API_KEY` |
| Output mode | JSON object |

The model receives PR title/body, changed production component diffs, test diffs, and relevant non-code changed files. It returns:

```json
{
  "changes": [{"full_name": "...", "change_summary": "..."}],
  "test_assertions": [{"name": "...", "assertion_summary": "..."}],
  "pr_summary": "...",
  "risk_score": 5
}
```

There is no default fallback model. Base repository summaries, embeddings, semantic search, and chat/review agents are out of scope unless explicitly added.

Use graph-only reprocessing when summaries should remain unchanged:

```http
POST /debug/prs/{prID}/reprocess/graph
```

Use AI-only reprocessing when graph data is already correct:

```http
POST /debug/prs/{prID}/reprocess/ai
```

Use full reprocess only when both graph and AI should refresh:

```http
POST /debug/prs/{prID}/reprocess
```

## Hosted Prototype Architecture

| Layer | Current service |
| --- | --- |
| Frontend | Next.js App Router in `web/`, deployed to Vercel at `https://isoprism.com` |
| API | Go chi API in `api/`, deployed to Railway at `https://api.isoprism.com` |
| DB/Auth | Supabase Postgres and Auth |
| GitHub | Single production GitHub App |
| Graph UI | React Flow (`@xyflow/react`) |

Local web development:

```bash
cd web
NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev
```

The Railway API must keep:

```text
FRONTEND_URL=https://isoprism.com
FRONTEND_URLS=https://isoprism.com,http://localhost:3000
```

## Repo Maintenance Rules

- Work on `main` for now; do not create feature branches or PRs unless the user explicitly changes this.
- Keep `preview` only as a synced mirror while external tooling expects it.
- Keep documentation updated with code changes.
- Do not worry about backwards compatibility or legacy code in this prototype.
- For API migrations, apply the migration before deploying API code and update `api/internal/db.RequiredMigrationVersion`.
- Tree-sitter grammar bindings require CGO and a C compiler. Do not force `CGO_ENABLED=0` for API builds.
- Local web uses the hosted API and production GitHub App by default. That means local UI testing can touch production GitHub/Supabase data.

## Useful API Routes

Public and debug:

```http
GET  /api/v1/auth/status
GET  /api/v1/github/callback
POST /webhooks/github
POST /debug/repos/{repoID}/reindex
POST /debug/prs/{prID}/reprocess
POST /debug/prs/{prID}/reprocess/graph
POST /debug/prs/{prID}/reprocess/ai
```

Authenticated repo and graph routes:

```http
GET  /api/v1/me/repos
GET  /api/v1/repos/{repoID}/status
GET  /api/v1/repos/{repoID}/queue
GET  /api/v1/repos/{repoID}/programs
GET  /api/v1/repos/{repoID}/programs/{nodeID}/graph
POST /api/v1/repos/{repoID}/graph/expand
GET  /api/v1/repos/{repoID}/prs/{prID}/graph
GET  /api/v1/repos/{repoID}/prs/number/{number}/graph
GET  /api/v1/repos/{repoID}/nodes/{nodeID}/code
GET  /api/v1/repos/{repoID}/prs/{prID}/nodes/{nodeID}/code
```

Local CLI server routes include:

```http
GET  /
GET  /api/diff
GET  /api/v1/local/repo
GET  /api/v1/repos/local/queue
GET  /api/v1/repos/local/programs
GET  /api/v1/repos/local/programs/{nodeID}/graph
POST /api/v1/repos/local/graph/expand
GET  /api/v1/repos/local/nodes/{nodeID}/code
```
