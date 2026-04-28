# Git Workflow

This is a single-person prototype. Use only `main` and `preview` branches:

- Develop on `preview` — no feature branches, no PRs
- Commit and push in-progress work to `preview`
- Merge `preview` into `main` only when the details are finalised and the change is ready for production deployment
- Keep `main` production-ready; do not use it for iterative development
- Never create new branches or open pull requests
- Don't worry about backwards compatibility or legacy code

# Development flow

- Can you make sure that all documentation is updated to reflect changes in the code. I should be able to use the documentation as a reliable source of the truth
- Run development locally before relying on hosted deployments:
  - API: from `api/`, run `PORT=8000 go run ./cmd/api`
  - Web: from `web/`, run `NEXT_PUBLIC_API_URL=http://localhost:8000 npm run dev`
  - Open the local web app at `http://localhost:3000`
- After local verification passes, commit and push changes to `preview`
- Use the hosted deployment at https://isoprism.com only for final verification after `preview` has been merged into `main`

# Debug tooling

Debug endpoints exist on the API (no auth required) for development:

- `POST /debug/repos/{repoID}/reindex` — re-runs RepoInit (rebuilds code_nodes + code_edges + code_test_references from main branch HEAD)
- `POST /debug/prs/{prID}/reprocess` — re-runs OpenPR (rebuilds pr_node_changes + call edges + changed-file test references for a PR)

These are safe to call at any time; they are idempotent (upserts on conflict).

## DB access

Supabase project: `sampxhpwbxvyphprnqtc` (eu-west-1)
Service role key is in `api/.env` — use it with the Supabase REST API or direct psql:

```
DATABASE_URL=postgresql://postgres.sampxhpwbxvyphprnqtc:<password>@aws-0-eu-west-1.pooler.supabase.com:6543/postgres
```

Supabase REST base URL: `https://sampxhpwbxvyphprnqtc.supabase.co/rest/v1/`
