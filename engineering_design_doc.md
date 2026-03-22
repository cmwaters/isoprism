# Aperture — Engineering Design Document

> Status: Draft v0.1 | Author: Callum | Date: 2026-03-22

---

## 1. Overview

Aperture is a SaaS web application that provides a visual intelligence layer over GitHub pull requests. It ingests GitHub events via webhook, enriches them with AI-generated analysis, and presents a real-time interface for engineers and team leads to understand flow, risk, and change meaning.

This document covers the technical architecture, data model, API design, infrastructure, and MVP delivery plan.

---

## 2. Tech Stack

| Layer | Technology | Hosting |
|---|---|---|
| Frontend | Next.js 14+ (App Router) + TypeScript + Tailwind CSS + shadcn/ui | Vercel |
| Backend API | Go (chi router) | Fly.io |
| Database | Supabase (Postgres 15) | Supabase (managed) |
| Auth | Supabase Auth (GitHub OAuth) | Supabase |
| Realtime | Supabase Realtime (Postgres changes) | Supabase |
| AI | Abstracted provider — Anthropic Claude + OpenAI GPT | External APIs |
| GitHub Integration | GitHub App (webhooks + API) | — |
| File Storage | Supabase Storage (if needed for attachments) | Supabase |

---

## 3. High-Level Architecture

```
                        ┌─────────────────────────────────────────┐
                        │               User's Browser             │
                        │        Next.js (Vercel) + Supabase       │
                        │        Realtime (WebSocket)              │
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
                        │  │         Core Services               │ │
                        │  │  PR Analyzer | Risk Scorer |        │ │
                        │  │  Queue Ranker | Insights Builder    │ │
                        │  └──────────────────┬─────────────────┘ │
                        │                     │                    │
                        │  ┌──────────────────▼─────────────────┐ │
                        │  │         AI Provider Layer           │ │
                        │  │   Anthropic Claude | OpenAI GPT     │ │
                        │  └────────────────────────────────────┘ │
                        └──────────────┬──────────────────────────┘
                                       │ Postgres client
                        ┌──────────────▼──────────────────────────┐
                        │           Supabase (Postgres)            │
                        │   teams | repos | pull_requests |        │
                        │   pr_analyses | comments | insights      │
                        └─────────────────────────────────────────┘
                                       ▲
                        ┌──────────────┴──────────────────────────┐
                        │              GitHub                      │
                        │   GitHub App webhooks + REST API v3      │
                        └─────────────────────────────────────────┘
```

---

## 4. GitHub App Integration

### Why GitHub App over OAuth App

- Installable at the org or repo level without a personal token
- Webhook delivery is tied to the App, not to a user
- Fine-grained permissions (read PRs, read code, post comments, post reviews)
- Installation tokens are short-lived and auto-refreshed — more secure

### Permissions Required

| Permission | Access | Reason |
|---|---|---|
| Pull requests | Read & Write | Read diffs, post comments, submit reviews |
| Contents | Read | Read file tree and diffs |
| Metadata | Read | Repo info |
| Issues | Read | Linked issue context |
| Members | Read | Reviewer identity |

### Webhook Events Consumed

| Event | Trigger |
|---|---|
| `pull_request` | opened, synchronize, closed, reopened, ready_for_review |
| `pull_request_review` | submitted |
| `pull_request_review_comment` | created, edited, deleted |
| `issue_comment` | created (on PRs) |

### Installation Flow

1. User clicks "Connect GitHub" in Settings
2. Redirected to GitHub App installation page
3. On callback, Aperture receives `installation_id`
4. Go backend exchanges `installation_id` for an installation access token
5. Team is linked to installation; selected repos are fetched and stored

### Token Management

- GitHub App authenticates with a JWT signed with the App's private key (RS256, 10-min expiry)
- Per-installation tokens are fetched from `POST /app/installations/{id}/access_tokens` (1-hour expiry)
- Go backend caches tokens in memory with expiry; refreshes on demand

---

## 5. Data Model

All tables include `created_at` and `updated_at` timestamps. Multi-tenancy is enforced by `team_id` on every resource. Row Level Security (RLS) is enabled on all tables via Supabase.

```sql
-- Teams (the top-level tenant)
teams
  id            uuid PK
  name          text
  slug          text UNIQUE
  created_at    timestamptz

-- Users (managed by Supabase Auth, extended here)
users
  id            uuid PK (matches auth.users.id)
  email         text
  display_name  text
  avatar_url    text
  created_at    timestamptz

-- Team membership
team_members
  id            uuid PK
  team_id       uuid FK → teams
  user_id       uuid FK → users
  role          text  -- 'owner' | 'member'
  created_at    timestamptz

-- GitHub App installations
github_installations
  id                  uuid PK
  team_id             uuid FK → teams
  installation_id     bigint UNIQUE   -- GitHub's installation ID
  account_login       text            -- org or user login
  account_type        text            -- 'Organization' | 'User'
  created_at          timestamptz

-- Repositories
repositories
  id                  uuid PK
  team_id             uuid FK → teams
  installation_id     uuid FK → github_installations
  github_repo_id      bigint UNIQUE
  full_name           text            -- e.g. "acme/backend"
  default_branch      text
  is_active           boolean DEFAULT true
  created_at          timestamptz

-- Pull Requests
pull_requests
  id                  uuid PK
  team_id             uuid FK → teams
  repo_id             uuid FK → repositories
  github_pr_id        bigint
  number              int
  title               text
  body                text
  author_github_login text
  base_branch         text
  head_branch         text
  state               text    -- 'open' | 'closed' | 'merged'
  draft               boolean
  additions           int
  deletions           int
  changed_files       int
  opened_at           timestamptz
  closed_at           timestamptz
  merged_at           timestamptz
  last_synced_at      timestamptz
  created_at          timestamptz
  updated_at          timestamptz

-- AI-generated analysis for a PR (versioned per push)
pr_analyses
  id                  uuid PK
  pull_request_id     uuid FK → pull_requests
  commit_sha          text            -- the HEAD sha this analysis covers
  summary             text            -- plain-language summary
  why                 text            -- inferred intent / linked issue context
  impacted_areas      text[]          -- e.g. ["auth-service", "billing"]
  key_files           text[]
  size_label          text            -- 'small' | 'medium' | 'large'
  risk_score          int             -- 1–10
  risk_label          text            -- 'low' | 'medium' | 'high'
  risk_reasons        text[]          -- bullet points explaining risk
  semantic_groups     jsonb           -- [{label, files[], description}]
  ai_provider         text            -- 'anthropic' | 'openai'
  ai_model            text            -- e.g. 'claude-opus-4-5', 'gpt-4o'
  generated_at        timestamptz
  created_at          timestamptz

-- PR review state (synced from GitHub)
pr_reviews
  id                  uuid PK
  pull_request_id     uuid FK → pull_requests
  github_review_id    bigint
  reviewer_login      text
  state               text    -- 'approved' | 'changes_requested' | 'commented'
  submitted_at        timestamptz

-- Comments (synced from GitHub)
pr_comments
  id                  uuid PK
  pull_request_id     uuid FK → pull_requests
  github_comment_id   bigint UNIQUE
  author_login        text
  body                text
  in_reply_to_id      bigint
  path                text            -- file path if inline
  line               int             -- line number if inline
  posted_at           timestamptz
  updated_at          timestamptz

-- Pre-computed team insights (weekly snapshots)
team_insights
  id                  uuid PK
  team_id             uuid FK → teams
  period_start        date
  period_end          date
  median_review_time_hours  numeric
  p95_review_time_hours     numeric
  avg_pr_size               numeric
  large_pr_count            int
  total_prs_opened          int
  total_prs_merged          int
  top_reviewers             jsonb   -- [{login, review_count}]
  computed_at               timestamptz

-- Team preferences
team_preferences
  id                        uuid PK
  team_id                   uuid FK → teams UNIQUE
  pr_size_small_max         int DEFAULT 100     -- lines changed
  pr_size_medium_max        int DEFAULT 400
  stale_after_hours         int DEFAULT 48
  risk_sensitivity          text DEFAULT 'medium'
  ai_provider               text DEFAULT 'anthropic'
  created_at                timestamptz
  updated_at                timestamptz
```

### Multi-Tenancy & Security

- Every query from the Go backend includes `team_id` — no cross-tenant data leakage
- Supabase RLS policies are a secondary enforcement layer
- The frontend uses Supabase Auth session tokens; the Go API validates these via Supabase JWT verification

---

## 6. Backend — Go API Server

### Structure

```
/cmd
  /api
    main.go           -- entry point, wires up server
/internal
  /api
    router.go         -- chi router, middleware
    /handlers         -- one file per resource group
      prs.go
      teams.go
      repos.go
      webhooks.go
      insights.go
  /db
    client.go         -- Supabase Postgres connection (pgx)
    /queries          -- raw SQL or sqlc-generated
  /github
    app.go            -- JWT + installation token management
    client.go         -- GitHub API wrapper
    webhook.go        -- webhook signature verification + dispatch
  /ai
    provider.go       -- Provider interface
    anthropic.go      -- Anthropic implementation
    openai.go         -- OpenAI implementation
    prompts.go        -- prompt templates
  /analyzer
    pr_analyzer.go    -- orchestrates AI analysis for a PR
    risk_scorer.go    -- post-processes AI output into risk score
    queue_ranker.go   -- computes urgency score for Activity View
  /insights
    builder.go        -- computes weekly team_insights rows
  /models
    types.go          -- shared Go structs
/config
  config.go           -- env-based config
```

### Key Middleware

- **Auth**: Validates Supabase JWT on all `/api/v1/*` routes; extracts `user_id` and `team_id`
- **Webhook verification**: `X-Hub-Signature-256` HMAC check before any GitHub event is processed
- **Rate limiting**: Simple per-IP limiter on webhook endpoint to guard against replay

### Webhook → Analysis Pipeline

```
GitHub Event
    │
    ▼
POST /webhooks/github
    │  verify signature
    │  parse event type
    ▼
pull_request (opened | synchronize)
    │
    ▼
Upsert pull_request row in DB
    │
    ▼
Enqueue analysis job
    │  (goroutine + buffered channel, simple at this scale)
    ▼
Fetch diff from GitHub API
    │
    ▼
Fetch linked issue/PR context (Linear/Jira if connected)
    │
    ▼
Call AI Provider (summary + risk + semantic grouping)
    │
    ▼
Persist pr_analyses row
    │
    ▼
Supabase Realtime broadcasts change → frontend updates live
```

### AI Provider Interface

```go
type Provider interface {
    AnalysePR(ctx context.Context, req AnalysisRequest) (*AnalysisResult, error)
}

type AnalysisRequest struct {
    Title       string
    Body        string
    Diff        string   // truncated to token budget
    IssueContext string  // from linked Linear/Jira/GitHub issue
}

type AnalysisResult struct {
    Summary       string
    Why           string
    ImpactedAreas []string
    KeyFiles      []string
    SizeLabel     string
    RiskScore     int
    RiskLabel     string
    RiskReasons   []string
    SemanticGroups []SemanticGroup
}
```

Both `AnthropicProvider` and `OpenAIProvider` implement this interface. The active provider is selected from `team_preferences.ai_provider` (or a global env default). Switching providers requires no code change — only a config/preference update.

### Queue Ranker — Activity View Scoring

Each open PR is scored for urgency using a weighted formula:

```
urgency = (wait_time_score × 0.4) + (risk_score × 0.35) + (system_impact_score × 0.25)
```

- **wait_time_score**: normalised hours since last action (caps at `stale_after_hours`)
- **risk_score**: from `pr_analyses.risk_score` (1–10 normalised to 0–1)
- **system_impact_score**: number of impacted areas, normalised

This is recomputed on each webhook event and on a 15-minute background tick.

---

## 7. Frontend — Next.js

### Route Structure

```
/app
  /(auth)
    /login              -- GitHub OAuth sign-in
    /callback           -- Supabase auth callback
  /(app)
    /[team]
      /page.tsx         -- Activity View (default)
      /pr/[id]
        /page.tsx       -- PR Intelligence Panel
      /insights
        /page.tsx       -- Insights View
      /settings
        /page.tsx       -- Settings & Integrations
        /repos
        /integrations
        /preferences
        /members
  /onboarding           -- first-time setup flow
```

### State & Data Fetching

- **Server Components** for initial data loads (SSR via Supabase server client)
- **Client Components** only where interactivity or realtime is needed
- **Supabase Realtime** subscription in the Activity View client component — listens for changes to `pull_requests` and `pr_analyses` for the team's repos, updates the queue without a page refresh
- No Redux or heavy state library — React state + Supabase client is sufficient at this scale

### Key UI Components

| Component | Description |
|---|---|
| `PRQueueItem` | Single row in Activity View — title, age, state badge, size/risk chips, impacted areas |
| `PRIntelligencePanel` | Full PR view — tabbed: Context / Changes / Discussion / Actions |
| `SemanticDiffView` | Grouped diff viewer — semantic groups → file list → line diff (progressive drill-down) |
| `InsightCard` | Single insight tile for Insights View |
| `RiskBadge` | Colour-coded risk indicator (low/medium/high) |
| `ProviderSelector` | Dropdown in settings to toggle Anthropic ↔ OpenAI |

### Design System

- **Tailwind CSS** for utility styling
- **shadcn/ui** as the component base (accessible, unstyled-first, composable)
- Custom design tokens for Aperture's Typeform-inspired palette: high whitespace, muted backgrounds, sharp typographic hierarchy
- Smooth page transitions via `next/navigation` + CSS animations — no heavy animation library needed

---

## 8. Authentication & Multi-Tenancy

### Auth Flow

1. User hits `/login` → Supabase Auth GitHub OAuth
2. On first sign-in, a `users` row is created (via Supabase Auth hook or trigger)
3. User either creates a team or is invited to one
4. All subsequent API calls carry the Supabase JWT; the Go backend extracts `user_id` and resolves `team_id` via `team_members`

### Team Isolation

- Every DB query in the Go backend is scoped with `WHERE team_id = $1`
- Supabase RLS provides defence-in-depth: a policy on each table ensures a user can only read/write rows belonging to their team
- The frontend never has direct write access to sensitive tables — all mutations go through the Go API

---

## 9. Integrations

### GitHub (MVP — required)
Covered in Section 4.

### Linear (MVP — read-only)
- OAuth2 flow to obtain Linear access token per team
- When enriching a PR, the Go backend searches Linear for issues matching the PR branch name or body links
- Fetches issue title, description, and status to include in `AnalysisRequest.IssueContext`

### Jira (post-MVP)
- Same pattern as Linear — OAuth2, search by branch/link
- Deferred because Linear covers the target early-adopter profile

### Notion (post-MVP)
- Read-only: link Notion docs for additional context in PR analysis

---

## 10. Deployment

### Infrastructure

| Service | Platform | Notes |
|---|---|---|
| Next.js frontend | Vercel | Auto-deploy from `main`; preview deploys on PRs |
| Go API server | Fly.io | Single `fly.toml`; 1 VM (shared-cpu-1x, 256MB) to start; scale up as needed |
| Postgres + Auth + Realtime | Supabase | Free tier sufficient for couple dozen teams |
| GitHub App | GitHub | Register once; point webhook URL to Fly.io |

### Environment Variables

**Go API (Fly.io secrets)**
```
SUPABASE_URL
SUPABASE_SERVICE_ROLE_KEY    -- server-side only, never exposed to client
GITHUB_APP_ID
GITHUB_APP_PRIVATE_KEY       -- PEM, base64-encoded
GITHUB_WEBHOOK_SECRET
ANTHROPIC_API_KEY
OPENAI_API_KEY
LINEAR_CLIENT_ID
LINEAR_CLIENT_SECRET
```

**Next.js (Vercel env vars)**
```
NEXT_PUBLIC_SUPABASE_URL
NEXT_PUBLIC_SUPABASE_ANON_KEY
NEXT_PUBLIC_API_URL           -- Fly.io Go API base URL
GITHUB_APP_NAME               -- for constructing install links
```

### CI/CD

- Single GitHub repo, monorepo: `/web` (Next.js) + `/api` (Go)
- Vercel auto-detects `/web` for frontend deploys
- GitHub Action on push to `main`: `go build ./...` + `go test ./...` → Fly.io deploy via `flyctl deploy`
- Database migrations managed with plain SQL files in `/db/migrations`, applied manually or via a small migration runner in Go on startup

---

## 11. MVP Delivery Phases

Given a solo build targeting a couple dozen teams, the recommended sequence ships a usable product early and avoids over-engineering.

### Phase 1 — Foundation (Weeks 1–3)
- [ ] Scaffold monorepo (`/web`, `/api`, `/db`)
- [ ] Supabase project setup: schema, RLS policies, Auth config
- [ ] Go API server skeleton: chi router, auth middleware, Supabase client
- [ ] Next.js skeleton: auth flow, team creation, basic layout shell
- [ ] GitHub App registration and installation flow
- [ ] Webhook handler: receive and verify GitHub events, upsert `pull_requests`

### Phase 2 — AI Pipeline (Weeks 4–5)
- [ ] AI provider interface + Anthropic implementation
- [ ] PR analysis pipeline: fetch diff → call AI → store `pr_analyses`
- [ ] OpenAI implementation (swap-in, same interface)
- [ ] Queue ranker: urgency scoring for open PRs

### Phase 3 — Activity View (Week 6)
- [ ] Activity View UI: ranked PR queue with state, size, risk, impacted areas
- [ ] Supabase Realtime subscription: live queue updates on webhook events
- [ ] PR state badges (needs review / needs author / stalled / draft)

### Phase 4 — PR Intelligence Panel (Weeks 7–8)
- [ ] Context tab: AI summary, why, impacted areas, linked issue context
- [ ] Changes tab: semantic group view → file list → line diff
- [ ] Discussion tab: synced comments from GitHub
- [ ] Actions tab: approve / comment / request changes (proxied through Go → GitHub API)

### Phase 5 — Insights View (Week 9)
- [ ] Weekly `team_insights` computation job
- [ ] Insights View UI: insight cards with trend deltas

### Phase 6 — Settings & Polish (Week 10)
- [ ] Settings UI: team, repos, preferences, integrations
- [ ] Onboarding flow for new teams
- [ ] Linear integration (read-only context enrichment)
- [ ] Performance pass, error states, empty states

---

## 12. Key Technical Decisions & Trade-offs

| Decision | Choice | Rationale |
|---|---|---|
| Go backend separate from Vercel | Fly.io | Vercel doesn't support persistent Go servers; Fly.io is simple and cheap for a solo project |
| In-process job queue (goroutines) | Yes, for MVP | No need for Redis/BullMQ at this scale; a buffered channel with worker goroutines is sufficient and eliminates infra complexity |
| Insights computed on a schedule | Weekly batch job | Real-time insight computation is expensive; weekly snapshots are sufficient for trend cards |
| Diff truncation for AI | Yes (first 8k tokens of diff) | Large PRs can have massive diffs; truncation with a note in the prompt keeps costs controlled |
| No separate search index | Correct for MVP | Postgres full-text search is sufficient for PR/comment search at dozens-of-teams scale |
| Monorepo | Yes | Easier for a solo developer to manage, share types, and deploy atomically |

---

## 13. Open Questions

- **Billing**: No billing in MVP. When monetising, Stripe + Supabase is the natural path.
- **PR diff storage**: Diffs are fetched live from GitHub on demand. If GitHub rate limits become an issue, diffs can be cached in Supabase Storage.
- **Self-serve onboarding vs invite-only**: Start invite-only for the first dozen teams to control quality. Open sign-up later.
- **Jira OAuth**: Jira's OAuth 2.0 (3LO) requires a public callback URL and app review for production — defer until there is clear demand.
- **AI cost management**: At couple-dozen teams, AI costs will be minimal. Add per-team usage tracking before scaling to manage costs proactively.
