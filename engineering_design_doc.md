# Aperture — Engineering Design Document

> Status: v0.5 | Author: Callum | Updated: 2026-03-26

---

## Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Data Model](#3-data-model)
4. [GitHub App Integration](#4-github-app-integration)
5. [Backend API](#5-backend-api)
6. [Frontend](#6-frontend)
7. [Auth & Security](#7-auth--security)
8. [Deployment](#8-deployment)
9. [Roadmap](#9-roadmap)
10. [Technical Decisions](#10-technical-decisions)
11. [Open Questions](#11-open-questions)
12. [Iteration Log](#12-iteration-log)

---

## 1. Overview

Aperture is a SaaS web application that provides a visual intelligence layer over GitHub pull requests. It ingests GitHub events via webhook, enriches them with AI-generated analysis, and presents an interface for engineers and team leads to understand flow, risk, and change meaning.

**Target scale:** a couple dozen engineering teams. Architecture is deliberately simple — no message queues, no search indices, no caching layer beyond what Postgres provides.

---

## 2. Architecture

### Tech Stack

| Layer | Technology | Hosting |
|---|---|---|
| Frontend | Next.js (App Router) + TypeScript + Tailwind CSS + shadcn/ui | Vercel |
| Backend API | Go (chi router) | Fly.io |
| Database | Supabase (Postgres 15) | Supabase |
| Auth | Supabase Auth (GitHub OAuth) | Supabase |
| AI | Anthropic Claude (primary) / OpenAI GPT (swap-in) | External APIs |
| GitHub Integration | GitHub App (webhooks + REST API v3) | — |

### System Diagram

```
                        ┌─────────────────────────────────────────┐
                        │               User's Browser             │
                        │        Next.js (Vercel)                  │
                        └──────────────┬──────────────────────────┘
                                       │ REST / fetch
                        ┌──────────────▼──────────────────────────┐
                        │           Go API Server (Fly.io)         │
                        │                                          │
                        │  ┌─────────────┐  ┌──────────────────┐  │
                        │  │  REST API   │  │  Webhook Handler  │  │
                        │  │  /api/v1/…  │  │  /webhooks/github │  │
                        │  └──────┬──────┘  └────────┬─────────┘  │
                        │         │                  │             │
                        │  ┌──────▼──────────────────▼──────────┐ │
                        │  │    Queue Ranker | PR Analyzer       │ │
                        │  │    AI Provider Layer                │ │
                        │  └──────────────────┬─────────────────┘ │
                        └──────────────────────┼──────────────────┘
                                               │ pgx
                        ┌──────────────────────▼──────────────────┐
                        │           Supabase (Postgres 15)         │
                        │   organizations | repositories |         │
                        │   pull_requests | pr_analyses            │
                        └──────────────────────▲──────────────────┘
                                               │ webhooks
                        ┌──────────────────────┴──────────────────┐
                        │              GitHub                      │
                        │   GitHub App webhooks + REST API v3      │
                        └─────────────────────────────────────────┘
```

### Monorepo Layout

```
/api          Go backend (chi router, handlers, GitHub client, AI)
/web          Next.js frontend (App Router)
/db
  /migrations Plain SQL migration files
```

---

## 3. Data Model

Multi-tenancy is scoped at the **organization** level. Every resource table carries `org_id`. RLS policies in Supabase enforce org isolation as a secondary layer; the Go backend always includes `org_id` in queries as the primary guard.

### Tables

```sql
-- Users (mirrors auth.users; populated by trigger on sign-in)
users
  id               uuid PK          -- matches auth.users.id
  email            text
  display_name     text
  avatar_url       text
  github_user_id   bigint UNIQUE
  github_username  text
  created_at       timestamptz

-- Organizations (top-level tenant; one per GitHub org or personal account)
organizations
  id                    uuid PK
  name                  text
  slug                  text UNIQUE          -- used in URLs: /orgs/{slug}
  github_account_login  text UNIQUE
  github_account_type   text                 -- 'Organization' | 'User'
  github_account_id     bigint
  avatar_url            text
  created_at            timestamptz

-- Org membership
org_members
  id          uuid PK
  org_id      uuid FK → organizations
  user_id     uuid FK → users
  role        text                           -- 'org_admin' | 'member'
  created_at  timestamptz
  UNIQUE (org_id, user_id)

-- Org join requests (for users whose GitHub org is already connected)
org_join_requests
  id           uuid PK
  org_id       uuid FK → organizations
  user_id      uuid FK → users
  status       text DEFAULT 'pending'        -- 'pending' | 'approved' | 'rejected'
  created_at   timestamptz
  resolved_at  timestamptz
  resolved_by  uuid FK → users
  UNIQUE (org_id, user_id)

-- GitHub teams (synced from GitHub on install, for org-type accounts)
teams
  id              uuid PK
  org_id          uuid FK → organizations
  name            text
  slug            text
  github_team_id  bigint
  created_at      timestamptz
  UNIQUE (org_id, slug)

-- GitHub team membership
team_members
  id          uuid PK
  team_id     uuid FK → teams
  user_id     uuid FK → users
  role        text                           -- 'team_admin' | 'member'
  created_at  timestamptz
  UNIQUE (team_id, user_id)

-- GitHub App installations (one per org)
github_installations
  id                  uuid PK
  org_id              uuid FK → organizations
  installation_id     bigint UNIQUE          -- GitHub's numeric installation ID
  account_login       text
  account_type        text                   -- 'Organization' | 'User'
  account_avatar_url  text
  created_at          timestamptz

-- Repositories
repositories
  id               uuid PK
  org_id           uuid FK → organizations
  installation_id  uuid FK → github_installations
  github_repo_id   bigint
  full_name        text                      -- e.g. "acme/backend"
  default_branch   text DEFAULT 'main'
  is_active        boolean DEFAULT true      -- user-controlled; inactive repos are hidden
  created_at       timestamptz
  UNIQUE (org_id, github_repo_id)

-- Team → repo mapping (which repos a GitHub team watches)
team_repos
  team_id  uuid FK → teams
  repo_id  uuid FK → repositories
  PRIMARY KEY (team_id, repo_id)

-- Pull requests (upserted on webhook events and manual syncs)
pull_requests
  id                    uuid PK
  org_id                uuid FK → organizations
  repo_id               uuid FK → repositories
  github_pr_id          bigint
  number                int
  title                 text
  body                  text
  author_github_login   text
  author_avatar_url     text
  base_branch           text
  head_branch           text
  state                 text                 -- 'open' | 'closed' | 'merged'
  draft                 boolean DEFAULT false
  additions             int
  deletions             int
  changed_files         int
  html_url              text
  opened_at             timestamptz
  closed_at             timestamptz
  merged_at             timestamptz
  last_activity_at      timestamptz
  last_synced_at        timestamptz
  created_at            timestamptz
  updated_at            timestamptz
  UNIQUE (repo_id, github_pr_id)

-- AI-generated analysis per PR (one row per HEAD commit analysed)
pr_analyses
  id                uuid PK
  pull_request_id   uuid FK → pull_requests
  commit_sha        text
  summary           text
  why               text
  impacted_areas    text[]
  key_files         text[]
  size_label        text                     -- 'small' | 'medium' | 'large'
  risk_score        int                      -- 1–10
  risk_label        text                     -- 'low' | 'medium' | 'high'
  risk_reasons      text[]
  semantic_groups   jsonb                    -- [{label, files[], description}]
  ai_provider       text
  ai_model          text
  generated_at      timestamptz
  created_at        timestamptz

-- PR reviews (synced from GitHub)
pr_reviews
  id                   uuid PK
  pull_request_id      uuid FK → pull_requests
  github_review_id     bigint UNIQUE
  reviewer_login       text
  reviewer_avatar_url  text
  state                text                  -- 'approved' | 'changes_requested' | 'commented' | 'dismissed'
  submitted_at         timestamptz

-- Org-level preferences
org_preferences
  id                  uuid PK
  org_id              uuid FK → organizations UNIQUE
  pr_size_small_max   int DEFAULT 100        -- lines changed threshold
  pr_size_medium_max  int DEFAULT 400
  stale_after_hours   int DEFAULT 48
  risk_sensitivity    text DEFAULT 'medium'  -- 'low' | 'medium' | 'high'
  ai_provider         text DEFAULT 'anthropic'
  created_at          timestamptz
  updated_at          timestamptz
```

### RLS Summary

Two helper functions gate all policies:
- `is_org_member(org_id)` — true if `auth.uid()` is in `org_members` for that org
- `is_org_admin(org_id)` — true if role is `org_admin`

All reads are gated by `is_org_member`; mutations (toggle repo, update preferences, resolve join requests) are gated by `is_org_admin`. The Go backend bypasses RLS using the service role key, which is never exposed to the client.

---

## 4. GitHub App Integration

### Why GitHub App

- Installable at org or personal-account level without a personal token
- Webhook delivery tied to the App, not to a user
- Fine-grained permissions; installation tokens are short-lived and auto-refreshed

### Permissions Required

| Permission | Access | Reason |
|---|---|---|
| Pull requests | Read & Write | Read diffs, post comments, submit reviews |
| Contents | Read | File tree and diffs |
| Metadata | Read | Repo info |
| Members | Read | Reviewer identity |

### Webhook Events

| Event | Actions handled | Status |
|---|---|---|
| `pull_request` | opened, synchronize, reopened, ready_for_review, closed | ✅ |
| `installation` | deleted | ✅ |
| `installation_repositories` | added, removed | ✅ |
| `pull_request_review` | submitted | Planned |
| `pull_request_review_comment` | created, edited, deleted | Planned |
| `issue_comment` | created | Planned |

All webhooks are verified with `X-Hub-Signature-256` HMAC before processing.

### Installation Flow

The callback URL is `GET /api/v1/github/callback`. GitHub passes `installation_id`, `setup_action`, and `state` (which carries the user's Supabase UUID, set when constructing the install URL).

| `setup_action` | Behaviour |
|---|---|
| `install` | Creates org + installation record, syncs repos + open PRs, redirects to `/onboarding/repos` |
| `update` | Looks up existing org by `installation_id`, re-syncs repos, redirects to queue |
| `request` | User lacks permission to install; redirects to `/request-sent` with an explanatory message |

When repos are added or removed via GitHub's own settings UI, the `installation_repositories` webhook fires. Added repos are upserted with `is_active = true` and their open PRs are backfilled asynchronously. Removed repos are marked `is_active = false`.

### Token Management

- **App JWT**: RS256-signed, 9-min expiry — used to fetch installation tokens
- **Installation tokens**: 1-hour expiry, fetched from `POST /app/installations/{id}/access_tokens`, cached in memory per installation ID and refreshed on demand

---

## 5. Backend API

### File Structure (actual)

```
api/
  cmd/api/main.go                  entry point
  internal/
    api/
      router.go                    chi router + CORS + auth middleware
      handlers/
        github.go                  installation callback, webhooks, repo sync
        orgs.go                    orgs, repos, PRs, join requests, auth status
        queue.go                   queue endpoint + urgency scoring
        teams.go                   teams list
    github/
      app.go                       JWT generation, installation token cache
      client.go                    GitHub REST API wrapper
      webhook.go                   signature verification, payload types
    models/
      types.go                     shared Go structs
    config/
      config.go                    env-based config loader
```

### API Routes

**Public (no auth)**
```
POST /webhooks/github
GET  /api/v1/github/callback
GET  /api/v1/auth/status
```

**Authenticated (Supabase JWT required)**
```
GET    /api/v1/me/orgs
DELETE /api/v1/me

GET    /api/v1/orgs/{orgSlug}
GET    /api/v1/orgs/{orgSlug}/queue
GET    /api/v1/orgs/{orgSlug}/repos
PATCH  /api/v1/orgs/{orgSlug}/repos/{repoID}
DELETE /api/v1/orgs/{orgSlug}/repos/{repoID}
POST   /api/v1/orgs/{orgSlug}/repos/{repoID}/sync
GET    /api/v1/orgs/{orgSlug}/prs/{prID}
GET    /api/v1/orgs/{orgSlug}/teams
POST   /api/v1/orgs/{orgSlug}/join-requests
GET    /api/v1/orgs/{orgSlug}/join-requests
PATCH  /api/v1/orgs/{orgSlug}/join-requests/{requestID}
```

Auth middleware parses the Supabase JWT without verifying the signature (tokens come from Supabase directly) and injects `X-User-ID` into the request for handlers to use.

### Queue Scoring

Each open, non-draft PR is scored for urgency at query time:

```
urgency = (wait_time_score × 0.4) + (risk_score × 0.35) + (impact_score × 0.25)
```

| Component | Source | Default (no analysis yet) |
|---|---|---|
| `wait_time_score` | Hours since `last_activity_at`, normalised, cap 48h | — |
| `risk_score` | `pr_analyses.risk_score` (1–10 → 0–1) | 0.3 |
| `impact_score` | `len(pr_analyses.impacted_areas)`, normalised, cap 5 | 0.2 |

The queue is sorted descending by urgency score. Scores are recomputed on every request; there is no background tick yet.

### AI Provider Interface (planned)

```go
type Provider interface {
    AnalysePR(ctx context.Context, req AnalysisRequest) (*AnalysisResult, error)
}
```

Both `AnthropicProvider` and `OpenAIProvider` will implement this. The active provider is selected from `org_preferences.ai_provider`. Switching requires no code change.

---

## 6. Frontend

### Route Structure

```
/login                           GitHub OAuth sign-in via Supabase
/auth/callback                   Supabase auth callback → redirects to org or onboarding
/onboarding                      GitHub App install (first-time users)
/onboarding/repos                Repo selection after install
/onboarding/join                 Request to join an existing org
/request-sent                    Shown after setup_action=request; auto-redirects after 5s
/                                Root: redirects to first org or /onboarding
/orgs/[orgSlug]                  Queue (Activity View)
/orgs/[orgSlug]/pr/[prId]        PR detail page
/orgs/[orgSlug]/settings         Settings: org switcher, repos, add org, delete account
```

### Data Fetching Strategy

- **Server Components** (`force-dynamic`) for the queue and PR detail — data fetched server-side on every request using the Supabase session token
- **Client Components** for settings (interactive: repo toggles, delete flows) and onboarding
- Queue has a manual **Refresh** button (`router.refresh()`) to re-run the server fetch
- No Supabase Realtime yet — planned for Phase 3

### Key Components

| Component | Location | Notes |
|---|---|---|
| `AppHeader` | `components/layout/app-header.tsx` | Nav + org switcher |
| `QueueList` | `components/queue/queue-list.tsx` | Renders ranked PR rows |
| `QueueItemRow` | `components/queue/queue-item-row.tsx` | Single PR row with badges |
| `RefreshButton` | `components/queue/refresh-button.tsx` | Client component, calls `router.refresh()` |

### Design System

- **Tailwind CSS** for utility styling; **shadcn/ui** for accessible base components
- Neutral palette: high whitespace, muted backgrounds, sharp typographic hierarchy
- No animation library — CSS transitions via Tailwind

---

## 7. Auth & Security

### Sign-in Flow

1. User hits `/login` → Supabase Auth GitHub OAuth
2. GitHub redirects to `/auth/callback` → Supabase exchanges code for session
3. Callback calls `GET /api/v1/auth/status?github_token=…&user_id=…`
4. API checks whether the user's GitHub account (or any of their org memberships) matches a connected Aperture org:
   - **Match + member**: redirect to `/orgs/{slug}`
   - **Match + pre-seeded** (added via team sync before sign-up): link records, redirect to queue
   - **Match + not member**: redirect to `/onboarding/join?org={slug}`
   - **No match**: redirect to `/onboarding`

### Org Isolation

- Go backend always scopes queries with `WHERE org_id = $1`
- Supabase RLS is a defence-in-depth layer (see [Data Model](#3-data-model))
- Frontend never writes directly to the DB — all mutations go through the Go API
- Service role key (bypasses RLS) is only used server-side in Go; never sent to the client

---

## 8. Deployment

### Infrastructure

| Service | Platform | Config |
|---|---|---|
| Next.js frontend | Vercel | Auto-deploys from `main`; root set to `/web` |
| Go API | Fly.io | `fly.toml` in `/api`; single shared-cpu-1x VM to start |
| Postgres + Auth | Supabase | Managed; free tier sufficient at current scale |
| GitHub App | GitHub | Webhook URL → Fly.io; single registered app |

### Branches & Deploy Model

- `main` — production; commit directly (solo project, no PRs)
- `preview` — fast-forward mirror of `main`; push both when syncing

### Environment Variables

**Go API (Fly.io secrets)**
```
SUPABASE_URL
GITHUB_APP_CLIENT_ID
GITHUB_APP_PRIVATE_KEY      PEM (literal \n sequences are normalised on load)
GITHUB_WEBHOOK_SECRET
FRONTEND_URL                e.g. https://app.aperture64.dev
ANTHROPIC_API_KEY           (planned)
OPENAI_API_KEY              (planned)
```

**Next.js (Vercel)**
```
NEXT_PUBLIC_SUPABASE_URL
NEXT_PUBLIC_SUPABASE_ANON_KEY
NEXT_PUBLIC_API_URL         Fly.io Go API base URL
NEXT_PUBLIC_GITHUB_APP_NAME Used to construct GitHub install links
```

### Migrations

Plain SQL files in `/db/migrations/`, applied manually via Supabase dashboard or CLI.

---

## 9. Roadmap

### Phase 1 — Foundation ✅ Complete

- [x] Monorepo scaffold (`/web`, `/api`, `/db`)
- [x] DB schema + RLS policies
- [x] Go API: chi router, Supabase JWT middleware
- [x] GitHub App client: JWT, installation token cache
- [x] Webhook handler: `pull_request`, `installation`, `installation_repositories`
- [x] Installation callback: install / update / request flows
- [x] Repo sync: open PRs backfilled on install and on `installation_repositories` add
- [x] Queue (Activity View): urgency-scored, manual refresh
- [x] PR detail page
- [x] Settings: org switcher, repo toggle/delete, add org/repos, delete account
- [x] Org join request flow

### Phase 2 — AI Pipeline

- [ ] AI provider interface + Anthropic implementation
- [ ] PR analysis pipeline: fetch diff → call AI → store `pr_analyses`
- [ ] OpenAI implementation (same interface, swap via preference)
- [ ] Queue: use real `risk_score` and `impacted_areas` instead of defaults

### Phase 3 — PR Intelligence Panel

- [ ] PR detail: AI summary, why, impacted areas, key files
- [ ] Semantic diff view: grouped → file list → line diff
- [ ] Synced GitHub reviews and comments
- [ ] Actions: approve / comment / request changes via Go → GitHub API
- [ ] Supabase Realtime: live queue updates on webhook events

### Phase 4 — Insights

- [ ] Weekly org insights computation (median review time, PR size distribution, top reviewers)
- [ ] Insights View UI

### Phase 5 — Polish & Integrations

- [ ] Linear integration (read-only, PR enrichment context)
- [ ] Org preferences UI (size thresholds, stale hours, AI provider)
- [ ] Error states, empty states, performance pass

---

## 10. Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Go on Fly.io, not Vercel | Fly.io | Vercel doesn't support persistent Go servers |
| In-process concurrency (goroutines) | Yes | No Redis/queue needed at this scale; buffered channels are sufficient |
| Scoring at query time | Yes (for now) | Avoids background workers; acceptable latency at small scale |
| Diff truncation for AI | First ~8k tokens | Keeps AI costs controlled for large PRs |
| No search index | Postgres FTS | Sufficient for dozens of orgs |
| Monorepo | Yes | Easier for a solo developer to manage and deploy atomically |
| Supabase JWT parsed but not verified | Yes | Tokens originate from Supabase; full JWKS verification is the next hardening step |

---

## 11. Open Questions

- **Billing**: No billing in MVP. Stripe + Supabase when monetising.
- **Realtime**: Currently polling via manual refresh. Supabase Realtime subscriptions planned for Phase 3.
- **Diff storage**: Fetched live from GitHub. Cache in Supabase Storage if rate limits become an issue.
- **JWT verification**: Currently parsed without signature check. Should add JWKS verification before opening to untrusted users.
- **AI cost management**: Negligible at current scale. Add per-org usage tracking before broader rollout.
- **Jira**: Deferred — requires public callback URL and app review. Add when there is clear demand.

---

## 12. Iteration Log

| Version | Date | Changes |
|---|---|---|
| v0.1 | 2026-03-22 | Initial design document |
| v0.2 | 2026-03-22 | Auth, onboarding, repo connection, queue (Activity View) |
| v0.3 | 2026-03-24 | PR detail page; settings page with org switcher, repo table, delete account |
| v0.4 | 2026-03-25 | GitHub App reinstall flow fix; onboarding redirect fix; org_members insert fix |
| v0.5 | 2026-03-26 | Queue refresh button; `setup_action=request` handling; `installation_repositories` webhook (add/remove repos + open PR backfill) |
