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
4. The backend indexes the repo, building a base code graph from the repository default branch HEAD
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
| AI | Google Gemini (`gemini-2.5-flash`) for PR analysis only | Google Gemini API |
| GitHub Integration | GitHub App (webhooks + REST API v3) | — |
| Code Parsing | Tree-sitter-backed Go, TypeScript, TSX, JavaScript, and JSX parser | In-process Go API with CGO |
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
                        │  │  Gemini PR enrichment               │ │
                        │  └──────────────────┬─────────────────┘ │
                        └──────────────────────┼──────────────────┘
                                               │ pgx
                        ┌──────────────────────▼──────────────────┐
                        │           Supabase (Postgres 15)         │
                        │   repositories | pull_requests |         │
                        │   code_nodes | code_edges |              │
                        │   pr_node_changes | pr_analyses          │
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
/supabase
  /migrations Plain SQL migration files used by Supabase CLI
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
  default_branch      text DEFAULT 'main' -- persisted from GitHub metadata
  main_commit_sha     text            -- HEAD of default branch as of last RepoInit or MergePR
  index_status        text DEFAULT 'pending'  -- 'pending' | 'running' | 'ready' | 'failed'
  is_active           boolean DEFAULT true
  created_at          timestamptz
  UNIQUE (user_id, github_repo_id)

-- Indexing jobs
indexing_jobs
  id                  uuid PK
  repo_id             uuid FK → repositories
  commit_sha          text
  status              text DEFAULT 'pending'  -- 'pending' | 'running' | 'ready' | 'failed'
  phase               text                    -- queued | fetching_files | writing_nodes | building_edges | extracting_tests | ready | failed
  message             text
  files_total         int DEFAULT 0
  files_done          int DEFAULT 0
  nodes_total         int DEFAULT 0
  nodes_done          int DEFAULT 0
  edges_total         int DEFAULT 0
  edges_done          int DEFAULT 0
  started_at          timestamptz
  updated_at          timestamptz
  finished_at         timestamptz
  error               text
  UNIQUE (repo_id, commit_sha)

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
  base_commit_sha     text            -- SHA of the default branch at time PR was opened/last rebased
  head_commit_sha     text            -- current HEAD of the PR branch; updated on synchronize
  state               text            -- 'open' | 'closed' | 'merged'
  draft               boolean DEFAULT false
  html_url            text
  opened_at           timestamptz
  merged_at           timestamptz
  last_activity_at    timestamptz
  graph_status        text DEFAULT 'pending'  -- 'pending' | 'running' | 'ready' | 'skipped' | 'failed'
  processor_commit_sha text           -- Isoprism API commit that last processed this PR
  processed_at        timestamptz     -- when the latest OpenPR processing snapshot was written
  processing_error    text            -- latest processing error or skip/suspicion reason, if any
  processing_stats    jsonb DEFAULT '{}'::jsonb
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
  full_name        text              -- qualified name, e.g. "AuthService.handleAuth"
  file_path        text              -- relative path within repo
  line_start       int
  line_end         int
  inputs           jsonb             -- [{name?: string, type: string}]
  outputs          jsonb             -- [{name?: string, type: string}]
  language         text              -- 'go' | 'typescript' | 'javascript'
  kind             text              -- 'function' | 'method' | 'type' | 'struct' | 'interface'
  body_hash        text              -- SHA-256 of the node body; used for change detection
  doc_comment      text              -- cleaned adjacent source comment that documents this component
  is_test     boolean           -- true for parsed test files / test entrypoints
  is_entrypoint boolean         -- true for explicit test entrypoints such as Go Test* functions
  summary          text              -- AI: what this node does (2 sentences)
  created_at       timestamptz
  UNIQUE (repo_id, commit_sha, full_name, file_path)

-- Semantic edges at a given commit
code_edges
  id               uuid PK
  repo_id          uuid FK → repositories
  commit_sha       text
  source_id        uuid FK → code_nodes
  destination_id   uuid FK → code_nodes
  edge_kind        text              -- 'calls' | 'owns_method'
  created_at       timestamptz
  UNIQUE (repo_id, commit_sha, source_id, destination_id, edge_kind)

The graph API serves this canonical function/type graph directly. `code_nodes` and `code_edges` remain the source of truth; graph responses do not expose package/object projections. Test nodes are stored in `code_nodes` and linked through `calls` edges, but default graph responses filter them out unless the frontend is showing a test-focused graph.

-- ────────────────────────────────────────────
-- PR delta: what changed and AI change summaries
-- ────────────────────────────────────────────

-- One row per semantic node that was added, modified, deleted, or renamed in a PR.
-- Added, modified, and renamed rows reference the HEAD-commit version of the
-- node in code_nodes. Deleted rows reference the base-commit version.
-- node_type (changed | caller | callee) is NOT stored here; it is derived
-- at query time by traversing code_edges from the changed set.
pr_node_changes
  id               uuid PK
  pull_request_id  uuid FK → pull_requests
  node_id          uuid FK → code_nodes
  change_type      text                   -- 'added' | 'modified' | 'deleted' | 'renamed'
  change_summary   text                   -- AI: what changed in this node (2 sentences)
  diff_hunk        text                   -- unified diff for this node
  old_full_name    text                   -- previous symbol name for renamed nodes
  old_file_path    text                   -- previous file path for renamed nodes
  created_at       timestamptz
  UNIQUE (pull_request_id, node_id)

-- PR-level summary (for the PR overview description and urgency scoring)
pr_analyses
  id               uuid PK
  pull_request_id  uuid FK → pull_requests UNIQUE
  summary          text              -- AI summary shown as the PR overview description
  nodes_changed    int               -- count of directly changed nodes
  risk_score       int               -- 1–10
  risk_label       text              -- legacy nullable field; UI derives labels from risk_score
  ai_model         text
  analysis_payload jsonb             -- validated PR AI output
  prompt_version   text
  generated_at     timestamptz
  created_at       timestamptz
```

### Pilot Tables

The pilot flow is registration-first. Prospective testers submit `/pilot/register`, admins review Registration forms, and selected users receive a generated access-code link by email. Review responses are saved through `/pilot/review/{token}`.

```sql
-- One row per pilot user.
pilot_users
  id                    uuid PK
  name                  text
  email                 text
  status                text DEFAULT 'registered'
  token                 text UNIQUE        -- generated when the admin sends the invite
  invited_at            timestamptz
  review_token          text UNIQUE
  review_sent_at        timestamptz
  pilot_languages       text
  public_repo_url       text
  issue_count           int
  feature_count         int
  expires_at            timestamptz
  accepted_at           timestamptz
  completed_at          timestamptz
  user_id               uuid FK -> users
  selected_repo_id      uuid FK -> repositories
  trial_starts_at       timestamptz
  trial_ends_at         timestamptz
  created_at            timestamptz

-- Registration and review form submissions.
pilot_forms
  id                    uuid PK
  pilot_user_id         uuid FK -> pilot_users
  form_type             text               -- 'registration' | 'review'
  name                  text
  email                 text
  answers               jsonb
  submitted_at          timestamptz
  created_at            timestamptz

-- Registration answers currently contain:
-- software_experience, ai_writes_most_software, current_review_tools,
-- review_work_percent, review_pain_points, ai_review_difference,
-- interested_in_pilot, name, email, pilot_languages, public_repo_url
-- Review answers currently contain:
-- would_keep_using, keep_using_reason, most_important_features,
-- not_keep_using_reason, switch_requirements, open_to_follow_up

-- One review questionnaire response per pilot user, kept for the existing review UI contract.
pilot_questionaire
  id                    uuid PK
  invite_id             uuid FK -> pilot_users UNIQUE
  user_id               uuid FK -> users
  repo_id               uuid FK -> repositories
  would_keep_using        text
  keep_using_reason       text
  most_important_features text
  not_keep_using_reason   text
  switch_requirements     text
  open_to_follow_up       text
  submitted_at            timestamptz
  created_at              timestamptz
```

The selected repository should be enforced at the product layer and, ideally, by API checks: once `selected_repo_id` is set for an active invite, the tester should not be able to index a second repository through normal UI/API paths.

### Pilot Admin Console

The pilot admin console lives at `/admin` and requires `ADMIN_PASSWORD` before rendering data.

It should let an operator:

- Review Registration and Review forms
- See registered pilot users and link to their registration form
- Add or delete pilot users manually
- Generate an access-code link and send the invite email with Mailtrap
- Track invited/active pilots by setup date, selected repo, and issue/feature submissions
- Send a review email after the pilot period

The admin API is password-gated with `ADMIN_PASSWORD`. Frontend requests send the password as `X-Admin-Password`; the API compares it server-side before listing forms, listing users, creating users, deleting users, or sending pilot emails.

### Design Notes

**`code_nodes` is a content-addressed snapshot store.** The same function at two different commits is two rows. The `body_hash` field enables change detection between commits without re-parsing. Base repository AI summaries are not generated in the current PR-focused AI flow.

**`node_type` is not stored.** Whether a node is `changed`, `caller`, `callee`, or general `context` is a property of its relationship to a specific PR, not of the node itself. The API computes this at query time by:
1. Looking up the set of `pr_node_changes` for the PR → these are `changed` nodes
2. Traversing one hop of `code_edges` from that set → callers, callees, and semantic context such as receiver owner types

**Separate base graph and PR delta.** `code_nodes` + `code_edges` are the base graph, built during `RepoInit` and kept current by `MergePR`. `pr_node_changes` is the PR-specific semantic overlay, built during `OpenPR`. The PR graph endpoint also returns GitHub's changed-file list from the Pull Request Files API so the PR view keeps full file-diff parity with GitHub, including docs, config, generated files, global variable edits, and other non-node changes that do not become graph nodes.

**Receiver ownership is a semantic edge.** A Go receiver type and its methods are separate `code_nodes`. The owner relation is persisted as `edge_kind = 'owns_method'` with `source_id` pointing at the struct/type/interface node and `destination_id` pointing at the method node. This lets PR graphs pull important receiver context, such as `BlockAPI`, into reviews even when only the methods changed.

**Tests are first-class code nodes, not default graph cards.** Test code is persisted in `code_nodes` with `is_test` / `is_entrypoint`, and test-to-production relationships are represented as `calls` edges. Default repo/PR graph responses filter test nodes out of the visible graph, then attach matching test callers to production nodes as `tests[]`. PR processing also persists changed test functions in `pr_node_changes`; the PR graph endpoint returns them in `test_changes[]` so the PR view can show test-function labels and diffs separately from graph changes. `test_changes[]` includes changed test helpers, but the PR overview lists only rows where `is_test` and `is_entrypoint` are both true. When a reviewer selects a changed test entrypoint, the frontend derives a temporary test-focused graph from that test node, reachable changed test helpers, and production nodes whose `tests[]` references or test edges match it; selecting a production component restores the normal PR diff graph.

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
| `installation_repositories` | added, removed | Repo sync; stores the GitHub default branch for added repos |

All webhooks verified with `X-Hub-Signature-256` HMAC before processing.

### Installation Flow

`GET /api/v1/github/callback` receives `installation_id`, `setup_action`, and `state` (user's Supabase UUID).

For the beta flow, `state` must preserve both the authenticated user/session identity and the active beta invite context. After GitHub App install or settings update, the callback re-syncs repository authorization and then decides from Isoprism account state where the user should land.

| Condition | Behaviour |
|---|---|
| `setup_action=request` | User lacks permission; redirects to `/request-sent` |
| Existing Isoprism setup | Redirects to `/settings`, which resolves the signed-in user's `/{user}/settings` route |
| No Isoprism setup yet | Redirects to `/onboarding/repos` so the user can pick their first repository |

Existing setup is determined from Isoprism state, not from GitHub's `setup_action`: `users.selected_repo_id`, `pilot_users.selected_repo_id`, or an already ready repository means the account has completed setup. This prevents a GitHub App settings edit from sending an existing user back through first-time repo selection. Re-syncing the repo list applies these repository states:

GitHub App settings updates may omit the original `state` value. When `state` does not contain a user ID, the callback resolves the user from existing repository rows for the installation before syncing repository authorization and deciding the redirect.

- Added GitHub repositories are stored as authorized and visible, but not indexed.
- Removed GitHub repositories are marked revoked immediately, hidden from `GET /api/v1/me/repos`, and scheduled for repository-row deletion after one day.
- Authorized repositories that already have indexed data keep their index unless the user explicitly uninstalls them or pilot-account selection rules mark them unused.

### Token Management

- **App JWT**: RS256-signed, 9-min expiry — used to fetch installation tokens
- **Installation tokens**: 1-hour expiry, fetched from `POST /app/installations/{id}/access_tokens`, cached in memory and refreshed on demand

---

## 5. Backend Events

The backend is defined around three events. All other logic flows from them.

---

### Event 1 — `RepoInit`

**Trigger:** `POST /api/v1/repos/{repoID}/index` (called by frontend after repo selection)

**Purpose:** Build the base code graph for the repository from the current HEAD of its GitHub default branch.

`RepoInit` is idempotent per `(repo_id, commit_sha)`. If the current default-branch commit already has a `ready` `indexing_jobs` row, `POST /index` returns immediately and the UI opens the existing graph. If that commit is already `pending` or `running`, `POST /index` returns the existing job instead of starting another worker. A new job is created only when the default branch points to a new commit or a failed job is retried.

**Steps:**

1. Fetch the current HEAD commit SHA of `repositories.default_branch` via `GET /repos/{owner}/{repo}/git/ref/heads/{default_branch}`
2. Fetch the full file tree at that SHA via `GET /repos/{owner}/{repo}/git/trees/{sha}?recursive=1`
3. Filter to supported source files (`.go`, `.ts`, `.tsx`, `.js`, `.jsx`)
4. For each file, fetch content via `GET /repos/{owner}/{repo}/contents/{path}?ref={sha}` and parse it to extract functions, methods, and types.
5. Persist production and test nodes in `code_nodes`, setting `is_test` and `is_entrypoint` from parser metadata.
6. Build semantic edges: resolve function/test calls as `calls` edges, and connect receiver owner types to their methods as `owns_method` edges → insert `code_edges`.
7. Set `repositories.main_commit_sha = HEAD`, `repositories.index_status = 'ready'`, and `repositories.indexed_at` so the graph is visible as soon as structural indexing completes.
8. Do not run AI during repository indexing. Base code-node summaries are intentionally out of scope; AI spend is reserved for PR analysis.

Files are processed concurrently (bounded goroutine pool, max 10 in-flight). The frontend polls `GET /api/v1/repos/{repoID}/status` every 2 seconds until `ready` or `failed`. The status response includes the current indexing job phase, progress percentage, counters, and a rough ETA. `ready` means the structural graph is available; summaries may continue to fill in afterward.

---

### Event 2 — `OpenPR`

**Trigger:** `pull_request` webhook with action `opened` or `synchronize`

**Purpose:** Compute which nodes changed between indexed `main` and the PR's head commit, and generate change summaries. During beta, only PRs targeting `main` are processed.

**Steps:**

1. **Validate branch and SHA:** Require `pull_requests.base_branch = repositories.default_branch` and `pull_requests.base_commit_sha = repositories.main_commit_sha`. If either check fails, mark `graph_status = 'skipped'` so the PR is hidden instead of rendered against an approximate graph. Reserve `failed` for processing errors.
2. **Check PR size limits:** Fetch GitHub PR metadata and skip oversized PRs before expensive file fetching. During beta the limits are 300 changed files, 20,000 additions, 20,000 deletions, or 30,000 total changed lines. Oversized PRs are marked `graph_status = 'skipped'`, stale `pr_node_changes` are cleared, and `pr_analyses.summary` stores the reason.
3. **Check cache:** Look up the PR's stored `head_commit_sha`. If the incoming webhook's `head_sha` matches → already processed, skip.
4. **Fetch semantic diff metadata:** Fetch GitHub PR files via `GET /repos/{owner}/{repo}/pulls/{number}/files`, falling back to `GET /repos/{owner}/{repo}/compare/{base_commit_sha}...{head_sha}` if needed. These responses provide changed paths, rename metadata, line counts, and unified file patches.
5. **Parse head commit only:** For each changed supported file that still exists at `head_sha`, fetch the head content and parse it. Insert all parsed head nodes, including test nodes, into `code_nodes` at `commit_sha = head_sha`. PR processing does not fetch base file contents; it loads base node metadata from the indexed `code_nodes` rows at `base_commit_sha`.
6. **Identify changed nodes:** Compare parsed head nodes with base `code_nodes` loaded from SQL for the affected base paths. Nodes with a differing `body_hash` and matching identity are `modified`; nodes present only at head are `added`; nodes present only at base are `deleted`; nodes paired by rename metadata or matching hashes are `renamed`. Line overlap alone is not a rename signal; when a new component overlaps an old component but does not share its identity or body hash, classify conservatively as added and let any unmatched base component appear as deleted.
7. **Build component diffs:** `modified`, `renamed`, and `deleted` nodes keep a component-scoped slice of the GitHub patch using the stored base/head line ranges. `added` nodes use a synthetic component hunk where every line of the fetched head component is marked `+`, so semantic node stats count the whole new component even when Git's file diff treats moved/copied body lines as unchanged context.
8. **Persist graph overlay:** Insert `pr_node_changes` rows without AI summaries, rebuild PR-head call edges for all parsed nodes including tests, and insert/update `pr_analyses.nodes_changed`. Graph-only reprocessing clears stale AI fields because node IDs and diffs may have changed.
9. **Generate PR AI:** After the graph overlay exists, call Gemini once for normal-sized PRs with PR title/body, production component diffs, test diffs, and other changed file diffs as context. Persist production change summaries, test assertion summaries, PR summary, numeric `risk_score`, `ai_model`, `analysis_payload`, `prompt_version`, and `generated_at`. Other changed files are prompt context only, not structured output.
10. **Update PR:** Set `graph_status = 'ready'` and stamp the latest processing metadata on `pull_requests`: `processor_commit_sha`, `processed_at`, `processing_error`, and `processing_stats`. If changed nodes were detected but zero `pr_node_changes` rows persisted, `processing_error` records that suspicion while leaving the structural counters available for debugging.

When serving a graph, the API returns `full_name` as the display name and structured `inputs[]`/`outputs[]` instead of a raw signature string. Each input/output item has an optional `name`, a `type`, and, when that type exists as a visible graph node, a `node_id` so the frontend can link directly to the type node.

When serving a PR graph, the API also fetches `GET /repos/{owner}/{repo}/pulls/{number}/files` and returns those file-level diffs as `files[]`. The PR overview uses `files[]` as the source of truth for rendered patches and total additions/deletions, matching GitHub's PR files view and including tests, changelogs, and other non-graph files. The semantic graph still treats `file_path + full_name` as the identity for visible nodes. This collapses duplicate rows for the same function across indexed-main and PR-head commits into one visual node, preferring the changed PR-head row when available. After collapsing, the API rewrites and de-duplicates edges and removes any edge whose endpoints are not in the final production-node set. Test nodes are filtered from graph responses and test coverage is returned only through each production node's `tests[]` field.

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

The current parser supports Go, TypeScript, TSX, JavaScript, and JSX through tree-sitter. The parser public API remains:

- `parser.Parse(content []byte, filePath string) []Node`
- `parser.ExtractCallEdges(content []byte, filePath string, nodeSet map[string]bool) []CallEdge`
- `parser.ExtractCallEdgesWithResolver(content []byte, filePath string, resolverIndex ResolverIndex) []CallEdge`

| Language | Parser | Extracted production nodes | Test code handling |
|---|---|---|---|
| Go | `tree-sitter-go` | functions, methods, type specs, structs, interfaces | Stores `*_test.go`, packages ending `_test`, and functions beginning `Test` as `code_nodes` with `is_test`; `Test*` functions are marked `is_entrypoint` |
| TypeScript / TSX | `tree-sitter-typescript` / `tree-sitter-tsx` | function declarations, const/let arrow functions, class methods | Stores `*.test.ts`, `*.spec.ts`, `*.test.tsx`, `*.spec.tsx`, and `__tests__/` nodes as `is_test`; `test(...)` / `it(...)` calls are stored as `is_entrypoint` nodes |
| JavaScript / JSX | `tree-sitter-javascript` | function declarations, const/let arrow functions, class methods | Stores `*.test.js`, `*.spec.js`, `*.test.jsx`, `*.spec.jsx`, and `__tests__/` nodes as `is_test`; `test(...)` / `it(...)` calls are stored as `is_entrypoint` nodes |

Rust and Python are not currently indexed.

For Go, TypeScript, TSX, JavaScript, and JSX, the parser also captures the contiguous `//`, `/* ... */`, or `/** ... */` comment block immediately above a component when there is no blank line between the comment and the declaration. The cleaned text is stored as `code_nodes.doc_comment`. Graph API responses keep this raw field as `doc_comment` and prepend it to `summary` for display; AI enrichment is not modified by this comment extraction path.

**Symbol identity:** Go full names include directory and package context (`path/to/package:package.Symbol`, or `path/to/package:package.Receiver.Method`) so repeated package-level names like `New` do not collide across packages. TypeScript/JavaScript full names include module path context (`path/to/module.Symbol`).

**Call graph extraction:** After extracting production node boundaries, function bodies are walked for calls and resolved against the full production node set for the same commit. Resolution is intentionally conservative: unresolvable names, stdlib/external package calls, ambiguous suffix matches, and selector/member calls whose receiver type is unknown are discarded. Go selector calls are never resolved by selector name alone; for example `sha256.New()` does not link to an internal `client.New` node. Imported Go selector calls match the repository-relative import path and selector, not the import path basename as a package-name guess, so `grpccore.StartGRPCServer(...)` can resolve to a node declared as `rpc/grpc:coregrpc.StartGRPCServer`.

**Resolver index:** Repo and PR indexing build a `ResolverIndex` from full source files before inserting call edges. The shared resolver index stores known node names plus language-specific semantic facts. The Go adapter currently records struct field types, receiver bindings, parameter types, simple local variable declarations, import aliases, and repository-relative import directories. This lets the call graph resolve safe receiver and field-chain calls such as `blockAPI.env.EventBus.Unsubscribe(...)` to `types.EventBus.Unsubscribe` when every hop has one known type. Ambiguous or missing field/type hops still produce no edge.

**Receiver ownership extraction:** Go methods whose full name extends a parsed receiver type full name are linked with `owns_method` edges. For example, `rpc/grpc:coregrpc.BlockAPI` is the source and `rpc/grpc:coregrpc.BlockAPI.Stop` is the destination.

**Test graph extraction:** Test files are parsed into `code_nodes` alongside production code. Test callers are linked to production callees through `calls` edges; the API derives each production node's `tests[]` from those edges. Changed PR test functions can also appear as `test_changes[]`; the frontend only renders them as graph cards in the temporary test-focused graph when the reviewer selects a test entrypoint.

**Build note:** Tree-sitter grammar bindings use CGO. Local and Railway API builds must run with CGO enabled and a working C compiler available.

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
      enricher.go          Gemini PR analysis enrichment
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
POST /api/v1/pilot/register
POST /api/v1/pilot/review/{token}
POST /api/v1/pilot/invites/{token}/accept
GET  /api/v1/admin/pilot/users
POST /api/v1/admin/pilot/users
DELETE /api/v1/admin/pilot/users/{userID}
GET  /api/v1/admin/pilot/forms
POST /api/v1/admin/pilot/users/{userID}/invite
POST /api/v1/admin/pilot/users/{userID}/review-email
```

**Authenticated**
```
POST   /api/v1/beta/feedback

GET    /api/v1/me/repos                                   list repos for current user
DELETE /api/v1/me

GET    /api/v1/repos/{repoID}                             repo detail + index_status
POST   /api/v1/repos/{repoID}/select                      select an already indexed repository
POST   /api/v1/repos/{repoID}/index                       trigger RepoInit
DELETE /api/v1/repos/{repoID}/index                       uninstall indexed data while keeping authorized repo visible
GET    /api/v1/repos/{repoID}/status                      {index_status, index_job, index_percent, eta_seconds, pr_count, ready_count}
GET    /api/v1/repos/{repoID}/queue                       top 5 PRs by urgency
GET    /api/v1/repos/{repoID}/graph                       function-level repo graph from default branch HEAD
GET    /api/v1/repos/{repoID}/nodes/{nodeID}/code         lazy repo node source
GET    /api/v1/repos/{repoID}/prs/{prID}/graph            function-level PR graph (nodes + edges + deltas)
GET    /api/v1/repos/{repoID}/prs/number/{number}/graph   function-level PR graph by GitHub PR number
GET    /api/v1/repos/{repoID}/prs/{prID}/nodes/{nodeID}/code lazy PR node source/diff
```

The PR node code view reconstructs a full component diff from fetched source only when the relevant source sides are present: both base and head for modified or renamed nodes, head for added nodes, and base for deleted nodes. If source lookup is incomplete, the frontend renders the persisted `diff_hunk` from `pr_node_changes` so a partial source response does not turn a real modification into an all-added or all-deleted display.

### Beta Access Rules

- A tester can only start from a valid beta invite token.
- GitHub OAuth must bind the invite to one `users.id`.
- GitHub App installation may expose several repositories, but pilot users can actively use only one indexed repository at a time.
- `users.selected_repo_id` is the current selected repository for both pilot and regular users; `pilot_users.selected_repo_id` mirrors it for pilot admin/reporting.
- `POST /api/v1/repos/{repoID}/index` selects the repository and triggers indexing. For pilot users, selecting a different repository marks the previous selected repo as unused and schedules its indexed data for deletion after one day. For regular users, previously indexed repositories remain indexed.
- `POST /api/v1/repos/{repoID}/select` changes the selected repository only after it is indexed. Only one repository can be selected at a time.
- `DELETE /api/v1/repos/{repoID}/index` marks indexed data unused and schedules cleanup. It does not remove the repository from the authorized list while GitHub still grants access.
- Revoked GitHub access marks the repository revoked, removes it from the authorized list immediately, and schedules full repository deletion after one day.
- The trial starts when the tester selects a repository and indexing is triggered.
- The trial ends seven calendar days after `trial_starts_at`.
- Feedback submissions are accepted during the trial and may continue after the questionnaire is due if the product remains accessible.
- The questionnaire is due once `now() >= trial_ends_at` and should be stored once per invite.
- Feedback submissions should create GitHub issues in the configured feedback repository using labels `bug` or `feature`, and should include the tester's user ID, selected repository, PR/node context, browser path, app commit SHA, and source commit SHA.

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
    "head_commit_sha": "def456",
    "body": "...",
    "author_login": "octocat"
  },
  "nodes": [
    {
      "id": "...",
      "full_name": "AuthService.handleAuth",
      "file_path": "internal/auth/service.go",
      "package_path": "internal/auth",
      "line_start": 42,
      "line_end": 78,
      "inputs": [{"name": "token", "type": "string"}],
      "outputs": [{"type": "*User"}, {"type": "error"}],
      "language": "go",
      "kind": "method",
      "node_type": "changed",
      "summary": "Validates a JWT token and returns the associated user. Returns an error if the token is expired or malformed.",
      "change_summary": "Now validates token expiry against a configurable grace period instead of hard-coding 5 minutes. Adds a new error type for expired tokens.",
      "diff_hunk": "@@ -42,10 +42,14 @@ ...",
      "change_type": "modified",
      "lines_added": 4,
      "lines_removed": 1,
      "tests": []
    },
    {
      "id": "...",
      "full_name": "authMiddleware",
      "file_path": "internal/api/middleware.go",
      "package_path": "internal/api",
      "line_start": 12,
      "line_end": 34,
      "inputs": [],
      "outputs": [],
      "language": "go",
      "kind": "function",
      "node_type": "caller",
      "summary": "HTTP middleware that extracts the Bearer token from the Authorization header and calls handleAuth.",
      "change_summary": null,
      "diff_hunk": null,
      "lines_added": 0,
      "lines_removed": 0,
      "tests": []
    }
  ],
  "edges": [
    {
      "source_id": "...",
      "destination_id": "...",
      "edge_kind": "calls"
    }
  ],
  "files": [
    {
      "filename": "internal/auth/service.go",
      "status": "modified",
      "additions": 4,
      "deletions": 1,
      "changes": 5,
      "patch": "@@ -42,10 +42,14 @@ ..."
    },
    {
      "filename": "internal/auth/service_test.go",
      "status": "added",
      "additions": 80,
      "deletions": 0,
      "changes": 80,
      "patch": "@@ -0,0 +1,80 @@ ..."
    }
  ]
}
```

### Queue Urgency Scoring

```
urgency = (wait_time_score × 0.4) + (risk_score × 0.35) + (nodes_changed_score × 0.25)
```

Computed at query time from `pr_analyses`. PRs with `graph_status != 'ready'` are excluded from the queue until processing completes. During beta, the queue also excludes PRs that do not target the repository's indexed default branch or whose `base_commit_sha` does not match the repository's indexed `main_commit_sha`.

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
- The API returns a bounded depth-2 neighborhood around weighted seed sets. Repo graphs seed from entrypoint functions such as `main`; PR graphs seed from changed nodes.
- Node `weight` is `lines_added + lines_removed + caller_count + callee_count`; high-weight seeds are prioritized near the center.
- The API caps initial graph responses at 150 visible nodes and marks nodes as `boundary=true` when more connected context exists outside the visible set.
- The client places one node per hex cell, keeps boundary nodes near the outer ring, and runs small local swaps to shorten visible edges.
- Node `kind` drives the card colour; `node_type` drives seed placement and changed-node diff pills
- Edges use a custom smart Bezier renderer that attaches to natural points on the raw card body, excludes diff pills from edge geometry, keeps anchors at least 20px away from corners, separates multiple anchors on the same face by at least 20px when space allows, and makes curves leave and enter perpendicular to the chosen faces
- In PR view, `NodeDetailPanel` keeps the PR overview visible and clicking a graph/test/file change opens a resizable middle `ComponentChangePanel` between the overview and the graph. Graph/test rows show the component overview followed by the diff/code view in one scroll; documentation and other rows show the GitHub file patch. Opening or resizing the panel refits the graph into the remaining canvas.

### Data Fetching

- **PR Queue page**: Server Component; fetches queue from Go API on every request. Manual refresh via `router.refresh()`.
- **Repo/PR Graph page**: The GitHub-shaped repo URL resolves to the internal repo ID, fetches the PR queue, and mounts `GraphCanvas` with an empty repo graph shell so the repo-wide graph is not loaded by default. Clicking a PR fetches `/prs/number/{number}/graph` in place and caches it for quick switching while the browser URL remains `/{owner}/{repo}`.
- **Lazy code loading**: The side panel fetches repo source from `/nodes/{nodeID}/code` or PR source/diff data from `/prs/{prID}/nodes/{nodeID}/code` only when the user opens code mode, then caches each node response in the mounted graph view.
- **PR description markdown**: `NodeDetailPanel` renders `GraphPR.body` with `react-markdown` and `remark-gfm`; HTML is not enabled, and links open in a new tab.
- **PR change buckets**: After the rendered PR description, the PR overview groups rows into Graph changes, Test changes, Documentation changes, and Other changes. Graph/test rows come from component metadata (`nodes[]` and `test_changes[]`); documentation/other rows come from GitHub `files[]`.
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

Frontend and API development happens on `main` for now. Local web runs at `http://localhost:3000` with `NEXT_PUBLIC_API_URL=https://api.isoprism.com`; pushes to `main` deploy the web app through Vercel and API changes through Railway. Keep `preview` only as a synced mirror of `main` while any external tooling still expects it.

The GitHub App install flow uses one production app. The web app encodes the current frontend origin in the GitHub install `state`, and the API redirects back to that origin only when it is listed in `FRONTEND_URLS`. Keep Railway configured with `FRONTEND_URL=https://isoprism.com` and `FRONTEND_URLS=https://isoprism.com,http://localhost:3000`.

### Environment Variables

**Go API (Railway)**
```
SUPABASE_URL
SUPABASE_SERVICE_ROLE_KEY
GITHUB_APP_ID
GITHUB_APP_PRIVATE_KEY       PEM; literal \n sequences normalised on load
GITHUB_WEBHOOK_SECRET
GEMINI_API_KEY
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

Plain SQL files live in `/supabase/migrations/` so `supabase db push` can apply them directly. Use the Supabase CLI against the linked project or pass `--db-url "$DATABASE_URL"` when applying production migrations from a machine that is not logged into Supabase.

At startup, the API queries `supabase_migrations.schema_migrations` and exits before serving traffic unless the latest recorded migration version matches `api/internal/db.RequiredMigrationVersion`. Each new migration must update that constant in the same change; the API tests compare it to the latest local migration filename.

---

## 10. Implementation Plan

### Phase 0 — Beta Access Loop *(~1.5 days)*

1. Add `pilot_users` and `pilot_questionaire` migrations.
2. Add invite-token validation using stored prototype tokens.
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

1. Implement tree-sitter parsing for Go, TypeScript, TSX, JavaScript, and JSX function declarations, const/let arrow functions, methods, and Go type declarations.
2. Implement `parser.Parse(content []byte, filePath string) []CodeNode` — returns node boundaries + signatures and flags test code.
3. Implement `parser.ExtractCallEdges(content []byte, filePath string, nodeSet map[string]uuid) []CallEdge` — walks call expressions, resolves against nodeSet
4. Mark test code and test entrypoint nodes during `parser.Parse` for Go, TypeScript, and JavaScript tests.
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

1. Implement `ai.EnrichPRChanges(ctx, input PRAnalysisInput) error`:
   - One Gemini call per normal-sized PR with PR title/body, production diffs, test diffs, and other changed files as context
   - Structured JSON output: `changes[]`, `test_assertions[]`, `pr_summary`, and numeric `risk_score`
   - Map production summaries and test assertion summaries back to `pr_node_changes.change_summary`
   - Store the full validated response in `pr_analyses.analysis_payload`
   - Also generates `pr_analyses.summary` and `risk_score` in the same call
3. Integrate enrichment into `RepoInit` after structural graph readiness, and into `OpenPR` after change detection

---

### Phase 5 — OpenPR and MergePR Events *(~1.5 days)*

1. Implement `events.OpenPR(ctx, pr PullRequest)`:
   - Check `pull_requests.head_commit_sha`; skip if unchanged
   - Fetch PR file metadata via GitHub PR files API, falling back to compare
   - Fetch and parse changed head files at `head_sha`; insert new `code_nodes` including test nodes
   - Load base node metadata from indexed `code_nodes`, diff `body_hash` between base and head, and populate `pr_node_changes`
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
   - Traverse `code_edges` one hop from changed nodes → source, destination, and receiver-owner context IDs
   - Fetch all node records; tag each with computed `node_type`
   - Fetch GitHub PR files and return them as `files[]` for GitHub-equivalent PR overview diffs
   - Cap at 20 nodes: keep all changed nodes, fill remaining slots by proximity
   - Serialise to graph JSON response (see §6)
2. Update queue handler to include `nodes_changed` and derive any risk label from `pr_analyses.risk_score`

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
3. Adjust parser call graph resolution if false positive rate is high; keep resolution conservative and avoid selector-name-only matching.
4. Adjust AI prompt if summaries are too vague or too verbose

---

## 11. Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Railway over Fly.io | Railway | Simpler deploy config; no fly.toml; Railway detects Dockerfile automatically |
| Current parser scope | Go + TypeScript/TSX + JavaScript/JSX via tree-sitter | Matches the implemented parser. Rust and Python should be documented only when production parsing exists. Tree-sitter introduces CGO in API builds. |
| `node_type` computed at query time | Not stored | It is a PR-relative property, not a property of a node. Storing it would couple the base graph to PR state. |
| `code_nodes` keyed by `(repo, commit_sha, full_name, file_path)` | Snapshot per commit | Allows change detection by `body_hash` comparison without storing diffs. Summaries are reused across commits when body is unchanged. |
| One-hop graph depth | Depth 1 from changed nodes | Deeper traversal produces unreadable graphs. One hop gives immediate structural context. |
| Batched AI calls per PR | Single Gemini call | One round-trip covers the changed production/test set and other-file context while keeping output consistent. |
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
| v0.8 | 2026-04-26 | Test code excluded from the default production graph; test nodes are stored in `code_nodes`, linked through `code_edges`, and parser docs reflect Go/TypeScript/JavaScript implementation |
| v0.9 | 2026-04-29 | Documented invite-only beta loop: unique access tokens, one selected repo, one-week trial, in-product feedback, and end-of-week questionnaire |
| v0.10 | 2026-05-04 | Parser migrated to tree-sitter with package/module-aware symbol names and conservative call edge resolution |
