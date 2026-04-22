# Infrastructure

## Overview

| Layer    | Service   | Project / URL                        |
|----------|-----------|--------------------------------------|
| Frontend | Vercel    | `isoprism` → https://isoprism.dev |
| Backend  | Railway   | `isoprism` → https://api.isoprism.dev |
| Database | Supabase  | `Isoprism` (ref: `ixgwhpigkkxpmllzlulc`) |

All three are deployed to production only — no staging environment yet.

---

## Vercel (Frontend)

**Project:** `isoprism` in the `cmwaters-projects` team
**Production URL:** https://isoprism.dev
**Source:** `web/` subdirectory of the `isoprism` GitHub repo
**Root directory** (set in Vercel dashboard): `web/`

### Deploy workflow
Pushes to `main` on GitHub auto-deploy to production via the Vercel GitHub integration. No manual deploy step needed.

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
**Production URL:** https://api.isoprism.dev
**Source:** `api/` subdirectory, built via `railway.toml`

### Deploy workflow
Pushes to `main` on GitHub auto-deploy via Railway's GitHub integration.

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
Migration files live in `db/migrations/`. They use `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` throughout, making them safe to re-run — **except** `CREATE POLICY` statements, which will error if the policy already exists. If in doubt, dump the schema and grep for the table name before re-running.

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
| `NEXT_PUBLIC_SUPABASE_URL` | Vercel project env | Next.js |
| `NEXT_PUBLIC_SUPABASE_ANON_KEY` | Vercel project env | Next.js |
| `SUPABASE_SERVICE_ROLE_KEY` | Vercel project env | Next.js server (if needed) |
| `NEXT_PUBLIC_API_URL` | Vercel project env | Next.js → Go API |
