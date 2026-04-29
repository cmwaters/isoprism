# Isoprism — Engineering Design Document

> Status: v0.9 | Author: Callum | Updated: 2026-04-29

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

Isoprism is an invite-only validation prototype testing the hypothesis that a **graph representation of software changes** is faster and more effective than reading code diffs.

The prototype delivers exactly one flow:

1. Beta tester opens a unique invite link containing an access token
2. Tester signs in with GitHub and installs or authorizes the Isoprism GitHub App
3. Tester selects exactly one repository for a one-week trial
4. The backend indexes the repo, building a base code graph from the `main` branch HEAD
5. The tester uses Isoprism while reviewing PRs for one week, with in-product bug and feature feedback available throughout
6. At the end of the week, the tester is asked to complete a short questionnaire

**Target scale:** Invite-only beta prototype. Each tester has one selected repository and one seven-day trial window. No teams, billing, or open signup. Architecture is deliberately simple.

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
| Code Parsing | Go AST + lightweight TypeScript/JavaScript parser | In-process |
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
                        │  │  parser → call graph →              │ │
                        │  │  Claude enrichment                  │ │
                        │  └──────────────────┬─────────────────┘ │
                        └──────────────────────┼──────────────────┘
                                               │ pgx
                        ┌──────────────────────▼──────────────────┐
                        │           Supabase (Postgres 15)         │
                        │   repositories | pull_requests |         │
                        │   code_nodes | code_edges |              │
                        │   code_test_references | pr_node_changes │
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
  language         text              -- 'go' | 'typescript' | 'javascript'
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

-- Test references at a given commit. Test code is not inserted into
-- code_nodes/code_edges; these rows attach test entrypoints to production nodes.
code_test_references
  id               uuid PK
  repo_id          uuid FK → repositories
  commit_sha       text
  test_name        text
  test_full_name   text
  test_file_path   text
  test_line_start  int
  test_line_end    int
  target_node_id   uuid FK → code_nodes
  created_at       timestamptz
  UNIQUE (repo_id, commit_sha, test_full_name, test_file_path, target_node_id)

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

### Planned Beta Tables

The current production graph schema does not yet model the beta loop. To support the intended tester flow, add a small beta access layer:

```sql
-- One row per invite link.
beta_invites
  id                    uuid PK
  beta_id               text UNIQUE        -- short human/reference ID used in feedback issues
  name                  text               -- beta tester name entered by the admin
  token_hash            text UNIQUE        -- store a hash, never the raw URL token
  email                 text               -- optional operator note, not used for auth
  status                text DEFAULT 'new' -- 'new' | 'active' | 'completed' | 'revoked' | 'expired'
  invited_at            timestamptz
  expires_at            timestamptz
  accepted_at           timestamptz
  completed_at          timestamptz
  user_id               uuid FK -> users
  selected_repo_id      uuid FK -> repositories
  trial_starts_at       timestamptz
  trial_ends_at         timestamptz
  created_at            timestamptz

-- Bug reports and feature requests submitted during the trial.
beta_feedback
  id                    uuid PK
  invite_id             uuid FK -> beta_invites
  user_id               uuid FK -> users
  repo_id               uuid FK -> repositories
  pull_request_id       uuid FK -> pull_requests
  node_id               uuid FK -> code_nodes
  type                  text               -- 'bug' | 'feature'
  title                 text
  details               text
  browser_path          text
  created_at            timestamptz

-- One questionnaire response per beta invite.
beta_questionnaires
  id                    uuid PK
  invite_id             uuid FK -> beta_invites UNIQUE
  user_id               uuid FK -> users
  repo_id               uuid FK -> repositories
  faster_rating         int
  risk_clarity_rating   int
  confusing_or_missing  text
  bugs_hit              text
  build_next            text
  would_keep_using      text
  submitted_at          timestamptz
  created_at            timestamptz
```

The selected repository should be enforced at the product layer and, ideally, by API checks: once `selected_repo_id` is set for an active invite, the tester should not be able to index a second repository through normal UI/API paths.

### Planned Admin Console

The beta admin console should live at `/admin` and require an operator-only check before rendering data.

It should let an operator:

- Enter a beta tester by name
- Generate a unique `beta_id`
- Generate a raw invite token once and store only `token_hash`
- Copy the full invite link
- Monitor whether the tester has used the link
- See which repository the tester has selected
- See trial start/end status
- Review submitted questionnaire answers

The admin page should not expose raw tokens after creation. If a link is lost, the operator should revoke the old invite and generate a new one.

The admin API is password-gated with `ADMIN_PASSWORD`. Frontend requests send the password as `X-Admin-Password`; the API compares it server-side before listing or creating beta testers.

### Design Notes

**`code_nodes` is a content-addressed snapshot store.** The same function at two different commits is two rows. The `body_hash` field enables change detection between commits without re-parsing: if `body_hash` is identical across commits, the node is unchanged and its existing `summary` can be reused without calling Claude again.

**`node_type` is not stored.** Whether a node is `changed`, `caller`, or `callee` is a property of its relationship to a specific PR, not of the node itself. The API computes this at query time by:
1. Looking up the set of `pr_node_changes` for the PR → these are `changed` nodes
2. Traversing one hop of `code_edges` from that set → callers and callees

**Separate base graph and PR delta.** `code_nodes` + `code_edges` are the base graph, built during `RepoInit` and kept current by `MergePR`. `pr_node_changes` is the PR-specific overlay, built during `OpenPR`.

**Tests are metadata, not graph nodes.** Test code is excluded from `code_nodes` and `code_edges`. `code_test_references` records which test entrypoints exercise each production node so the UI can show tests in the node detail panel without cluttering the graph.

---

## 4. GitHub App Integration

### Permissions Required

| Permission | Access | Reason |
|---|---|---|
| Contents | Read | Fetch file contents for parsing |
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

For the beta flow, `state` must preserve both the authenticated user/session identity and the active beta invite context. After GitHub App install or update, the callback should return the tester to repo selection only when the invite is valid and not completed.

| `setup_action` | Behaviour |
|---|---|
| `install` | Creates installation + repository records; redirects to `/onboarding/repos` |
| `update` | Re-syncs repo list; redirects to `/onboarding/repos` |
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
3. Filter to supported source files (`.go`, `.ts`, `.tsx`, `.js`, `.jsx`)
4. For each file, fetch content via `GET /repos/{owner}/{repo}/contents/{path}?ref={sha}` and parse it to extract functions, methods, and types.
5. Exclude test code from `code_nodes`/`code_edges`, then build production call/reference edges by resolving identifiers in function bodies against the production node set → insert `code_edges`.
6. Extract test entrypoints and store the production nodes they exercise in `code_test_references`.
7. Generate AI summaries for production nodes in Claude batches of up to 30 nodes (see `ai.md`)
8. Persist all `code_nodes`, `code_edges`, and `code_test_references` with `commit_sha = HEAD`
9. Set `repositories.main_commit_sha = HEAD` and `repositories.index_status = 'ready'`

Files are processed concurrently (bounded goroutine pool, max 10 in-flight). The frontend polls `GET /api/v1/repos/{repoID}/status` (returns `index_status`) every 2 seconds until `ready` or `failed`.

---

### Event 2 — `OpenPR`

**Trigger:** `pull_request` webhook with action `opened` or `synchronize`

**Purpose:** Compute which nodes changed between `main` and the PR's head commit, and generate change summaries.

**Steps:**

1. **Check cache:** Look up the PR's stored `head_commit_sha`. If the incoming webhook's `head_sha` matches → already processed, skip.
2. **Fetch diff:** `GET /repos/{owner}/{repo}/compare/{base_commit}...{head_sha}` — returns a list of changed files with their unified diffs.
3. **Parse head commit:** For each changed file at `head_sha`, fetch content and parse it. Insert new production `code_nodes` at `commit_sha = head_sha` if not already present. Reuse existing `summary` where `body_hash` is unchanged.
4. **Identify changed nodes:** Compare `body_hash` for each node between `base_commit_sha` and `head_sha`. Nodes with a differing hash are `modified`; nodes present only at head are `added`; nodes present only at base are `deleted`.
5. **Build component diffs:** `modified` nodes keep a component-scoped slice of the GitHub patch. `added` and `deleted` nodes use synthetic component hunks where every source line is marked `+` or `-`, so semantic node stats count the whole new/removed component even when Git's file diff treats moved/copied body lines as unchanged context.
6. **Generate change summaries:** For all `modified` and `added` nodes, call Claude with the diff hunk and new function body to generate `change_summary`. Batch into a single API call.
7. **Persist:** Insert `pr_node_changes` rows, rebuild test references for changed test files, and insert/update `pr_analyses` (summary, `nodes_changed`, `risk_score`).
8. **Update PR:** Set `pull_requests.head_commit_sha = head_sha` and `graph_status = 'ready'`.

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

### Parsing

The current parser supports Go, TypeScript, and JavaScript.

| Language | Parser | Extracted production nodes | Test code handling |
|---|---|---|---|
| Go | Standard `go/parser` + `go/ast` | functions, methods, type specs, structs, interfaces | Excludes `*_test.go`, packages ending `_test`, and functions beginning `Test`; indexes `Test*` references to production nodes |
| TypeScript | Lightweight regex parser | function declarations, exported const arrow functions, class methods | Excludes `*.test.ts`, `*.spec.ts`, `*.test.tsx`, `*.spec.tsx`, and `__tests__/`; indexes `test(...)` / `it(...)` references |
| JavaScript | Lightweight regex parser | function declarations, exported const arrow functions, class methods | Excludes `*.test.js`, `*.spec.js`, `*.test.jsx`, `*.spec.jsx`, and `__tests__/`; indexes `test(...)` / `it(...)` references |

Rust and Python are not currently indexed.

**Call graph extraction:** After extracting production node boundaries, function bodies are walked for calls and resolved against the full production node set for the same commit. Unresolvable names (stdlib, external packages, or ambiguous methods) are discarded.

**Test reference extraction:** Test files are parsed separately from the production graph. Test functions/cases never become `code_nodes`; instead, each test entrypoint that reaches a production node writes a `code_test_references` row. The graph API attaches those rows to each returned production node so the detail panel can show callers, callees, and tests without rendering tests as graph nodes.

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
      parse.go             parser dispatch + node extraction
      callgraph.go         call edge and test-reference resolution
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
GET  /api/v1/beta/invites/{token}/status
```

**Authenticated**
```
POST   /api/v1/beta/invites/{token}/accept
GET    /api/v1/beta/trial
POST   /api/v1/beta/feedback
POST   /api/v1/beta/questionnaire

GET    /api/v1/admin/beta/testers
POST   /api/v1/admin/beta/testers
GET    /api/v1/admin/beta/testers/{betaID}

GET    /api/v1/me/repos                                   list repos for current user
DELETE /api/v1/me

GET    /api/v1/repos/{repoID}                             repo detail + index_status
POST   /api/v1/repos/{repoID}/index                       trigger RepoInit
GET    /api/v1/repos/{repoID}/status                      {index_status, pr_count, ready_count}
GET    /api/v1/repos/{repoID}/queue                       top 5 PRs by urgency
GET    /api/v1/repos/{repoID}/graph                       repo graph from main HEAD
GET    /api/v1/repos/{repoID}/nodes/{nodeID}/code         lazy repo node source
GET    /api/v1/repos/{repoID}/prs/{prID}/graph            PR graph (nodes + edges + deltas)
GET    /api/v1/repos/{repoID}/prs/number/{number}/graph   PR graph by GitHub PR number
GET    /api/v1/repos/{repoID}/prs/{prID}/nodes/{nodeID}/code lazy PR node source/diff
```

### Beta Access Rules

- A tester can only start from a valid beta invite token.
- GitHub OAuth must bind the invite to one `users.id`.
- GitHub App installation may expose several repositories, but Isoprism allows one `selected_repo_id` for the beta invite.
- `POST /api/v1/repos/{repoID}/index` should reject attempts to index a second repository for the same active beta invite.
- The trial starts when the tester selects the repository and indexing is triggered.
- The trial ends seven calendar days after `trial_starts_at`.
- Feedback submissions are accepted during the trial and may continue after the questionnaire is due if the product remains accessible.
- The questionnaire is due once `now() >= trial_ends_at` and should be stored once per invite.
- Feedback submissions should create GitHub issues in the configured feedback repository using labels `bug` or `feature`, and should include the tester's `beta_id`, selected repository, PR/node context, browser path, app commit SHA, and source commit SHA.

### Feedback Issue Configuration

Feedback issue creation uses:

```text
GITHUB_FEEDBACK_TOKEN
GITHUB_FEEDBACK_REPO
```

`GITHUB_FEEDBACK_REPO` should be an `owner/repo` slug for the repository where beta feedback issues are filed. `GITHUB_FEEDBACK_TOKEN` needs permission to create issues and apply the `bug` and `feature` labels in that repository.

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
/                                Root: auth/status redirect to repo queue, repo selection, or login
/{owner}/{repo}                  Single repo/PR graph workspace with in-place PR switching
/repos/[repoID]                  Legacy repo-ID route; redirects/falls back to canonical repo view
/repos/[repoID]/pr/[prID]        Legacy PR-ID route; redirects to the repo workspace
/settings                        Repo management, delete account
```

### Key Components

| Component | Location | Notes |
|---|---|---|
| `GraphCanvas` | `components/graph/graph-canvas.tsx` | React Flow canvas: pan, zoom, click |
| `GraphNode` | `components/graph/graph-node.tsx` | Custom React Flow node: badge, name, signature divider, parameters, return types |
| `NodeDetailPanel` | `components/graph/node-detail-panel.tsx` | Side panel updating on node selection; includes a top-left back control that returns to the PR overview and renders PR descriptions as GitHub-flavored Markdown |
| `DiffBlock` | `components/graph/diff-block.tsx` | Unified diff with line highlighting |
| `IndexingProgress` | `components/onboarding/indexing-progress.tsx` | Animated bar; polls `/status` |
| `PRQueue` | `components/queue/pr-queue.tsx` | List of 5 PR cards |

### Graph Rendering

React Flow (`@xyflow/react`) with a weighted hex-grid layout:
- The API returns a bounded depth-2 neighborhood around weighted seed sets: Go repo graphs seed from `main`, and PR graphs seed from changed nodes.
- Node `weight` is `lines_added + lines_removed + caller_count + callee_count`; high-weight seeds are prioritized near the center.
- The API caps initial graph responses at 150 visible nodes and marks nodes as `boundary=true` when more connected context exists outside the visible set.
- The client places one node per hex cell, keeps boundary nodes near the outer ring, and runs small local swaps to shorten visible edges.
- Node `kind` drives the card colour; `node_type` drives seed placement and changed-node diff pills
- Edges use a custom smart Bezier renderer that attaches to natural points on the raw card body, excludes diff pills from edge geometry, keeps anchors at least 20px away from corners, separates multiple anchors on the same face by at least 20px when space allows, and makes curves leave and enter perpendicular to the chosen faces
- `onNodeClick` updates `selectedNodeId` state; `NodeDetailPanel` reads from it, and its back control clears the selection to return to the PR summary

### Data Fetching

- **PR Queue page**: Server Component; fetches queue from Go API on every request. Manual refresh via `router.refresh()`.
- **Repo/PR Graph page**: The GitHub-shaped repo URL resolves to the internal repo ID, fetches the repo graph and PR queue, and passes them to the shared client-side `GraphCanvas`. Clicking a PR fetches `/prs/number/{number}/graph` in place and caches it for quick switching while the browser URL remains `/{owner}/{repo}`.
- **Lazy code loading**: The side panel fetches repo source from `/nodes/{nodeID}/code` or PR source/diff data from `/prs/{prID}/nodes/{nodeID}/code` only when the user opens code mode, then caches each node response in the mounted graph view.
- **PR description markdown**: `NodeDetailPanel` renders `GraphPR.body` with `react-markdown` and `remark-gfm`; HTML is not enabled, and links open in a new tab.
- **Indexing status**: Client Component polls `GET /repos/{repoID}/status` every 2 seconds until `index_status = 'ready'`.

---

## 8. Auth & Security

### Sign-in Flow

1. `/login` → Supabase Auth GitHub OAuth
2. GitHub → `/auth/callback` → Supabase session
3. `/` and `/auth/callback` both call `GET /api/v1/auth/status?user_id=…`
4. API checks whether the user has a ready repo, installed-but-unindexed repo, or no connected repos:
   - **Ready repo**: redirect to `/{owner}/{repo}`
   - **Installed but not indexed**: redirect to `/onboarding/repos`
   - **No match**: redirect to `/onboarding`
5. Root entry treats `/onboarding` from auth status as `/login`, so a direct visit to `isoprism.com` is always login-first. The OAuth callback keeps the `/onboarding` redirect, because that is the point where Isoprism knows the GitHub user has not connected the app/repo permissions yet.

### Isolation

- All API queries are scoped by `user_id` (single-user prototype; no org-level isolation needed)
- Supabase service role key used only server-side in Go; never sent to client
- Webhook payloads verified with HMAC before processing

---

## 9. Deployment

| Service | Platform | Notes |
|---|---|---|
| Next.js frontend | Vercel | Auto-deploys from `main`; root set to `/web` |
| Go API | Railway | Deployed production API at `https://api.isoprism.com`; single service, 512 MB RAM sufficient |
| Postgres + Auth | Supabase | Managed; free tier sufficient |
| GitHub App | GitHub | Single production app; webhook URL → `https://api.isoprism.com/webhooks/github` |

Frontend-only development happens on `preview` with the web app running at `http://localhost:3000` and `NEXT_PUBLIC_API_URL=https://api.isoprism.com`. API changes are production changes and are made on `main` so Railway deploys them before frontend work depends on them.

The GitHub App install flow uses one production app. The web app encodes the current frontend origin in the GitHub install `state`, and the API redirects back to that origin only when it is listed in `FRONTEND_URLS`. Keep Railway configured with `FRONTEND_URL=https://isoprism.com` and `FRONTEND_URLS=https://isoprism.com,http://localhost:3000`.

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
FRONTEND_URLS                    Comma-separated allowed frontend origins
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

### Phase 0 — Beta Access Loop *(~1.5 days)*

1. Add `beta_invites`, `beta_feedback`, and `beta_questionnaires` migrations.
2. Add invite-token validation using hashed tokens; never persist raw URL tokens.
3. Add beta routes for invite status, invite acceptance, trial status, feedback submission, and questionnaire submission.
4. Preserve invite context across Supabase GitHub OAuth and GitHub App installation callback.
5. Enforce one selected repository per active beta invite in repo selection and `POST /repos/{repoID}/index`.
6. Add trial timing: `trial_starts_at` when the selected repo is indexed, `trial_ends_at = trial_starts_at + interval '7 days'`.
7. Add frontend states for invalid invite, active trial status, questionnaire due, feedback modal, and questionnaire page.
8. Add `/admin` with tester creation, invite-link generation, usage monitoring, selected repo visibility, and questionnaire answer review.
9. Configure feedback issue creation with `GITHUB_FEEDBACK_TOKEN` and `GITHUB_FEEDBACK_REPO`.

### Phase 1 — Auth, Repo Selection, and DB Foundation *(~1 day)*

1. Write and apply new DB migration (clean schema: `users`, `github_installations`, `repositories`, `pull_requests`, `code_nodes`, `code_edges`, `pr_node_changes`, `pr_analyses`)
2. Migrate Railway service: update `fly.toml` → `railway.json` / `Dockerfile`; confirm env vars set
3. Strip all old multi-tenant and org-based code from the backend and frontend
4. Simplify auth flow: on sign-in, upsert `users` record; check for existing `repositories` and redirect accordingly
5. Update `/onboarding/repos`: fetch user's GitHub repos, single-select, submit → `POST /repos/{repoID}/index`

---

### Phase 2 — Parsing *(~1.5 days)*

1. Implement Go parsing with `go/parser` + `go/ast`; implement lightweight TypeScript/JavaScript parsing for function declarations, exported const arrow functions, and class methods.
2. Implement `parser.Parse(content []byte, filePath string) []CodeNode` — returns node boundaries + signatures and flags test code.
3. Implement `parser.ExtractCallEdges(content []byte, filePath string, nodeSet map[string]uuid) []CallEdge` — walks call expressions, resolves against nodeSet
4. Implement `parser.ExtractTestReferences(content []byte, filePath string, nodeSet map[string]uuid) []TestReference` for Go, TypeScript, and JavaScript tests.
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
   - Batch repo node summaries in groups of up to 30; process PR change summaries in one Claude call
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
3. Adjust parser call graph resolution if false positive rate is high (e.g. add file-scoped name filtering)
4. Adjust AI prompt if summaries are too vague or too verbose

---

## 11. Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Railway over Fly.io | Railway | Simpler deploy config; no fly.toml; Railway detects Dockerfile automatically |
| Current parser scope | Go + TypeScript + JavaScript | Matches the implemented parser. Rust and Python should be documented only when production parsing exists. |
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
| v0.7 | 2026-04-21 | Railway; clean schema; graph parser pipeline; three-event backend model (RepoInit/OpenPR/MergePR); `code_nodes` keyed by commit SHA; `node_type` derived at query time |
| v0.8 | 2026-04-26 | Test code excluded from production graph; `code_test_references` records tests that exercise production nodes; parser docs reflect Go/TypeScript/JavaScript implementation |
| v0.9 | 2026-04-29 | Documented invite-only beta loop: unique access tokens, one selected repo, one-week trial, in-product feedback, and end-of-week questionnaire |
