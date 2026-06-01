# Cloud UI

## Scope

Cloud UI is the hosted Next.js application. It owns authentication, pilot registration, onboarding, settings, admin, feedback, and the cloud graph workspace.

## Routes

Key product routes:

```text
/                         public WIP landing or signed-in redirect
/pilot/register           pilot registration form
/pilot/{token}            invite entry
/pilot/review/{token}     end-of-pilot questionnaire
/auth/callback            Supabase auth callback
/onboarding/repos         repository selection/indexing
/{owner}/{repo}           graph workspace
/settings                 repository settings
/admin                    pilot admin console
```

## Onboarding

Cloud onboarding includes:

- GitHub sign-in
- GitHub App installation/authorization
- repository selection
- indexing progress

Indexing progress should use the same status contract as settings:

```text
GET /api/v1/repos/{repoID}/status
```

## Settings

`/settings` is a dedicated route, not an overlay. It lets the signed-in user:

- view GitHub connection state
- manage the GitHub App installation
- view authorized repositories
- select the current review workspace
- start or retry indexing
- see indexing progress

Settings should stay operational and minimal. No teams, billing, notification preferences, org settings, or audit logs during beta.

## Graph Workspace

The cloud graph workspace should use common React components for:

- repo/program context
- optional review item list
- graph canvas
- node cards
- node detail panel
- code and diff rendering
- graph expansion controls

Cloud supplies review items from the PR queue. Local may supply none, local diffs, staged diffs, or future `gh` PRs. The shared UI should not assume review items always exist.

## Cloud-Unique Components

Cloud-only components include:

- pilot registration form
- pilot invite/login copy
- onboarding repo selection
- indexing progress shell
- settings view
- pilot admin console
- beta feedback banner and feedback modal
- auth/session guards

## Review Items

In cloud, review items are currently open PRs returned by:

```text
GET /api/v1/repos/{repoID}/queue
```

The UI may rank and display:

- PR number
- title
- author
- open time
- additions/deletions
- changed-node count
- AI summary
- risk score/derived label

## Feedback

Authenticated graph views show a compact beta feedback banner with "Report a problem" and "Request a feature". Submissions call:

```text
POST /api/v1/beta/feedback
```

The created GitHub issue includes user, repo, PR, selected node, browser path, app commit, and source commit context.
