# Cloud Storage

## Overview

Cloud storage lives in Supabase Postgres. GitHub and Supabase Auth provide external identity and source data; Postgres stores Isoprism's indexed graph, PR processing state, pilot data, and AI outputs.

## Main Tables

### Users

`users` mirrors Supabase Auth users and stores profile/GitHub identity fields. The user ID matches `auth.users.id`.

### GitHub Installations

`github_installations` stores GitHub App installation metadata such as installation ID, account login, account type, and avatar URL.

### Repositories

`repositories` stores authorized repositories for each user:

- GitHub repo ID and full name
- default branch
- latest indexed default-branch SHA
- index status
- GitHub access status
- selected/indexed/unused timestamps

The cloud product uses repository IDs from Postgres. The CLI should not reuse or fake these identifiers.

### Indexing Jobs

`indexing_jobs` tracks repository indexing progress for a repo and commit SHA:

- status
- phase
- message
- file/node/edge counters
- started/updated/finished timestamps
- error

### Pull Requests

`pull_requests` stores PR metadata from GitHub webhooks and syncs:

- PR number and GitHub PR ID
- title/body/author
- base/head branches and SHAs
- state/draft/opened/merged timestamps
- graph status
- processing metadata and errors

### Code Nodes

`code_nodes` stores parsed semantic components at a repo commit:

- full name
- file path
- line range
- inputs and outputs
- language and kind
- body hash
- doc comment
- test and entrypoint flags

Rows are commit-scoped. The same logical component at two commits is represented by two rows.

### Code Edges

`code_edges` stores semantic relationships at a repo commit:

- `calls`
- `owns_method`
- `uses_type`

Test nodes and edges are stored but default graph responses filter tests out unless showing test-focused context.

### PR Node Changes

`pr_node_changes` stores semantic components changed by a PR. Changes may be:

- added
- modified
- deleted
- renamed

Rows may include diff hunks, line counts, old name/path for renames, and AI change summaries.

### PR Analyses

`pr_analyses` stores PR-level AI output:

- summary
- numeric risk score
- nodes changed
- model
- prompt version
- raw structured payload
- generated timestamp

## Migrations

Migrations live in:

```text
supabase/migrations/
```

The API fails fast unless the latest row in `supabase_migrations.schema_migrations` matches `api/internal/db.RequiredMigrationVersion`. When adding a migration, update the required version and tests in the same change.

## Data Lifecycle

Pilot users have one active indexed repository. Selecting a new repository marks the previous indexed repository unused and schedules cleanup. Revoked GitHub access marks repositories revoked and removes them from active lists.
