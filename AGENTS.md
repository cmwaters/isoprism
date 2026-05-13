# Git Workflow

This is a single-person prototype. Use `main` for all development and production changes for now:

- Develop frontend and API changes directly on `main` — no feature branches, no PRs
- Run the web app locally against the deployed production API at `https://api.isoprism.com`
- Push `main` when changes are verified; Vercel deploys the web app and Railway deploys API changes
- Keep `preview` only as a synced mirror of `main` while any external tooling still expects it
- Never create new branches or open pull requests
- For parallel local ideas, short-lived local-only worktree branches are allowed because Git requires each worktree to have a distinct checked-out branch. Merge verified work back into `main`, push `main`, then delete the local branch/worktree.
- Don't worry about backwards compatibility or legacy code

# Development flow

- Can you make sure that all documentation is updated to reflect changes in the code. I should be able to use the documentation as a reliable source of the truth
- Default frontend development uses the hosted API and the single production GitHub App:
  - Web: from `web/`, run `NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev`
  - Open the local web app at `http://localhost:3000`
- Railway API should keep `FRONTEND_URL=https://isoprism.com` and `FRONTEND_URLS=https://isoprism.com,http://localhost:3000` so both production web and local web can use the same API/GitHub App
- For API work, run the API locally when needed from `api/` with `go run ./cmd/api` (defaults to `http://localhost:8080`), then push the API change to `main`
- After local verification passes, commit and push changes to `main`
- Use the hosted deployment at https://isoprism.com for final verification after `main` deploys

## Codex worktrees

Use the repo scripts when running multiple ideas in parallel:

```
scripts/codex-worktree-setup.sh
scripts/codex-worktree-setup.sh graph-context 3001
scripts/codex-worktree-setup.sh onboarding-polish 3002
```

In Codex Desktop, use `scripts/codex-worktree-setup.sh` as the project setup script. Codex provides `CODEX_SOURCE_TREE_PATH` and `CODEX_WORKTREE_PATH`; the script configures that worktree, copies local env files when present, installs `web/` dependencies, and writes `.worktree.env` with `NEXT_PUBLIC_API_URL=https://api.isoprism.com`.

When run manually without `CODEX_WORKTREE_PATH`, the setup script creates a local Git worktree under `$HOME/.codex/worktrees/<thread-and-idea>/isoprism`.

Supported overrides:

```
CODEX_SOURCE_TREE_PATH=/Users/callum/Developer/isoprism
CODEX_WORKTREE_PATH=/Users/callum/.codex/worktrees/example/isoprism
CODEX_WORKTREE_PARENT=/Users/callum/.codex/worktrees
CODEX_WORKTREE_SLUG=my-session
CODEX_WORKTREE_BRANCH=idea/my-idea
CODEX_WEB_PORT=3003
CODEX_API_URL=https://api.isoprism.com
```

After a worktree branch has been merged into `main`, clean it up with:

```
scripts/codex-worktree-cleanup.sh
scripts/codex-worktree-cleanup.sh graph-context
```

# Debug tooling

Debug endpoints exist on the API (no auth required) for development:

- `POST /debug/repos/{repoID}/reindex` — re-runs RepoInit (rebuilds code_nodes + code_edges, including test nodes/edges, from the repository default branch HEAD)
- `POST /debug/prs/{prID}/reprocess` — full PR reprocess: rebuilds graph data, then reruns AI summaries/risk
- `POST /debug/prs/{prID}/reprocess/graph` — graph-only PR reprocess: rebuilds pr_node_changes, PR call edges, test node changes, and latest processing metadata while preserving existing AI summaries for unchanged node IDs
- `POST /debug/prs/{prID}/reprocess/ai` — AI-only PR reprocess: requires existing pr_node_changes and regenerates summaries/risk without rebuilding graph data

These are safe to call at any time; they are idempotent (upserts on conflict).

## DB access

Supabase project: `sampxhpwbxvyphprnqtc` (eu-west-1)
Service role key is in `api/.env` — use it with the Supabase REST API or direct psql:

```
DATABASE_URL=postgresql://postgres.sampxhpwbxvyphprnqtc:<password>@aws-0-eu-west-1.pooler.supabase.com:6543/postgres
```

Supabase REST base URL: `https://sampxhpwbxvyphprnqtc.supabase.co/rest/v1/`
