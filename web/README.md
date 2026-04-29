# Isoprism Web

Isoprism is a PR review interface that turns code changes into a semantic call graph. Reviewers can inspect changed functions, structs, and nearby callers/callees, then switch between summary-first review and source/diff review for a selected node.

## Local development

```bash
npm install
NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev
```

The web app runs on [http://localhost:3000](http://localhost:3000). It talks to the Railway Go API at `NEXT_PUBLIC_API_URL`, defaulting to `https://api.isoprism.com`.

Local frontend development uses the single production GitHub App and the deployed API. GitHub App installs started from localhost redirect back to localhost through the encoded install `state`, as long as the Railway API includes `http://localhost:3000` in `FRONTEND_URLS`.

If an API change is required, make that change on `main`, deploy it to Railway, then continue frontend iteration on `preview`.

Useful checks:

```bash
npm run lint
npm run build
```

## Auth routing

The intended beta route is invite-first: testers receive a unique link containing an access token, connect GitHub, authorize the Isoprism GitHub App, and select one repository for a one-week trial.

The current root route is login-first: unauthenticated visitors go to `/login`, and signed-in visitors only skip login when `GET /api/v1/auth/status?user_id=...` returns a ready repo (`/{owner}/{repo}`) or an installed-but-unindexed repo (`/onboarding/repos`). If auth status returns `/onboarding`, root maps that to `/login`; the OAuth callback keeps `/onboarding` so newly signed-in GitHub users without a connected Isoprism repo are prompted to install the GitHub App and grant repo permissions.

To fully match the beta loop, routing should preserve invite context across OAuth and GitHub App installation, reject invalid or completed beta tokens, lock the tester to one selected repository, show trial status for seven days, and ask for the questionnaire once the trial window ends.

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

## Beta admin

The beta admin console is available at `/admin`. It prompts for the admin password and then calls the Go API with `X-Admin-Password`.

Admin capabilities:

- Create a beta tester by name, with an optional email/note.
- Generate a unique beta ID, raw token, and invite link.
- Show the raw token and full link only immediately after creation.
- Monitor invite status, whether the invite has been used, selected repository, trial dates, and questionnaire answers.

The API routes are:

```http
GET  /api/v1/admin/beta/testers
POST /api/v1/admin/beta/testers
```

The Railway API must have `ADMIN_PASSWORD` set before this page can unlock.

## Repository Graph View

The primary review route mirrors the GitHub repository path:

- `/{owner}/{repo}` fetches `GET /api/v1/repos/{repoID}/graph` plus `GET /api/v1/repos/{repoID}/queue`.

The repo route renders one persistent `GraphCanvas` and side panel:

- A repo graph for the whole indexed system.
- A ranked PR list in the repo overview panel. Each PR card shows the PR number, title, AI summary, changed-function count, risk label, and a client-updated open-time badge.
- In-place PR graph loading when a reviewer clicks a PR card. The URL stays `/{owner}/{repo}`.
- A small client-side cache so previously opened PR graphs reappear without another fetch.
- A side panel that reviewers can resize between bounded minimum and maximum widths.
- Node cards with package/type labels, signatures, and added/removed/deleted pills.
- Production nodes only; test code is indexed separately and shown as tests attached to the production nodes it exercises.

PR review does not have a separate route or page.

During the beta, this repository is the tester's selected trial repository. Feedback controls for bug reports and feature requests should be available from this review workspace and should capture current context such as repository, PR number, selected node, and browser path.

The graph workspace shows a black beta footer banner with "Report a problem" and "Request a feature" actions. Each action opens a centered feedback form and submits to:

```http
POST /api/v1/beta/feedback
```

The API creates a GitHub issue in the configured feedback repository with either the `bug` or `feature` label. The payload includes repo/PR/node context, browser path, frontend app commit, and source commit.

The side panel has two modes:

- `Overview`: repo, PR, or node summary, calls, callers, and tests. PR context additionally shows change explanations and diff stats.
- `Code`: a lazy-loaded source viewer for the selected function or struct. PR changed nodes automatically show the full component with changed lines highlighted. Repo nodes and unchanged context nodes show plain source.

The overview/code icon controls switch the side panel mode without changing the selected graph node.

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
  diff_hunk?: string;
  change_type?: "added" | "modified" | "deleted";
}
```

Expected behavior:

- `modified`: may include both `base` and `head`, plus `diff_hunk`.
- `added`: includes `head` when source can be fetched, plus a synthetic `diff_hunk` where every line of the new component is marked as added.
- `deleted`: includes `base` when source can be fetched, plus a synthetic `diff_hunk` where every line of the removed component is marked as deleted.
- `modified`: uses a component-scoped slice of the GitHub patch; added/deleted component stats are counted from the synthetic component hunks so moved/copied body lines are not undercounted as unchanged context.
- Caller/callee context nodes are not PR changes; they show the head version when available.
- If GitHub source fetching fails, the UI still uses `diff_hunk` from graph processing when present.

## Development debug endpoints

The API includes unauthenticated debug endpoints for rebuilding graph data:

```http
POST /debug/repos/{repoID}/reindex
POST /debug/prs/{prID}/reprocess
```

Both endpoints are idempotent and safe to call during development. `reindex` rebuilds `code_nodes`, `code_edges`, and `code_test_references` from the repository default branch HEAD. `reprocess` rebuilds `pr_node_changes`, PR call edges, and changed-file test references for one PR.
