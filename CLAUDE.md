# Git Workflow

This is a single-person prototype. Use only `main` and `preview` branches:

- Develop frontend-only changes on `preview` — no feature branches, no PRs
- Run the web app locally against the deployed production API at `https://api.isoprism.com`
- Merge `preview` into `main` only when frontend details are finalised and ready for production deployment
- API changes are production changes: make them on `main`, then push `main` so Railway deploys the API
- Keep `main` production-ready; do not use it for iterative frontend work
- Never create new branches or open pull requests
- Don't worry about backwards compatibility or legacy code

# Development flow

- Can you make sure that all documentation is updated to reflect changes in the code. I should be able to use the documentation as a reliable source of the truth
- Default frontend development uses the hosted API and the single production GitHub App:
  - Web: from `web/`, run `NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev`
  - Open the local web app at `http://localhost:3000`
- Railway API should keep `FRONTEND_URL=https://isoprism.com` and `FRONTEND_URLS=https://isoprism.com,http://localhost:3000` so both production web and local web can use the same API/GitHub App
- For API work only, run the API locally when needed from `api/` with `go run ./cmd/api` (defaults to `http://localhost:8080`), then push the API change to `main`
- After local frontend verification passes, commit and push web changes to `preview`
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
