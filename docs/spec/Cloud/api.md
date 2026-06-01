# Cloud API

## Boundary

The cloud API is the boundary between the hosted backend server and frontend browser. It owns auth, GitHub App integration, repository indexing, PR processing, feedback, admin, and AI-backed PR analysis.

Production base URL:

```text
https://api.isoprism.com
```

Local frontend development should use the production API by setting:

```bash
NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev
```

## Auth

Authenticated routes require:

```http
Authorization: Bearer <Supabase JWT>
```

The API parses the Supabase JWT `sub` claim and sets `X-User-ID` internally.

Admin pilot routes require:

```http
X-Admin-Password: <password>
```

Debug endpoints are currently unauthenticated development endpoints.

## Public Infrastructure

```text
GET  /health
POST /webhooks/github
GET  /api/v1/github/callback
GET  /api/v1/auth/status?user_id={uuid}
```

## Debug

```text
POST /debug/repos/{repoID}/reindex
POST /debug/prs/{prID}/reprocess
POST /debug/prs/{prID}/reprocess/graph
POST /debug/prs/{prID}/reprocess/ai
POST /debug/prs/{prID}/sync
```

Use `/reprocess/graph` when summaries should remain stable and only graph rows need rebuilding. Use `/reprocess/ai` when graph rows are already present and only summaries/risk should refresh. Use full `/reprocess` when both should refresh.

## Pilot And Admin

```text
GET    /api/v1/admin/beta/testers
POST   /api/v1/admin/beta/testers
DELETE /api/v1/admin/beta/testers/{testerID}

GET    /api/v1/admin/pilot/users
POST   /api/v1/admin/pilot/users
DELETE /api/v1/admin/pilot/users/{testerID}
GET    /api/v1/admin/pilot/forms
POST   /api/v1/admin/pilot/users/{testerID}/invite
POST   /api/v1/admin/pilot/users/{testerID}/review-email

POST   /api/v1/pilot/register
POST   /api/v1/pilot/invites/{token}/accept
GET    /api/v1/pilot/review/{token}
POST   /api/v1/pilot/review/{token}
```

## Authenticated User And Repo

```text
GET    /api/v1/me/repos
DELETE /api/v1/me
POST   /api/v1/beta/feedback

GET    /api/v1/repos/{repoID}
POST   /api/v1/repos/{repoID}/select
POST   /api/v1/repos/{repoID}/index
DELETE /api/v1/repos/{repoID}/index
GET    /api/v1/repos/{repoID}/status
GET    /api/v1/repos/{repoID}/queue
```

## Cloud Graph API

```text
GET  /api/v1/repos/{repoID}/programs
GET  /api/v1/repos/{repoID}/programs/{nodeID}/graph
GET  /api/v1/repos/{repoID}/graph
POST /api/v1/repos/{repoID}/graph/expand
GET  /api/v1/repos/{repoID}/nodes/{nodeID}/code

GET  /api/v1/repos/{repoID}/prs/{prID}/graph
GET  /api/v1/repos/{repoID}/prs/{prID}/issue?owner={owner}&repo={repo}&number={number}
GET  /api/v1/repos/{repoID}/prs/number/{number}/graph
GET  /api/v1/repos/{repoID}/prs/{prID}/nodes/{nodeID}/code
```

`POST /graph/expand` body:

```json
{
  "node_id": "node-id",
  "visible_node_ids": ["node-id"],
  "graph_context": { "mode": "repo" }
}
```

PR context:

```json
{
  "node_id": "node-id",
  "visible_node_ids": ["node-id"],
  "graph_context": { "mode": "pr", "pr_id": "pr-id" }
}
```

## Frontend Rewrites

The Next web app rewrites:

```text
/api/v1/*   -> NEXT_PUBLIC_API_URL/api/v1/*
/webhooks/* -> NEXT_PUBLIC_API_URL/webhooks/*
```

It does not rewrite `/debug/*`.

## Relationship To CLI API

The cloud API should not be treated as the canonical route design for local. Cloud routes are database/GitHub/App-session oriented. Local routes should remain checkout/daemon oriented. The shared contract lives in React adapters and graph payload types, not in identical HTTP paths.
