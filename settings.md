# Isoprism Settings

> Beta specification | Updated: 2026-04-29

## 1. Purpose

Settings should be a single operational page. The beta tester should be able to:

- See their GitHub connection
- Manage the Isoprism GitHub App installation on GitHub
- See the current repository Isoprism is using
- Select a different accessible repository
- Index that different repository and see the same indexing status used during onboarding

Settings are not a multi-account admin area during beta. There are no tabs, organization settings pages, members, billing, notification preferences, or access-control controls.

## 2. Route

The settings route remains:

```text
/{user}/settings
```

The page should be user-scoped only. Organization-specific settings pages are out of scope for the beta.

## 3. Layout

The page should have one content column with three sections:

1. GitHub connection
2. Current repository
3. Swap repository

The GitHub connection section should show the signed-in GitHub user and provide one action to manage the GitHub App. Installation happens during onboarding; settings should not show a separate install action because that makes the connected state ambiguous.

The current repository section should show the single repository currently indexed for the beta and provide a link back to the graph page.

The swap repository section should show a searchable list of repositories available through the GitHub App installation. Selecting a repository and clicking "Index selected repo" should trigger indexing. While indexing runs, the page should show the same `IndexingProgress` experience used during onboarding.

## 4. Repository Swapping

Swapping repositories means:

- The user selects one repository from the accessible repository list.
- Isoprism triggers `POST /api/v1/repos/{repoID}/index`.
- The UI shows indexing status until the repo is ready or failed.
- Once ready, the user is sent to `/{owner}/{repo}`.

For the beta, only one repository should be considered the active trial repository at a time. The backend should persist that active choice once beta invite tables exist.

## 5. Out of Scope

- Organization settings pages
- Settings categories or tabs
- Self-serve team/member management
- Billing
- Notifications
- Audit logs
- Multiple active repositories per beta tester
