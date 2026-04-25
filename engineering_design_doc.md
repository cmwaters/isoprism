# Isoprism — Engineering Design Document

> Status: v0.7 | Author: Callum | Updated: 2026-04-21

---

## Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Data Model](#3-data-model)
4. [GitHub App Integration](#4-github-app-integration)
5. [Backend Events](#5-backend-events)
6. [Backend API](#6-backend-api)
7. [Frontend](#7-frontend)
8. [Auth & Security](#8-auth--security)
9. [Deployment](#9-deployment)
10. [Implementation Plan](#10-implementation-plan)
11. [Technical Decisions](#11-technical-decisions)
12. [Open Questions](#12-open-questions)
13. [Iteration Log](#13-iteration-log)

---

## 1. Overview

Isoprism is a validation prototype testing the hypothesis that a **graph representation of software changes** is faster and more effective than reading code diffs.

The prototype delivers exactly one flow:

1. User signs in with GitHub and selects a single repository
2. The backend indexes the repo, building a base code graph from the `main` branch HEAD
3. The user sees the top five open PRs ranked by urgency
4. The user selects a PR and sees an interactive graph where each node is a function, method, or type affected by the PR — with its signature, a plain-English summary of what it does, and a summary of what changed

**Target scale:** Single-user prototype. No multi-tenancy, no teams, no billing. Architecture is deliberately simple.

---

## 2. Architecture

### Tech Stack

| Layer | Technology | Hosting |
|---|---|---|
| Frontend | Next.js (App Router) + TypeScript + Tailwind CSS + shadcn/ui | Vercel |
| Backend API | Go (chi router) | Railway |
| Database | Supabase (Postgres 15) | Supabase |
| Auth | Supabase Auth (GitHub OAuth) | Supabase |
| AI | Anthropic Claude (`claude-sonnet-4-6`) | Anthropic API |
| GitHub Integration | GitHub App (webhooks + REST API v3) | — |
| Code Parsing | tree-sitter (`go-tree-sitter`) | In-process |
| Graph Rendering | React Flow (`@xyflow/react`) | Frontend |

### System Diagram

```
                        ┌─────────────────────────────────────────┐
                        │               User's Browser             │
                        │        Next.js (Vercel)                  │
                        └──────────────┬──────────────────────────┘
                                       │ REST / fetch
                        ┌──────────────▼──────────────────────────┐
                        │         Go API Server (Railway)          │
                        │                                          │
                        │  ┌─────────────┐  ┌──────────────────┐  │
                        │  │  REST API   │  │  Webhook Handler  │  │
                        │  │  /api/v1/…  │  │  /webhooks/github │  │
                        │  └──────┬──────┘  └────────┬─────────┘  │
                        │         │                  │             │
                        │  ┌──────▼──────────────────▼──────────┐ │
                        │  │  Event Handlers                     │ │
                        │  │  RepoInit | OpenPR | MergePR        │ │
                        │  │                                     │ │
                        │  │  tree-sitter → call graph →         │ │
                        │  │  Claude enrichment                  │ │
                        │  └──────────────────┬─────────────────┘ │
                        └──────────────────────┼──────────────────┘
                                               │ pgx
                        ┌──────────────────────▼──────────────────┐
                        │           Supabase (Postgres 15)         │
                        │   repositories | pull_requests |         │
                        │   code_nodes | code_edges |              │
                        │   pr_node_changes                        │
                        └──────────────────────▲──────────────────┘
                                               │ webhooks
                        ┌──────────────────────┴──────────────────┐
                        │              GitHub                      │
                        │   GitHub App webhooks + REST API v3      │
                        └─────────────────────────────────────────┘
```

### Monorepo Layout

```
/api          Go backend (chi router, handlers, event handlers, parser, AI)
/web          Next.js frontend (App Router)
/db
  /migrations Plain SQL migration files
```

---

## 3. Data Model

### Tables

```sql
-- Users (mirrors auth.users; populated by trigger on sign-in)
users
  id               uuid PK          -- matches auth.users.id
  email            text
  display_name     text
  avatar_url       text
  github_user_id   bigint UNIQUE
  github_username  text
  created_at       timestamptz

-- GitHub App installations
github_installations
  id                  uuid PK
  installation_id     bigint UNIQUE   -- GitHub's numeric installation ID
  account_login       text
  account_type        text            -- 'Organization' | 'User'
  account_avatar_url  text
  created_at          timestamptz

-- Repositories
repositories
  id                  uuid PK
  user_id             uuid FK → users
  installation_id     uuid FK → github_installations
  github_repo_id      bigint
  full_name           text            -- e.g. "acme/backend"
  default_branch      text DEFAULT 'main'
  main_commit_sha     text            -- HEAD of main as of last RepoInit or MergePR
  index_status        text DEFAULT 'pending'  -- 'pending' | 'running' | 'ready' | 'failed'
  is_active           boolean DEFAULT true
  created_at          timestamptz
  UNIQUE (user_id, github_repo_id)

-- Pull requests
pull_requests
  id                  uuid PK
  repo_id             uuid FK → repositories
  github_pr_id        bigint
  number              int
  title               text
  body                text
  author_login        text
  author_avatar_url   text
  base_branch         text
  head_branch         text
  base_commit_sha     text            -- SHA of main at time PR was opened/last rebased
  head_commit_sha     text            -- current HEAD of the PR branch; updated on synchronize
  state               text            -- 'open' | 'closed' | 'merged'
  draft               boolean DEFAULT false
  html_url            text
  opened_at           timestamptz
  merged_at           timestamptz
  last_activity_at    timestamptz
  graph_status        text DEFAULT 'pending'  -- 'pending' | 'running' | 'ready' | 'failed'
  created_at          timestamptz
  UNIQUE (repo_id, github_pr_id)

-- ────────────────────────────────────────────
-- Base code graph: nodes keyed by (repo, commit)
-- ────────────────────────────────────────────

-- Code nodes: one row per function/method/type at a given commit SHA.
-- A node at commit A and the same node at commit B are separate rows.
-- Equality across commits is determined by (repo_id, full_name, file_path).
code_nodes
  id               uuid PK
  repo_id          uuid FK → repositories
  commit_sha       text
  name             text              -- bare name, e.g. "handleAuth"
  full_name        text              -- qualified name, e.g. "AuthService.handleAuth"
  file_path        text              -- relative path within repo
  line_start       int
  line_end         int
  signature        text              -- full signature string
  language         text              -- 'go' | 'typescript' | 'rust' | 'python'
  kind             text              -- 'function' | 'method' | 'type' | 'struct' | 'interface'
  body_hash        text              -- SHA-256 of the node body; used for change detection
  summary          text              -- AI: what this node does (2 sentences)
  created_at       timestamptz
  UNIQUE (repo_id, commit_sha, full_name, file_path)

-- Call/reference edges at a given commit
code_edges
  id               uuid PK
  repo_id          uuid FK → repositories
  commit_sha       text
  caller_id        uuid FK → code_nodes
  callee_id        uuid FK → code_nodes
  created_at       timestamptz
  UNIQUE (repo_id, commit_sha, caller_id, callee_id)

-- ────────────────────────────────────────────
-- PR delta: what changed and AI change summaries
-- ────────────────────────────────────────────

-- One row per node that was added, modified, or deleted in a PR.
-- References the HEAD-commit version of the node in code_nodes.
-- node_type (changed | caller | callee) is NOT stored here; it is derived
-- at query time by traversing code_edges from the changed set.
pr_node_changes
  id               uuid PK
  pull_request_id  uuid FK → pull_requests
  node_id          uuid FK → code_nodes   -- the HEAD-commit version of this node
  change_type      text                   -- 'added' | 'modified' | 'deleted'
  change_summary   text                   -- AI: what changed in this node (2 sentences)
  diff_hunk        text                   -- unified diff for this node
  created_at       timestamptz
  UNIQUE (pull_request_id, node_id)

-- PR-level summary (for queue display and urgency scoring)
pr_analyses
  id               uuid PK
  pull_request_id  uuid FK → pull_requests UNIQUE
  summary          text              -- one-line summary for queue display
  nodes_changed    int               -- count of directly changed nodes
  risk_score       int               -- 1–10
  risk_label       text              -- 'low' | 'medium' | 'high'
  ai_model         text
  generated_at     timestamptz
  created_at       timestamptz
```

### Design Notes

**`code_nodes` is a content-addressed snapshot store.** The same function at two different commits is two rows. The `body_hash` field enables change detection between commits without re-parsing: if `body_hash` is identical across commits, the node is unchanged and its existing `summary` can be reused without calling Claude again.

**`node_type` is not stored.** Whether a node is `changed`, `caller`, or `callee` is a property of its relationship to a specific PR, not of the node itself. The API computes this at query time by:
1. Looking up the set of `pr_node_changes` for the PR → these are `changed` nodes
2. Traversing one hop of `code_edges` from that set → callers and callees

**Separate base graph and PR delta.** `code_nodes` + `code_edges` are the base graph, built during `RepoInit` and kept current by `MergePR`. `pr_node_changes` is the PR-specific overlay, built during `OpenPR`.

---

## 4. GitHub App Integration

### Permissions Required

| Permission | Access | Reason |
|---|---|---|
| Contents | Read | Fetch file contents for tree-sitter parsing |
| Pull requests | Read | Fetch PR diffs and metadata |
| Metadata | Read | Repo info |

### Webhook Events

| Event | Actions | Handler |
|---|---|---|
| `pull_request` | opened, synchronize | `OpenPR` |
| `pull_request` | closed (merged) | `MergePR` |
| `installation` | deleted | Cleanup |
| `installation_repositories` | added, removed | Repo sync |

All webhooks verified with `X-Hub-Signature-256` HMAC before processing.

### Installation Flow

`GET /api/v1/github/callback` receives `installation_id`, `setup_action`, and `state` (user's Supabase UUID).

| `setup_action` | Behaviour |
|---|---|
| `install` | Creates installation + repository records; redirects to `/onboarding/repos` |
| `update` | Re-syncs repo list; redirects to queue |
| `request` | User lacks permission; redirects to `/request-sent` |

### Token Management

- **App JWT**: RS256-signed, 9-min expiry — used to fetch installation tokens
- **Installation tokens**: 1-hour expiry, fetched from `POST /app/installations/{id}/access_tokens`, cached in memory and refreshed on demand

---

## 5. Backend Events

The backend is defined around three events. All other logic flows from them.

---

### Event 1 — `RepoInit`

**Trigger:** `POST /api/v1/repos/{repoID}/index` (called by frontend after repo selection)

**Purpose:** Build the base code graph for the repository from the current HEAD of `main`.

**Steps:**

1. Fetch the current HEAD commit SHA of `main` via `GET /repos/{owner}/{repo}/git/ref/heads/main`
2. Fetch the full file tree at that SHA via `GET /repos/{owner}/{repo}/git/trees/{sha}?recursive=1`
3. Filter to supported source files (`.go`, `.ts`, `.tsx`, `.js`, `.rs`, `.py`)
4. For each file, fetch content via `GET /repos/{owner}/{repo}/contents/{path}?ref={sha}` and parse with tree-sitter to extract all `code_nodes` (functions, methods, types)
5. For each parsed node, build call/reference edges by resolving identifiers in function bodies against the full node set → insert `code_edges`
6. Generate AI summaries for all nodes in a single batched Claude call (see §Parsing below)
7. Persist all `code_nodes` and `code_edges` with `commit_sha = HEAD`
8. Set `repositories.main_commit_sha = HEAD` and `repositories.index_status = 'ready'`

Files are processed concurrently (bounded goroutine pool, max 10 in-flight). The frontend polls `GET /api/v1/repos/{repoID}/status` (returns `index_status`) every 2 seconds until `ready` or `failed`.

---

### Event 2 — `OpenPR`

**Trigger:** `pull_request` webhook with action `opened` or `synchronize`

**Purpose:** Compute which nodes changed between `main` and the PR's head commit, and generate change summaries.

**Steps:**

1. **Check cache:** Look up the PR's stored `head_commit_sha`. If the incoming webhook's `head_sha` matches → already processed, skip.
2. **Fetch diff:** `GET /repos/{owner}/{repo}/compare/{base_commit}...{head_sha}` — returns a list of changed files with their unified diffs.
3. **Parse head commit:** For each changed file at `head_sha`, fetch content and parse with tree-sitter. Insert new `code_nodes` at `commit_sha = head_sha` if not already present. Reuse existing `summary` where `body_hash` is unchanged.
4. **Identify changed nodes:** Compare `body_hash` for each node between `base_commit_sha` and `head_sha`. Nodes with a differing hash are `modified`; nodes present only at head are `added`; nodes present only at base are `deleted`.
5. **Generate change summaries:** For all `modified` and `added` nodes, call Claude with the diff hunk and new function body to generate `change_summary`. Batch into a single API call.
6. **Persist:** Insert `pr_node_changes` rows. Insert/update `pr_analyses` (summary, `nodes_changed`, `risk_score`).
7. **Update PR:** Set `pull_requests.head_commit_sha = head_sha` and `graph_status = 'ready'`.

---

### Event 3 — `MergePR`

**Trigger:** `pull_request` webhook with action `closed` and `merged = true`

**Purpose:** Advance the base code graph to the merge commit.

**Steps:**

1. Fetch the merge commit SHA from the webhook payload (`pull_request.merge_commit_sha`).
2. The merge commit's `code_nodes` may already exist (if the PR head was fast-forwarded). If not, run the same parse-and-insert flow as `RepoInit` Stage 3–6 but scoped to the files changed in the PR.
3. Set `repositories.main_commit_sha = merge_commit_sha`.
4. Mark the PR as `state = 'merged'`.

This keeps the base graph current without re-indexing the entire repo on every merge.

---

### Parsing with tree-sitter

All code parsing uses `github.com/smacker/go-tree-sitter` — a single Go dependency that wraps the tree-sitter C library with language grammars for Go, TypeScript, Rust, and Python.

**Language dispatch:**

```go
func grammarForLanguage(lang string) *sitter.Language {
    switch lang {
    case "go":         return golang.GetLanguage()
    case "typescript": return typescript.GetLanguage()
    case "rust":       return rust.GetLanguage()
    case "python":     return python.GetLanguage()
    }
    return nil
}
```

**Per-language tree-sitter queries** extract function/method/type nodes:

| Language | Query pattern |
|---|---|
| Go | `(function_declaration)`, `(method_declaration)`, `(type_spec)` |
| TypeScript | `(function_declaration)`, `(method_definition)`, `(arrow_function)`, `(interface_declaration)`, `(type_alias_declaration)` |
| Rust | `(function_item)`, `(impl_item)`, `(struct_item)`, `(trait_item)` |
| Python | `(function_definition)`, `(class_definition)` |

The same tree-sitter walk is used for both full-file indexing (RepoInit) and changed-file parsing (OpenPR). There are no language-specific code paths beyond the query strings above.

**Call graph extraction:** After extracting node boundaries, each function body is walked for `call_expression` (TS/Python), `call_expr` (Rust), or `call_expression` / `selector_expression` (Go) nodes. Resolved callee names are matched against the full node set for the same commit. Unresolvable names (stdlib, external packages) are discarded.

---

## 6. Backend API

### File Structure

```
api/
  cmd/api/main.go
  internal/
    api/
      router.go
      handlers/
        github.go          installation callback, webhooks → event dispatch
        repos.go           repo list, status, index trigger
        queue.go           queue endpoint + urgency scoring
        graph.go           PR graph endpoint
    events/
      repo_init.go         RepoInit handler
      open_pr.go           OpenPR handler
      merge_pr.go          MergePR handler
    parser/
      parse.go             tree-sitter dispatch + node extraction
      callgraph.go         call edge resolution
    ai/
      enricher.go          Claude batched enrichment
    github/
      app.go               JWT generation, installation token cache
      client.go            GitHub REST API wrapper
      webhook.go           signature verification, payload types
    models/
      types.go
    config/
      config.go
```

### Routes

**Public**
```
POST /webhooks/github
GET  /api/v1/github/callback
GET  /api/v1/auth/status
```

**Authenticated**
```
GET    /api/v1/me/repos                                   list repos for current user
DELETE /api/v1/me

GET    /api/v1/repos/{repoID}                             repo detail + index_status
POST   /api/v1/repos/{repoID}/index                       trigger RepoInit
GET    /api/v1/repos/{repoID}/status                      {index_status, pr_count, ready_count}
GET    /api/v1/repos/{repoID}/queue                       top 5 PRs by urgency
GET    /api/v1/repos/{repoID}/prs/{prID}/graph            PR graph (nodes + edges + deltas)
```

### `GET /repos/{repoID}/queue` Response

```json
{
  "prs": [
    {
      "id": "...",
      "number": 42,
      "title": "...",
      "html_url": "...",
      "author_login": "...",
      "author_avatar_url": "...",
      "opened_at": "...",
      "graph_status": "ready",
      "summary": "Replaces the token validation logic with a configurable grace period.",
      "nodes_changed": 4,
      "risk_score": 6,
      "risk_label": "medium",
      "urgency_score": 0.72
    }
  ]
}
```

### `GET /repos/{repoID}/prs/{prID}/graph` Response

`node_type` is computed here, not stored.

```json
{
  "pr": {
    "id": "...",
    "number": 42,
    "title": "...",
    "html_url": "...",
    "base_commit_sha": "abc123",
    "head_commit_sha": "def456"
  },
  "nodes": [
    {
      "id": "...",
      "name": "handleAuth",
      "full_name": "AuthService.handleAuth",
      "file_path": "internal/auth/service.go",
      "line_start": 42,
      "line_end": 78,
      "signature": "func (s *AuthService) handleAuth(ctx context.Context, token string) (*User, error)",
      "language": "go",
      "kind": "method",
      "node_type": "changed",
      "summary": "Validates a JWT token and returns the associated user. Returns an error if the token is expired or malformed.",
      "change_summary": "Now validates token expiry against a configurable grace period instead of hard-coding 5 minutes. Adds a new error type for expired tokens.",
      "diff_hunk": "@@ -42,10 +42,14 @@ ..."
    },
    {
      "id": "...",
      "name": "middleware",
      "full_name": "authMiddleware",
      "file_path": "internal/api/middleware.go",
      "line_start": 12,
      "line_end": 34,
      "signature": "func authMiddleware(next http.Handler) http.Handler",
      "language": "go",
      "kind": "function",
      "node_type": "caller",
      "summary": "HTTP middleware that extracts the Bearer token from the Authorization header and calls handleAuth.",
      "change_summary": null,
      "diff_hunk": null
    }
  ],
  "edges": [
    {
      "caller_id": "...",
      "callee_id": "..."
    }
  ]
}
```

### Queue Urgency Scoring

```
urgency = (wait_time_score × 0.4) + (risk_score × 0.35) + (nodes_changed_score × 0.25)
```

Computed at query time from `pr_analyses`. PRs with `graph_status != 'ready'` are excluded from the queue until processing completes.

---

## 7. Frontend

### Route Structure

```
/login                           GitHub OAuth sign-in
/auth/callback                   Supabase auth callback
/onboarding                      GitHub App install (first-time)
/onboarding/repos                Repo selection → triggers RepoInit
/                                Root: redirect to repo queue or /onboarding
/repos/[repoID]                  PR Queue (top 5 PRs)
/repos/[repoID]/pr/[prID]        PR Graph View
/settings                        Repo management, delete account
```

### Key Components

| Component | Location | Notes |
|---|---|---|
| `GraphCanvas` | `components/graph/graph-canvas.tsx` | React Flow canvas: pan, zoom, click |
| `GraphNode` | `components/graph/graph-node.tsx` | Custom React Flow node: badge, name, summary |
| `NodeDetailPanel` | `components/graph/node-detail-panel.tsx` | Side panel updating on node selection; includes a top-left back control that returns to the PR overview and renders PR descriptions as GitHub-flavored Markdown |
| `DiffBlock` | `components/graph/diff-block.tsx` | Unified diff with line highlighting |
| `IndexingProgress` | `components/onboarding/indexing-progress.tsx` | Animated bar; polls `/status` |
| `PRQueue` | `components/queue/pr-queue.tsx` | List of 5 PR cards |

### Graph Rendering

React Flow (`@xyflow/react`) with a concentric ring layout:
- Changed nodes anchor the centre; a few changed nodes with surrounding context are placed in a tight centre row
- BFS over graph edges assigns surrounding caller/callee nodes to outer rings
- Node `kind` drives the card colour; `node_type` drives central placement and changed-node diff pills
- Edges use a custom smart Bezier renderer that connects the closest points on each card border and makes the curve leave and enter perpendicular to those borders
- `onNodeClick` updates `selectedNodeId` state; `NodeDetailPanel` reads from it, and its back control clears the selection to return to the PR summary
- Maximum 20 nodes rendered; excess nodes shown as a count notice

### Data Fetching

- **PR Queue page**: Server Component; fetches queue from Go API on every request. Manual refresh via `router.refresh()`.
- **PR Graph page**: Server Component fetches `/prs/{prID}/graph`; passes data as props to client-side `GraphCanvas`.
- **PR description markdown**: `NodeDetailPanel` renders `GraphPR.body` with `react-markdown` and `remark-gfm`; HTML is not enabled, and links open in a new tab.
- **Indexing status**: Client Component polls `GET /repos/{repoID}/status` every 2 seconds until `index_status = 'ready'`.

---

## 8. Auth & Security

### Sign-in Flow

1. `/login` → Supabase Auth GitHub OAuth
2. GitHub → `/auth/callback` → Supabase session
3. Callback calls `GET /api/v1/auth/status?github_token=…&user_id=…`
4. API checks if user's GitHub account matches an existing installation:
   - **Match**: redirect to `/repos/{repoID}`
   - **No match**: redirect to `/onboarding`

### Isolation

- All API queries are scoped by `user_id` (single-user prototype; no org-level isolation needed)
- Supabase service role key used only server-side in Go; never sent to client
- Webhook payloads verified with HMAC before processing

---

## 9. Deployment

| Service | Platform | Notes |
|---|---|---|
| Next.js frontend | Vercel | Auto-deploys from `main`; root set to `/web` |
| Go API | Railway | Dockerfile in `/api`; single service, 512 MB RAM sufficient |
| Postgres + Auth | Supabase | Managed; free tier sufficient |
| GitHub App | GitHub | Webhook URL → Railway service URL |

### Environment Variables

**Go API (Railway)**
```
SUPABASE_URL
SUPABASE_SERVICE_ROLE_KEY
GITHUB_APP_ID
GITHUB_APP_PRIVATE_KEY       PEM; literal \n sequences normalised on load
GITHUB_WEBHOOK_SECRET
ANTHROPIC_API_KEY
FRONTEND_URL
```

**Next.js (Vercel)**
```
NEXT_PUBLIC_SUPABASE_URL
NEXT_PUBLIC_SUPABASE_ANON_KEY
NEXT_PUBLIC_API_URL            Railway Go API base URL
NEXT_PUBLIC_GITHUB_APP_NAME    Used to construct GitHub install links
```

### Migrations

Plain SQL files in `/db/migrations/`, applied via Supabase dashboard or CLI. No migration framework — manual sequencing is sufficient at this scale.

---

## 10. Implementation Plan

### Phase 1 — Auth, Repo Selection, and DB Foundation *(~1 day)*

1. Write and apply new DB migration (clean schema: `users`, `github_installations`, `repositories`, `pull_requests`, `code_nodes`, `code_edges`, `pr_node_changes`, `pr_analyses`)
2. Migrate Railway service: update `fly.toml` → `railway.json` / `Dockerfile`; confirm env vars set
3. Strip all old multi-tenant and org-based code from the backend and frontend
4. Simplify auth flow: on sign-in, upsert `users` record; check for existing `repositories` and redirect accordingly
5. Update `/onboarding/repos`: fetch user's GitHub repos, single-select, submit → `POST /repos/{repoID}/index`

---

### Phase 2 — tree-sitter Parsing *(~1.5 days)*

1. Add `github.com/smacker/go-tree-sitter` dependency; add language grammars (Go, TypeScript, Rust, Python) to `go.mod`
2. Implement `parser.Parse(content []byte, language string) []CodeNode` — runs tree-sitter query, returns node boundaries + signatures
3. Implement `parser.ExtractCallEdges(content []byte, language string, nodeSet map[string]uuid) []CallEdge` — walks call expressions, resolves against nodeSet
4. Write unit tests for each language grammar with fixture source files
5. Implement `parser.BodyHash(content []byte, start, end int) string` — SHA-256 of the function body bytes

---

### Phase 3 — RepoInit Event *(~1.5 days)*

1. Implement `events.RepoInit(ctx, repoID)`:
   - Fetch file tree from GitHub API
   - Spawn bounded goroutine pool (max 10); fetch + parse each source file
   - Insert `code_nodes` + `code_edges` at `commit_sha = HEAD`
   - Set `repositories.index_status = 'running'` on start, `'ready'` on completion
2. Wire `POST /repos/{repoID}/index` handler to dispatch `RepoInit` as a goroutine
3. Implement `GET /repos/{repoID}/status` → `{index_status, pr_count, ready_count}`
4. Build `IndexingProgress` frontend component; poll status until ready

---

### Phase 4 — AI Enrichment *(~1 day)*

1. Implement `ai.EnrichNodes(ctx, nodes []CodeNode, client *anthropic.Client) error`:
   - Batch all nodes for a single repo/PR into one Claude API call
   - Structured JSON output: array of `{full_name, summary}` (and `{change_summary}` for PR enrichment)
   - Map responses back to nodes by `full_name`; update `code_nodes.summary`
2. Implement `ai.EnrichPRChanges(ctx, changes []PRNodeChange, client) error`:
   - Same batched call pattern; generates `change_summary` for each changed node
   - Also generates `pr_analyses.summary` and `risk_score` in the same call
3. Integrate enrichment into `RepoInit` (after node insertion) and `OpenPR` (after change detection)

---

### Phase 5 — OpenPR and MergePR Events *(~1.5 days)*

1. Implement `events.OpenPR(ctx, pr PullRequest)`:
   - Check `pull_requests.head_commit_sha`; skip if unchanged
   - Fetch diff via GitHub compare API
   - Parse changed files at `head_sha`; insert new `code_nodes` (reuse summary where `body_hash` matches)
   - Diff `body_hash` between base and head; populate `pr_node_changes`
   - Call AI enrichment for changed nodes
   - Update `pull_requests.graph_status = 'ready'`
2. Implement `events.MergePR(ctx, pr PullRequest)`:
   - Parse merge commit if `code_nodes` not already present at that SHA
   - Update `repositories.main_commit_sha`
3. Wire webhook handler to dispatch `OpenPR` / `MergePR` based on action
4. On install callback: for each open PR in the repo, dispatch `OpenPR` to backfill

---

### Phase 6 — Graph API *(~0.5 days)*

1. Implement `GET /repos/{repoID}/prs/{prID}/graph`:
   - Query `pr_node_changes` → changed node IDs
   - Traverse `code_edges` one hop from changed nodes → caller and callee IDs
   - Fetch all node records; tag each with computed `node_type`
   - Cap at 20 nodes: keep all changed nodes, fill remaining slots by proximity
   - Serialise to graph JSON response (see §6)
2. Update queue handler to include `nodes_changed` and `risk_label` from `pr_analyses`

---

### Phase 7 — Frontend Graph View *(~2 days)*

1. Install `@xyflow/react` and `dagre`
2. Implement `GraphCanvas`: transform API response to React Flow nodes/edges, apply dagre layout, wire `onNodeClick`
3. Implement `GraphNode` custom component: node type badge, name, truncated summary, border colour by `node_type`
4. Implement `NodeDetailPanel`: signature block, "What it does", "What changed", diff toggle, caller/callee chips
5. Implement `DiffBlock`: parse unified diff string, render with green/red line backgrounds
6. Implement `TopBar`: PR breadcrumb and "View on GitHub →" link
7. Update `/repos/[repoID]/pr/[prID]` page to use new graph layout
8. Update `PRCard` in queue: show "N functions changed" badge; show spinner if `graph_status != 'ready'`

---

### Phase 8 — Validation Pass *(~1 day)*

1. Run the full flow against 5–10 real PRs from at least two different repos and languages
2. Check: graph size reasonable (≤20 nodes), summaries accurate, change summaries specific, call edges not overloaded with false positives
3. Adjust tree-sitter call graph resolution if false positive rate is high (e.g. add file-scoped name filtering)
4. Adjust AI prompt if summaries are too vague or too verbose

---

## 11. Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Railway over Fly.io | Railway | Simpler deploy config; no fly.toml; Railway detects Dockerfile automatically |
| tree-sitter for all languages | `go-tree-sitter` | Single dependency, uniform API, production-quality grammars. No per-language special cases. |
| `node_type` computed at query time | Not stored | It is a PR-relative property, not a property of a node. Storing it would couple the base graph to PR state. |
| `code_nodes` keyed by `(repo, commit_sha, full_name, file_path)` | Snapshot per commit | Allows change detection by `body_hash` comparison without storing diffs. Summaries are reused across commits when body is unchanged. |
| One-hop graph depth | Depth 1 from changed nodes | Deeper traversal produces unreadable graphs. One hop gives immediate structural context. |
| Batched AI calls per PR | Single Claude call | One round-trip covers the entire changed node set. Keeps latency low and cost minimal (~$0.002/PR). |
| No message queue | Goroutines | Sufficient at prototype scale. A queue adds operational complexity with no benefit yet. |

---

## 12. Open Questions

- **Call graph false positives:** Name-based resolution will collide when the same function name exists in multiple packages. How much does this affect graph quality? Measure in Phase 8; add package-qualified matching if needed.
- **Large repos:** Fetching every file via the GitHub Contents API for `RepoInit` will be slow and rate-limited for repos with thousands of files. Consider a shallow clone via `git archive` if this becomes a problem.
- **Merge commits:** Some repos use squash-merge or rebase-merge, so the merge SHA may differ significantly from the PR head SHA. `MergePR` handles this but will trigger a partial re-parse.
- **Body hash granularity:** Hashing the raw bytes of a function body means whitespace-only changes generate a new hash and trigger AI re-enrichment. Consider normalising whitespace before hashing.

---

## 13. Iteration Log

| Version | Date | Changes |
|---|---|---|
| v0.1 | 2026-03-22 | Initial design document |
| v0.2 | 2026-03-22 | Auth, onboarding, repo connection, queue |
| v0.3 | 2026-03-24 | PR detail page; settings page |
| v0.4 | 2026-03-25 | GitHub App reinstall flow; onboarding redirect fixes |
| v0.5 | 2026-03-26 | Queue refresh; `setup_action=request`; `installation_repositories` webhook |
| v0.6 | 2026-04-21 | Pivot to graph validation prototype; graph pipeline; function nodes; AI enrichment |
| v0.7 | 2026-04-21 | Railway; clean schema; tree-sitter single dependency; three-event backend model (RepoInit/OpenPR/MergePR); `code_nodes` keyed by commit SHA; `node_type` derived at query time |
