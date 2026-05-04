# Infrastructure

## Overview

| Layer    | Service   | Project / URL                        |
|----------|-----------|--------------------------------------|
| Frontend | Vercel    | `isoprism` → https://isoprism.com |
| Backend  | Railway   | `isoprism` → https://api.isoprism.com |
| Database | Supabase  | `Isoprism` (ref: `ixgwhpigkkxpmllzlulc`) |
| GitHub App | GitHub | Single production app used by local web, API, and production web |

The API and GitHub App are production-only. Frontend iteration happens locally on `preview` while pointing at the deployed Railway API.

---

## Vercel (Frontend)

**Project:** `isoprism` in the `cmwaters-projects` team
**Production URL:** https://isoprism.com
**Source:** `web/` subdirectory of the `isoprism` GitHub repo
**Root directory** (set in Vercel dashboard): `web/`

### Deploy workflow
Pushes to `main` on GitHub auto-deploy to production via the Vercel GitHub integration. Develop frontend changes locally on `preview`; merge `preview` into `main` only when the UI is ready to ship.

### CLI

```bash
# List recent deployments
vercel ls isoprism

# Inspect a specific deployment
vercel inspect <deployment-url>

# Tail logs for a deployment
vercel logs <deployment-url>

# Manual production deploy (from web/ directory, fallback only)
cd web && vercel --prod
```

> **Note:** The local `.vercel/project.json` in `web/` must point to project `isoprism`
> (`prj_<isoprism-id>`), not to the `web` project. If auto-deploy breaks, check this file first.

---

## Railway (Backend — Go API)

**Project:** `isoprism`
**Service:** `isoprism`
**Production URL:** https://api.isoprism.com
**Source:** `api/` subdirectory, built via `railway.toml`

### Deploy workflow
Pushes to `main` on GitHub auto-deploy via Railway's GitHub integration. API changes are production changes and should be made on `main`.

### GitHub App

Use one production GitHub App for both local frontend development and production. The app webhook URL points at the Railway API:

```text
https://api.isoprism.com/webhooks/github
```

When developing the web app locally, the install link sends the current frontend origin through GitHub's `state` parameter. The Railway API only redirects back to origins listed in `FRONTEND_URLS`, so one production GitHub App can serve both `https://isoprism.com` and `http://localhost:3000`.

Recommended Railway values:

```text
FRONTEND_URL=https://isoprism.com
FRONTEND_URLS=https://isoprism.com,http://localhost:3000
```

### CLI

```bash
# Link to the project/service (run once per machine)
railway link   # select isoprism project → isoprism service

# View live logs
railway logs --service isoprism

# Check deployment status
railway status

# Open the Railway dashboard
railway open

# Run a one-off command inside the service environment (e.g. check env vars)
railway run env | grep DATABASE
```

### Build config (`railway.toml`)

The API parser uses tree-sitter grammar bindings, so builds require CGO and a C compiler. Railway's Nixpacks Go builder provides the needed toolchain; do not force `CGO_ENABLED=0` for API builds.

```toml
[build]
builder = "NIXPACKS"
buildCommand = "cd api && go build -o /app/server ./cmd/api"

[deploy]
startCommand = "/app/server"
healthcheckPath = "/health"
healthcheckTimeout = 30
restartPolicyType = "ON_FAILURE"
restartPolicyMaxRetries = 3
```

---

## Supabase (Database + Auth)

**Project:** `Isoprism`
**Reference ID:** `ixgwhpigkkxpmllzlulc`
**Region:** Central EU (Frankfurt)
**Organisation:** `enbdekxbrxcqlcuyrdpc`

### CLI

```bash
# List projects (confirm linked project)
supabase projects list

# Dump remote schema (useful to verify migrations were applied)
supabase db dump --linked

# Push local migrations to production
supabase db push --linked

# Pull remote schema into local migration history
supabase db pull --linked

# Check migration diff
supabase db diff --linked
```

### Migrations
Migration files live in `supabase/migrations/` so the Supabase CLI can discover them without a temporary workdir. They use `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` throughout, making them safe to re-run — **except** `CREATE POLICY` statements, which will error if the policy already exists. If in doubt, dump the schema and grep for the table name before re-running.

If the CLI is not logged in, use the production database URL from `api/.env`:

```bash
DATABASE_URL=$(sed -n 's/^DATABASE_URL=//p' api/.env)
supabase db push --db-url "$DATABASE_URL"
```

```bash
# Quick check — did migration 002 apply?
supabase db dump --linked | grep -E "pr_check_runs|pr_review_threads|pr_review_requests"
```

---

## Environment Variables

| Variable | Where set | Used by |
|----------|-----------|---------|
| `DATABASE_URL` | Railway service env | Go API → pgxpool |
| `GITHUB_APP_ID` | Railway service env | Go API → GitHub App |
| `GITHUB_APP_PRIVATE_KEY` | Railway service env | Go API → GitHub App |
| `GITHUB_WEBHOOK_SECRET` | Railway service env | Go API → webhook validation |
| `FRONTEND_URL` | Railway service env | Go API → default OAuth/install redirect |
| `FRONTEND_URLS` | Railway service env | Go API → allowed CORS origins and allowed install redirect origins |
| `ANTHROPIC_API_KEY` | Railway service env | Go API → AI enrichment for code summaries, PR change summaries, and PR analyses |
| `OPENAI_API_KEY` | Railway service env | Go API config only; currently unused by AI call sites |
| `GITHUB_FEEDBACK_TOKEN` | Railway service env | Go API → creates GitHub issues for beta bug reports and feature requests |
| `GITHUB_FEEDBACK_REPO` | Railway service env | Go API → `owner/repo` target for beta feedback issues |
| `ADMIN_PASSWORD` | Railway service env | Go API → password gate for `/api/v1/admin/beta/testers` |
| `NEXT_PUBLIC_SUPABASE_URL` | Vercel project env | Next.js |
| `NEXT_PUBLIC_SUPABASE_ANON_KEY` | Vercel project env | Next.js |
| `SUPABASE_SERVICE_ROLE_KEY` | Vercel project env | Next.js server (if needed) |
| `NEXT_PUBLIC_API_URL` | Vercel project env and local shell | Next.js → Go API; defaults to `https://api.isoprism.com` |
| `NEXT_PUBLIC_VERCEL_GIT_COMMIT_SHA` | Vercel system env | Next.js → included in beta feedback issues as the app commit |
