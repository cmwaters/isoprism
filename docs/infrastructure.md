# Infrastructure

## Overview

Isoprism has two product runtimes:

- CLI: local Go daemon plus embedded browser viewer, running inside a user's git checkout.
- Cloud: hosted Next.js frontend, Railway Go API, Supabase Postgres/Auth, GitHub App, and Gemini PR analysis.

The CLI and cloud products share graph/UI concepts, but they do not share one HTTP API design.

## CLI Runtime

The CLI binary contains:

- local graph/diff command implementation
- local daemon server
- embedded React viewer assets
- tree-sitter-backed parser and graph builder

Default local URL:

```text
http://127.0.0.1:3717/local
```

Default cache:

```text
.isoprism/
```

Normal installed users should not need:

- Node.js
- Vercel
- Railway
- Supabase credentials
- GitHub OAuth
- the Isoprism source checkout

The CLI may use `gh` in the future to pull PR metadata into local review items. That integration belongs behind the local daemon API, not in React components.

## Cloud Runtime

| Layer | Service | Project / URL |
|---|---|---|
| Frontend | Vercel | `isoprism` -> https://isoprism.com |
| Backend | Railway | `isoprism` -> https://api.isoprism.com |
| Database/Auth | Supabase | project `sampxhpwbxvyphprnqtc` |
| GitHub App | GitHub | single production app |
| AI | Gemini API | `gemini-2.5-flash` |

## Development Flow

Development happens on `main` for now. After local verification, commit and push `main`; Vercel deploys the web app and Railway deploys API changes. Keep `preview` only as a synced mirror while external tooling still expects it.

Default frontend development uses the hosted API:

```bash
cd web
NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev
```

Open:

```text
http://localhost:3000
```

For API work:

```bash
cd api
go run ./cmd/api
```

## Vercel

Project: `isoprism`

Production URL:

```text
https://isoprism.com
```

Source directory:

```text
web/
```

Pushes to `main` auto-deploy through Vercel's GitHub integration.

Useful commands:

```bash
vercel ls isoprism
vercel inspect <deployment-url>
vercel logs <deployment-url>
cd web && vercel --prod
```

## Railway

Project/service: `isoprism`

Production URL:

```text
https://api.isoprism.com
```

Source directory:

```text
api/
```

Useful commands:

```bash
railway link
railway logs --service isoprism
railway status
railway open
railway run env
```

The API parser uses tree-sitter grammar bindings, so Railway builds need CGO and a C compiler. Keep `CGO_ENABLED=1` and `api/nixpacks.toml` build tooling aligned.

## GitHub App

Cloud uses one production GitHub App for production web and local frontend development against the hosted API.

Webhook URL:

```text
https://api.isoprism.com/webhooks/github
```

Recommended Railway frontend values:

```text
FRONTEND_URL=https://isoprism.com
FRONTEND_URLS=https://isoprism.com,http://localhost:3000
```

The CLI does not require the GitHub App. Future local PR metadata should come from `gh` where available.

## Supabase

Project ref:

```text
sampxhpwbxvyphprnqtc
```

Migrations:

```text
supabase/migrations/
```

Useful commands:

```bash
supabase projects list
supabase db dump --linked
supabase db push --linked
supabase db pull --linked
supabase db diff --linked
```

The API startup checks `api/internal/db.RequiredMigrationVersion` against Supabase migration history. Apply migrations before deploying API code that requires them.

## Environment Variables

Cloud API:

| Variable | Used by |
|---|---|
| `DATABASE_URL` | Go API Postgres connection |
| `GITHUB_APP_ID` | GitHub App |
| `GITHUB_APP_PRIVATE_KEY` | GitHub App |
| `GITHUB_WEBHOOK_SECRET` | webhook validation |
| `FRONTEND_URL` | default redirect URL |
| `FRONTEND_URLS` | CORS and redirect allowlist |
| `GEMINI_API_KEY` | PR AI analysis |
| `GITHUB_FEEDBACK_TOKEN` | feedback issue creation |
| `GITHUB_FEEDBACK_REPO` | feedback issue target |
| `ADMIN_PASSWORD` | pilot admin routes |
| `MAILTRAP_API_KEY` | pilot invite/review emails |
| `PILOT_EMAIL_FROM` | pilot email sender |

Cloud web:

| Variable | Used by |
|---|---|
| `NEXT_PUBLIC_SUPABASE_URL` | Supabase client |
| `NEXT_PUBLIC_SUPABASE_ANON_KEY` | Supabase client |
| `NEXT_PUBLIC_API_URL` | API base URL |
| `NEXT_PUBLIC_VERCEL_GIT_COMMIT_SHA` | feedback context |

CLI:

| Variable/Flag | Used by |
|---|---|
| `--cache-dir` | local `.isoprism` cache override |
| `--host` | local daemon bind host |
| `--port` | local daemon bind port |
| `--web-dir` | frontend development bridge |
| `--web-port` | frontend development bridge port |
