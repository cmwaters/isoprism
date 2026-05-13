# Isoprism Settings

> Product specification | Updated: 2026-05-13

## 1. Purpose

Settings should be a single operational page. The user should be able to:

- See their GitHub connection
- Manage the Isoprism GitHub App installation on GitHub
- See every repository currently authorized through GitHub
- Index an authorized repository
- Select exactly one indexed repository as the current review workspace
- Uninstall indexed data without removing the repository while GitHub still authorizes it
- See the same indexing status used during onboarding

Settings are not a multi-account admin area during beta. There are no tabs, organization settings pages, members, billing, notification preferences, or access-control controls.

## 2. Route

The settings route remains:

```text
/{user}/settings
```

The callback-safe `/settings` route is a client redirect that resolves the signed-in user's GitHub login and forwards to `/{user}/settings`.

The page should be user-scoped only. Organization-specific settings pages are out of scope for the beta.

## 3. Layout

The page should have one content column with two sections:

1. GitHub connection
2. Repositories

The GitHub connection section should show the signed-in GitHub user and provide one action to manage the GitHub App. Installation happens during onboarding; settings should not show a separate install action because that makes the connected state ambiguous.

The repositories section should show a searchable list of repositories available through the GitHub App installation. Added repositories should appear immediately as authorized but not indexed. Revoked repositories should disappear from this list because GitHub no longer authorizes them.

Each row should show repository name, default branch, selected/indexed/unused state, and the available actions:

- **Index** starts `POST /api/v1/repos/{repoID}/index`.
- **Select** starts `POST /api/v1/repos/{repoID}/select` for an already indexed repository.
- **Open** links to `/{owner}/{repo}` for the selected ready repository.
- **Uninstall** starts `DELETE /api/v1/repos/{repoID}/index`; it schedules indexed data cleanup but keeps the authorized repository visible while GitHub still grants permission.

## 4. Repository Rules

- Only one repository can be selected at a time for both pilot and regular users.
- Pilot users are free-tier users. They can have only one indexed repository in active use. Selecting or indexing a different repository marks the previous selected repository as unused and schedules its indexed data for deletion after one day.
- Regular users can keep multiple indexed repositories. Selecting a different repository changes the current workspace but does not uninstall previously indexed repositories.
- If GitHub revokes access to a repository, Isoprism marks it revoked immediately, removes it from the authorized repository list, and deletes the repository row after one day.
- If GitHub adds access to a repository, Isoprism shows it immediately as authorized but not indexed.
- While indexing runs, the UI shows concrete job progress: phase, percentage, file/node/edge counters where available, and a rough ETA.

## 5. Callback Behavior

The GitHub App callback is available at `/api/v1/github/callback`. It re-syncs GitHub authorization and then decides from Isoprism account state:

- Already setup accounts return to `/settings`, which resolves the signed-in user's `/{user}/settings` route.
- Accounts that have not selected or indexed a repository yet return to `/onboarding/repos`.

The redirect decision must not rely on GitHub's `setup_action`, because settings edits can arrive with values that look like first-time setup. Existing setup is determined from `users.selected_repo_id`, `pilot_users.selected_repo_id`, or an already ready repository.

GitHub may not preserve Isoprism's `state` value when a user edits GitHub App permissions from GitHub settings. In that case the callback resolves the user from the existing installation repositories before syncing and redirecting.

The callback logs a `github_callback` breadcrumb in Railway for production debugging. The log includes query parameter names, `setup_action`, `installation_id`, whether `state` was present and decoded to a user, the user resolution source (`state`, `installation`, or `none`), repository sync counts, and the final redirect. It intentionally does not log the raw `state` value.

## 6. Out of Scope

- Organization settings pages
- Settings categories or tabs
- Self-serve team/member management
- Billing
- Notifications
- Audit logs
