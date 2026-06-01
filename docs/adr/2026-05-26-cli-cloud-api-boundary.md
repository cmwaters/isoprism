# ADR: Separate CLI And Cloud APIs, Share React Components

## Status

Accepted.

## Context

The CLI and cloud products both render semantic repository graphs, but their API designs are naturally different.

Cloud is organized around:

- Supabase users
- GitHub App installations
- repositories in Postgres
- PR webhook processing
- indexing jobs
- PR queue and AI summaries

CLI is organized around:

- a local git checkout
- local refs and blobs
- `.isoprism/` cache files
- daemon-served graph snapshots
- optional PR metadata from `gh`

The first local viewer reused cloud-shaped routes so it could mount existing graph UI quickly. That route shape is compatibility scaffolding, not the desired long-term local API.

## Decision

Keep CLI and cloud APIs separate. Share React components and TypeScript graph/repo contracts through adapters.

The common UI receives:

```ts
type RepoWorkspaceModel = {
  repo: RepoInfo;
  programs: GraphProgram[];
  reviewItems?: ReviewItem[];
  graphClient: GraphClient;
};
```

`reviewItems` may be empty or omitted. The shared UI must not assume a PR queue exists.

Cloud maps its PR queue to review items. CLI may provide no review items, populate them from local diffs, staged diffs, unstaged diffs, or populate them from open `gh` pull requests.

## Consequences

- Cloud routes can stay auth/database/GitHub-App oriented.
- Local routes can become checkout/daemon/git oriented.
- React graph and repo components remain shared.
- The local API no longer needs fake cloud repository semantics.
- Local performance can improve by creating one daemon graph snapshot per session/ref instead of rebuilding graph data for every cloud-shaped endpoint.

## Follow-Up Work

1. Add a shared `RepoWorkspace` and `GraphClient` interface.
2. Add cloud and local graph client adapters.
3. Replace cloud-shaped compatibility routes with canonical CLI routes under `/api/...`.
4. Keep compatibility routes temporarily only if needed during migration.
5. Extend `gh` PR discovery with richer issue context when the local review surface needs it.
