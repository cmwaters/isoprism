# CLI UI

## Scope

The CLI UI is the local browser experience served by `isoprism serve`. It should reuse common graph/repo components wherever possible, while keeping local-only shell behavior and data adapters separate from cloud.

## Local Shell

The installed CLI serves an embedded React viewer from:

```text
http://127.0.0.1:3717/local
```

The local shell owns:

- loading local session metadata
- creating the local graph client
- mapping daemon responses into shared component props
- displaying local-only loading and error states
- omitting cloud-only navigation such as settings, pilot feedback, onboarding, and auth

## Repo Workspace Props

The shared workspace should accept:

```ts
type RepoWorkspaceModel = {
  repo: RepoInfo;
  programs: GraphProgram[];
  reviewItems?: ReviewItem[];
  graphClient: GraphClient;
};
```

For local, `reviewItems` may be `[]` or omitted. The panel should not show an empty PR queue unless a product explicitly asks for one.

## Review Compare

The local repo panel includes a `Review` section above `Programs`. It allows the user to compare two local git states and render the result as a semantic review graph.

Default inputs:

- Base: detected default branch, normally `main`
- Compare: `worktree`

The compare input accepts branches, tags, commits, `staged`, `worktree`, `working-tree`, or `unstaged`. When the user runs a comparison, the daemon indexes uncached branches, commits, staged blobs, or working-tree files before calculating the semantic diff.

## Local-Unique Components

Local-specific components should remain thin:

- `LocalViewerShell`: boots the local app and fetches session data.
- `LocalRepoAdapter`: maps local repo/program/review responses into shared workspace props.
- `LocalGraphClient`: calls local daemon routes.
- Local loading/error views: show daemon or git errors plainly.

## Shared Components Used By CLI

The CLI should reuse:

- repo workspace layout
- repo/program side panel primitives
- optional review item list
- graph canvas
- graph node cards
- node detail panel
- code/diff rendering
- graph controls

## Out of Scope For CLI UI

- Supabase auth
- GitHub App install flow
- pilot registration or review forms
- cloud settings
- beta feedback banner
- admin console
