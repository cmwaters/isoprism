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

The pilot starts at `/pilot/register`. Prospective testers submit the registration form, including their software experience, what parts of their software AI does not write, their process before generating prompts, how they currently review code, how much of their work is spent reviewing code, review pain points, how they use AI to review software, issue types they do or would resolve without a human in the loop, whether they review AI-written software differently, and whether they want to pilot Isoprism for one week with one repository.

Admins review registrations at `/admin`, generate an access code, and send the invite email through Mailtrap. The invite link goes to `/pilot/{token}`, which forwards into the login/GitHub setup flow. A review email can only be sent after the invite has been generated and the pilot user has registered with GitHub; it goes to `/pilot/review/{token}` and submits the end-of-pilot review form.

The current root route is pilot-first: unauthenticated visitors to `/` see a work-in-progress page with a link to `/pilot/register`. Signed-in visitors only skip that page when `GET /api/v1/auth/status?user_id=...` returns a ready repo (`/{owner}/{repo}`) or an installed-but-unindexed repo (`/onboarding/repos`). If auth status returns `/onboarding`, root maps that back to the work-in-progress page; the OAuth callback keeps `/onboarding` so invited pilot users who authenticated through `/pilot/{token}` are prompted to install the GitHub App and grant repo permissions. GitHub App installation/settings callbacks use `/api/v1/github/callback` and decide from Isoprism setup state whether to return the user to `/settings` or `/onboarding/repos`.

The GitHub OAuth login requests `read:user`, `user:email`, and `read:org`. The org scope lets the settings UI discover private organization memberships when GitHub exposes them to the signed-in user's OAuth token.

## Settings

Authenticated views link to `/settings` from the graph side panel. The settings entry is a left-aligned button without a pill background, and it opens the dedicated settings page instead of an in-graph overlay.

The graph side panel uses a Next route link so the settings route can be prefetched while the repo page is open. Repo pages also warm a short-lived client cache for `GET /api/v1/me/repos`; settings renders from that cache immediately when available, then refreshes it through the same API call.

Settings are intentionally simple during beta. `/settings` is a dedicated Manage Repositories page for the signed-in user, with a left panel showing that person and a back link to the repo view. The tester can:

- See their GitHub connection
- Install or manage the Isoprism GitHub App on GitHub
- See the current indexed repository
- Select a different accessible repository, which automatically starts indexing when needed
- Open the selected repository once indexing is ready

When a different repository is selected from settings and needs indexing, the selected row shows the same progress bar, status message, counters, and ETA copy used during onboarding.

- `GET /api/v1/me/repos` supplies repositories currently available to Isoprism.
- `POST /api/v1/repos/{repoID}/index` selects the repository and starts or retries indexing from the repositories settings category.

GitHub App install and permission changes still happen on GitHub.

## Pilot admin

The pilot admin console is available at `/admin`. It prompts for the admin password and then calls the Go API with `X-Admin-Password`.

Admin capabilities:

- Review Registration and Review forms as question-and-response text using the same question wording as the live forms.
- Add pilot users manually with a name and optional email.
- Generate an access code and send the pilot invite email through Mailtrap.
- Copy generated invite links from the pilot user details.
- Track started pilots by setup date, selected repo, and submitted issue/feature counts. The selected repo is updated when the pilot user indexes a repo and replaces the requested public repo from registration. Repository names are shown without the `https://github.com/` prefix and selected repositories link to an admin repo viewer for exploring that pilot user's PRs; requested public repos fall back to GitHub links.
- Open the actual review form link that is sent by email when an invite token exists.
- Jump from a pilot user to their Registration Form or submitted Review Form from the user action row. The Review Form button stays disabled until that pilot user has submitted the review form.
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
- A ranked PR list in the repo overview panel. Each PR card shows the PR number in the graph accent pink, title, author pill, colored addition/deletion pills, a risk label derived from numeric `risk_score` where used, and a client-updated compact open-time badge (`5h`, `3d`, `1w`).
- In-place PR graph loading when a reviewer clicks a PR card. The URL stays `/{owner}/{repo}`.
- A small client-side cache so previously opened PR graphs reappear without another fetch.
- A side panel that reviewers can resize between bounded minimum and maximum widths.
- Node cards with package/type labels, function or method titles, structured inputs/outputs, and added/removed/deleted pills.
- Production nodes only; test code is indexed separately and shown as tests attached to the production nodes it exercises.
- PR graphs initially show changed production nodes plus exactly one hop of production callers/callees in either direction. Context-to-context nodes beyond that first hop are not included until the reviewer expands the graph.
- Clicking a visible PR graph edge requests one additional one-hop production neighborhood around that edge's endpoints and merges the returned nodes/edges into the current graph session.
- Clicking a function or method node expands that component into the visible graph. Clicking a type/class node loads its methods and referenced type contents into the detail panel only; those methods or referenced types are added to the graph only when the reviewer clicks them from the panel.
- Graph placement uses a deterministic edge-length relaxation layout. Visible edges pull connected nodes together, larger collision padding keeps cards readable, unconnected nodes repel each other, and existing node positions are preserved during expansion so the map does not reshuffle abruptly.
- Global function/class/package collapse controls are not part of the current graph UI. Future collapse should be per-component so a reviewer can collapse or expand a specific component cluster without changing the whole map.

During beta, the PR queue only includes open, non-draft PRs targeting the repository's indexed default branch whose `base_commit_sha` exactly matches the repository's indexed `main_commit_sha`. PRs from other base branches, or PRs whose base SHA is out of sync with the indexed default-branch graph, are hidden rather than shown with approximate graph data.

The API also skips oversized PRs before expensive graph processing. The beta limits are 300 changed files, 20,000 additions, 20,000 deletions, or 30,000 total changed lines. Skipped PRs get `graph_status = "skipped"` and a `pr_analyses.summary` explaining which size limit was exceeded.

Large PRs can skip per-function AI summaries so the graph and changed-node overlay still become available without waiting on an oversized enrichment request. AI enrichment is PR-only and uses Gemini; repo indexing does not generate base code-node summaries.

Each PR stores the latest processing snapshot on `pull_requests`: `processor_commit_sha`, `processed_at`, `processing_error`, and `processing_stats`. The stats JSON records file counts, supported-file counts, parsed node counts, detected semantic changes, persisted `pr_node_changes`, and call-edge persistence counters so empty PR views can be debugged without guessing which deployed API revision processed them.

Graph responses are function-level plus type nodes. The API returns canonical `full_name` values, but the UI splits them into a pink package/receiver label and a black title that contains only the function or method name. Function and method nodes expose `inputs[]`/`outputs[]` as structured `{name?, type, node_id?}` records. Struct field type relationships are represented as `uses_type` graph edges from the struct node to the indexed type node so they participate in expansion and boundary behavior.

PR graph responses still use `file_path + full_name` as the semantic identity for function-level visible nodes. If the same function exists as both an indexed-main node and a PR-head node, the API collapses them into one visual node and prefers the changed PR-head metadata. Edges are rewritten after this collapse, and test nodes are filtered from the visible graph; tests remain available through each production node's `tests[]` detail. Dynamic expansion uses `POST /api/v1/repos/{repoID}/graph/expand` with PR graph context; edge clicks issue that expansion for both edge endpoints and merge the returned `nodes` and `edges`.

Graph nodes can be expanded from the canvas. Clicking a function or method selects it and, on the first click for that visible node, calls `POST /api/v1/repos/{repoID}/graph/expand` with the selected node ID, current visible node IDs, and either repo context or PR context (`{ mode: "pr", pr_id }`). The API returns hidden direct callers/callees plus visible edges for the expanded set. The client merges nodes by `id`, merges edges by `source_id|destination_id|edge_kind`, keeps existing node positions stable, preserves the selected-node highlight across the node rebuild, places newly loaded nodes near the expanded node before the edge-length relaxation pass, and preserves the current camera viewport instead of refitting. Selecting another node animates to that component at the current zoom level.

Type/class nodes use the same expansion endpoint but initially keep the result in panel-only detail state. The side panel shows a `Contents` tab for type nodes instead of source code. Contents lists methods from `owns_method` edges first, used field types from `uses_type` edges next, then any type references resolved from structured `inputs[]` and `outputs[]`; clicking a row materializes that component in the current graph session and wires it to already visible edges where available. Owned methods and used types are not drawn on the canvas until the reviewer clicks a row.

PR review does not have a separate route or page.

During the beta, this repository is the tester's selected trial repository. Feedback controls for bug reports and feature requests should be available from this review workspace and should capture current context such as repository, PR number, selected node, and browser path.

The graph workspace shows a black beta footer banner with "Report a problem" and "Request a feature" actions, plus a dismiss button. Dismissal is page-session only, so the banner reappears after a full browser refresh. Each feedback action opens a centered feedback form and submits to:

```http
POST /api/v1/beta/feedback
```

The API creates a GitHub issue in the configured feedback repository with either the `bug` or `feature` label. The payload includes repo/PR/node context, browser path, frontend app commit, and source commit.

The side panel has two modes:

- `Overview`: repo, PR, or node summary, calls, callers, and tests. PR context shows GitHub-equivalent file diffs from the PR files API, including test files, plus a separate semantic graph-change list when graph nodes are available.
- `Code`: a lazy-loaded source viewer for the selected function or method. PR changed nodes automatically show the full component with changed lines highlighted. Repo nodes and unchanged context nodes show plain source.
- `Contents`: shown for type/class nodes instead of `Code`. It lists owned methods, used field types, and other type references, and lets the reviewer add a resolved referenced type to the current graph on demand.

The overview/code/contents icon controls switch the side panel mode without changing the selected graph node.

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

Opening a PR shows the AI PR summary as the overview description. If no AI summary exists, the overview falls back to the PR body. When an AI summary is shown, the summary ends with a `View PR Description` action that opens the complete PR body in the same resizable middle panel used for components. That panel is titled `PR Description` and links to the PR on Github at the bottom with `Open on Github`. If the PR body references a GitHub issue through an issue URL, `owner/repo#123`, or a same-repo `fixes #123` / `closes #123` / `resolves #123` style reference, a right-aligned `View Issue` action appears beside it and fetches the issue description from GitHub into that middle panel. Closed issue badges render red, issue metadata uses pill badges, issue links also use `Open on Github`, and loaded issue descriptions are cached for the current graph workspace session.

Clicking a graph/test row opens a resizable middle component panel between the PR overview and the graph. Function and method components show the component overview first and the `Code` section below it, with extra space above and below the section label. Type/class components do not show source code in this panel; they show relation sections plus `Contents` for referenced types. Relation and code line numbers align to the panel's left content edge. The code/diff block itself is transparent so it inherits the panel background; added and removed lines use green and red text without `+` or `-` prefixes. Selecting a test row opens a focused test graph centered on that test entrypoint and the production nodes returned with matching `tests[]` references. While the focused test graph is open, selecting or expanding production methods keeps additions inside that test graph instead of returning to the normal PR diff graph. Clicking a documentation or other file row opens the same middle panel with the file-level GitHub patch.

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
- Caller/callee and receiver-owner context nodes are not PR changes; they show the head version when available.
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
