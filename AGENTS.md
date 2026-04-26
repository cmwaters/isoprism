# Git Workflow

This is a single-person prototype. Use only `main` and `preview` branches:

- Commit directly to `main` — no feature branches, no PRs
- `preview` mirrors `main` (fast-forward only); sync it by merging `main` into `preview` and pushing both
- Never create new branches or open pull requests
- Don't worry about backwards compatibility or legacy code

# Development flow

- Can you make sure that all documentation is updated to reflect changes in the code. I should be able to use the documentation as a reliable source of the truth
- After local verification passes, commit and push the changes to `main` so delivery can be verified through the deployment at https://isoprism.com

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
