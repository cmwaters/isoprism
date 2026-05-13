# Isoprism Web

Isoprism is a PR review interface that turns code changes into a semantic call graph. Reviewers can inspect changed functions, structs, and nearby callers/callees, then switch between summary-first review and source/diff review for a selected node.

## Local development

```bash
npm install
NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev
```

The web app runs on [http://localhost:3000](http://localhost:3000). It talks to the Railway Go API at `NEXT_PUBLIC_API_URL`, defaulting to `https://api.isoprism.com`.

Local frontend development uses the single production GitHub App and the deployed API. GitHub App installs started from localhost redirect back to localhost through the encoded install `state`, as long as the Railway API includes `http://localhost:3000` in `FRONTEND_URLS`.

Use `main` for frontend and API changes for now. Push verified changes to `main`; Vercel deploys the web app and Railway deploys API changes.

Useful checks:

```bash
npm run lint
npm run build
```

## Pilot registration and auth

The pilot starts at `/pilot/register`. Prospective testers submit the registration form, including their software experience, how they currently review code, whether AI writes most of their software, how much of their work is spent reviewing code, whether they review AI-written software differently, and whether they want to pilot Isoprism for one week with one repository.

Admins review registrations at `/admin`, generate an access code, and send the invite email through Mailtrap. The invite link goes to `/pilot/{token}`, which forwards into the login/GitHub setup flow. A review email can only be sent after the invite has been generated and the pilot user has registered with GitHub; it goes to `/pilot/review/{token}` and submits the end-of-pilot review form.

The current root route is login-first: unauthenticated visitors go to `/login`, and signed-in visitors only skip login when `GET /api/v1/auth/status?user_id=...` returns a ready repo (`/{owner}/{repo}`) or an installed-but-unindexed repo (`/onboarding/repos`). If auth status returns `/onboarding`, root maps that to `/login`; the OAuth callback keeps `/onboarding` so newly signed-in GitHub users without a connected Isoprism repo are prompted to install the GitHub App and grant repo permissions.

The GitHub OAuth login requests `read:user`, `user:email`, and `read:org`. The org scope lets the settings UI discover private organization memberships when GitHub exposes them to the signed-in user's OAuth token.

## Settings

Authenticated views render a fixed account pill in the top-right corner. The pill shows the signed-in user's GitHub avatar and display name, and links to `/{user}/settings`.

Settings are intentionally simple during beta. `/{user}/settings` is a single page where the tester can:

- See their GitHub connection
- Install or manage the Isoprism GitHub App on GitHub
- See the current indexed repository
- Select a different accessible repository
- Trigger indexing for the selected repository

When a different repository is indexed from settings, the page shows the same `IndexingProgress` component used during onboarding.

- `GET /api/v1/me/repos` supplies repositories currently available to Isoprism.
- `POST /api/v1/repos/{repoID}/index` starts or retries indexing from the repositories settings category.

GitHub App install and permission changes still happen on GitHub.

## Pilot admin

The pilot admin console is available at `/admin`. It prompts for the admin password and then calls the Go API with `X-Admin-Password`.

Admin capabilities:

- Review Registration and Review forms.
- Add pilot users manually with a name and optional email.
- Generate an access code and send the pilot invite email through Mailtrap.
- Track started pilots by setup date, selected repo, and submitted issue/feature counts. The selected repo is updated when the pilot user indexes a repo and replaces the requested public repo from registration.
- Send a review email after the pilot period once the pilot user has been invited and has registered with GitHub.
- Delete pilot users.

The API routes are:

```http
POST /api/v1/pilot/register
GET  /api/v1/pilot/review/{token}
POST /api/v1/pilot/review/{token}
POST /api/v1/pilot/invites/{token}/accept
GET  /api/v1/admin/pilot/users
POST /api/v1/admin/pilot/users
DELETE /api/v1/admin/pilot/users/{userID}
GET  /api/v1/admin/pilot/forms
POST /api/v1/admin/pilot/users/{userID}/invite
POST /api/v1/admin/pilot/users/{userID}/review-email
```

Registration emails are unique for the registration form. A second submission with the same email returns `409 Conflict` and does not create another pilot user.

The Railway API must have `ADMIN_PASSWORD` set before this page can unlock. Sending emails also requires `MAILTRAP_API_KEY`; `PILOT_EMAIL_FROM` controls the sender address and must belong to a verified Mailtrap sending domain.

## Repository Graph View

The primary review route mirrors the GitHub repository path:

- `/{owner}/{repo}` fetches `GET /api/v1/repos/{repoID}/graph` plus `GET /api/v1/repos/{repoID}/queue`.

The repo route renders one persistent `GraphCanvas` and side panel:

- A repo graph for the whole indexed system at function-level detail.
- A ranked PR list in the repo overview panel. Each PR card shows the PR number in the graph accent pink, title, AI summary, PR diff totals, a risk label derived from numeric `risk_score` where used, and a client-updated compact open-time badge (`5h`, `3d`, `1w`).
- In-place PR graph loading when a reviewer clicks a PR card. The URL stays `/{owner}/{repo}`.
- A small client-side cache so previously opened PR graphs reappear without another fetch.
- A side panel that reviewers can resize between bounded minimum and maximum widths.
- Node cards with package/type labels, function or method titles, structured inputs/outputs, and added/removed/deleted pills.
- Production nodes only; test code is indexed separately and shown as tests attached to the production nodes it exercises.

During beta, the PR queue only includes open, non-draft PRs targeting the repository's indexed default branch whose `base_commit_sha` exactly matches the repository's indexed `main_commit_sha`. PRs from other base branches, or PRs whose base SHA is out of sync with the indexed default-branch graph, are hidden rather than shown with approximate graph data.

The API also skips oversized PRs before expensive graph processing. The beta limits are 300 changed files, 20,000 additions, 20,000 deletions, or 30,000 total changed lines. Skipped PRs get `graph_status = "skipped"` and a `pr_analyses.summary` explaining which size limit was exceeded.

Large PRs can skip per-function AI summaries so the graph and changed-node overlay still become available without waiting on an oversized enrichment request. AI enrichment is PR-only and uses Gemini; repo indexing does not generate base code-node summaries.

Each PR stores the latest processing snapshot on `pull_requests`: `processor_commit_sha`, `processed_at`, `processing_error`, and `processing_stats`. The stats JSON records file counts, supported-file counts, parsed node counts, detected semantic changes, persisted `pr_node_changes`, and call-edge persistence counters so empty PR views can be debugged without guessing which deployed API revision processed them.

Graph responses are function-level. The API returns canonical `full_name` values, but the UI splits them into a pink package/receiver label and a black title that contains only the function or method name. Nodes expose `inputs[]`/`outputs[]` as structured `{name?, type, node_id?}` records.

PR graph responses still use `file_path + full_name` as the semantic identity for function-level visible nodes. If the same function exists as both an indexed-main node and a PR-head node, the API collapses them into one visual node and prefers the changed PR-head metadata. Edges are rewritten after this collapse, and test nodes are filtered from the visible graph; tests remain available through each production node's `tests[]` detail.

PR review does not have a separate route or page.

During the beta, this repository is the tester's selected trial repository. Feedback controls for bug reports and feature requests should be available from this review workspace and should capture current context such as repository, PR number, selected node, and browser path.

The graph workspace shows a black beta footer banner with "Report a problem" and "Request a feature" actions, plus a dismiss button. Dismissal is page-session only, so the banner reappears after a full browser refresh. Each feedback action opens a centered feedback form and submits to:

```http
POST /api/v1/beta/feedback
```

The API creates a GitHub issue in the configured feedback repository with either the `bug` or `feature` label. The payload includes repo/PR/node context, browser path, frontend app commit, and source commit.

The side panel has two modes:

- `Overview`: repo, PR, or node summary, calls, callers, and tests. PR context shows GitHub-equivalent file diffs from the PR files API, including test files, plus a separate semantic graph-change list when graph nodes are available.
- `Code`: a lazy-loaded source viewer for the selected function or struct. PR changed nodes automatically show the full component with changed lines highlighted. Repo nodes and unchanged context nodes show plain source.

The overview/code icon controls switch the side panel mode without changing the selected graph node.

## API contract used by the PR overview

PR graph responses include the semantic graph and the GitHub file-level diff used by the PR overview:

```http
GET /api/v1/repos/{repoID}/prs/{prID}/graph
GET /api/v1/repos/{repoID}/prs/number/{number}/graph
```

```ts
interface GraphResponse {
  pr: GraphPR;
  nodes: GraphNode[];
  edges: GraphEdge[];
  files: PRFileDiff[];
  test_changes: GraphNode[];
}

interface PRFileDiff {
  filename: string;
  previous_filename?: string;
  status: "added" | "modified" | "removed" | "renamed" | string;
  additions: number;
  deletions: number;
  changes: number;
  patch?: string;
}
```

`files[]` is fetched from GitHub's pull request files endpoint and is the source of truth for PR-level diff totals and rendered patches. It includes docs and non-graph files. `nodes[]` remains the default semantic production graph. Changed test functions are returned separately in `test_changes[]` with the same component-level fields as graph nodes (`change_type`, `diff_hunk`, `lines_added`, `lines_removed`, and rename metadata). `test_changes[]` includes changed test helpers so selected test-entrypoint graphs can show their helper chain, but the PR overview's Test changes list only shows rows where `is_test` and `is_entrypoint` are both true.

The PR overview groups changes into four sections after the rendered description:
- **Graph changes**: changed production functions/types represented in the graph.
- **Test changes**: changed test entrypoints from `test_changes[]`; changed test helpers remain available to the focused test graph but are not listed as review entrypoints.
- **Documentation changes**: Markdown files from `files[]`.
- **Other changes**: remaining file diffs not captured by the graph, tests, or docs.

Documentation and other file rows render the file basename as a human title without the extension, for example `3003-blockapi-stop-deadlock.md` becomes `3003 Blockapi Stop Deadlock`.

Clicking a graph/test row opens a resizable middle component panel between the PR overview and the graph. The panel shows the component overview first and the `Code` section below it, with extra space above and below the section label. Relation and code line numbers align to the panel's left content edge. The code/diff block itself is transparent so it inherits the panel background; added and removed lines use green and red text without `+` or `-` prefixes. Opening or resizing the panel refits the graph. Selecting a test row temporarily centers the graph on that test entrypoint and the production nodes returned with matching `tests[]` references; selecting any production component returns the canvas to the normal PR diff graph. Clicking a documentation or other file row opens the same middle panel with the file-level GitHub patch.

## API contract used by the code panel

The code panel lazily fetches repo source when there is no PR context:

```http
GET /api/v1/repos/{repoID}/nodes/{nodeID}/code
```

It fetches PR-aware source and diff data when reviewing a PR:

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
  doc_comment?: string;
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted" | "renamed";
}
```

Expected behavior:

- `modified`: may include both `base` and `head`, plus `diff_hunk`.
- `added`: includes `head` when source can be fetched, plus a synthetic `diff_hunk` where every line of the new component is marked as added.
- `deleted`: points at the indexed base `code_nodes` row and uses a component-scoped slice of the GitHub patch based on the stored base line range; source is fetched only if the code panel needs it.
- `renamed`: preserves `old_full_name` / `old_file_path` and uses a component-scoped GitHub patch slice when changed lines are available.
- `modified`: uses a component-scoped slice of the GitHub patch; added component stats are counted from the synthetic component hunk so moved/copied body lines are not undercounted as unchanged context.
- `doc_comment`: cleaned contiguous source comment immediately above the parsed component, if present. The graph API prepends it to `summary` for display without changing AI enrichment.
- Rename detection is conservative: matching body hashes or rename metadata can produce `renamed`, but line overlap alone leaves the head component `added` and any unmatched base component `deleted`.
- Caller/callee context nodes are not PR changes; they show the head version when available.
- The UI reconstructs a full component diff from source only when the required sides are available: both `base` and `head` for `modified` / `renamed`, `head` for `added`, and `base` for `deleted`.
- If source fetching or identity lookup leaves a PR change without the required side, the UI uses `diff_hunk` from graph processing so it does not render a modified component as entirely added or deleted.

## Development debug endpoints

The API includes unauthenticated debug endpoints for rebuilding graph data:

```http
POST /debug/repos/{repoID}/reindex
POST /debug/prs/{prID}/reprocess
POST /debug/prs/{prID}/reprocess/graph
POST /debug/prs/{prID}/reprocess/ai
POST /debug/prs/{prID}/sync
```

These endpoints are idempotent and safe to call during development. `reindex` rebuilds `code_nodes` and `code_edges`, including test nodes and test-to-code edges, from the repository default branch HEAD. `reprocess` runs the full PR path: graph rebuild first, then AI summaries/risk. `reprocess/graph` rebuilds `pr_node_changes`, PR call edges, changed test nodes, and processing metadata, then clears stale AI fields. `reprocess/ai` requires existing `pr_node_changes` and regenerates only AI summaries, test assertion summaries, PR summary, numeric risk, model, payload, and prompt version. `sync` refetches GitHub PR metadata such as title, body/description, author, branch SHAs, and draft/state when a webhook was missed or an upstream copied PR description changed.
