# Isoprism CLI Implementation Plan

## Summary

Isoprism is pivoting from a GitHub OAuth web prototype to a local-first CLI and local browser experience. The CLI is the core product: it reads the local git repository, builds a semantic graph without GitHub permissions, and produces either a portable static diff view or a persistent local graph browser.

The current hosted prototype should not be discarded. The parser, semantic graph extraction, PR diff classification, graph selection, dynamic expansion behavior, and React graph UI are the foundation of the CLI. The work is primarily a delivery and storage refactor: replace GitHub App and Postgres as the primary runtime dependencies with local git, a committed `.isoprism/` cache, and a static/local-server frontend.

## Product Shape

The CLI exposes two primary modes:

```bash
isoprism diff
isoprism serve
```

`isoprism diff` generates a self-contained static HTML semantic diff for the current branch, staged changes, unstaged changes, or explicitly provided refs. It opens the file in the browser by default. This is the pre-push and PR-review surface.

`isoprism serve` starts a local server for ambient semantic browsing of the full repository. It reuses the same graph engine and frontend, but serves graph data dynamically instead of embedding a bounded payload into one file.

The CLI should also read adjacent context when available:

- `gh` CLI for PR title, body, URL, base branch, and issue references.
- Linear or Jira issue APIs for ticket intent.
- Entire checkpoints from the `entire/checkpoints/v1` branch when present.
- Isoprism annotations written by AI assistants or humans when Entire is not present.

## CLI Commands

### `isoprism diff`

Default behavior:

```bash
isoprism diff
```

Compare the current branch tip against the repository default branch. The default branch must be detected from git, not hardcoded as `main`.

Supported forms:

```bash
isoprism diff
isoprism diff <ref>
isoprism diff <ref1> <ref2>
isoprism diff staged
isoprism diff unstaged
```

Resolution rules:

- `isoprism diff` compares the default branch to `HEAD`.
- `isoprism diff <ref>` compares `<ref>` to `HEAD`.
- `isoprism diff <ref1> <ref2>` compares the two named refs directly.
- `isoprism diff staged` compares the git index against `HEAD`.
- `isoprism diff unstaged` compares the working tree against `HEAD`.
- `staged` and `unstaged` are reserved keywords, not branch names.

Default branch detection order:

1. `git symbolic-ref refs/remotes/origin/HEAD`
2. `git remote show origin`
3. local `main`
4. local `master`
5. fail loudly with a message explaining how to pass an explicit ref

Ref resolution order:

1. exact commit hash
2. local branch name
3. remote branch name as `origin/<name>`

Useful flags:

```bash
isoprism diff --output <path>
isoprism diff --open
isoprism diff --no-open
isoprism diff --json
isoprism diff --cache-dir <path>
isoprism diff --share
isoprism diff --rebuild-cache
```

`--share` is Phase 2. It uploads the generated payload to Isoprism cloud and returns a permanent URL. The local static HTML path must work without it.

### `isoprism serve`

```bash
isoprism serve
isoprism serve --port 3717
isoprism serve --host 127.0.0.1
```

`serve` starts a local-only HTTP server by default. It should not bind to a public interface unless the user explicitly passes a non-loopback host.

The server provides the full repo browsing experience:

- list semantic entrypoints/programs
- load a bounded program graph
- expand nodes dynamically
- show node details and source
- update when files change

## Local Git Integration

Git is the source of truth for refs, branches, commits, trees, and blob identity. Isoprism must not mirror git branch refs in `.isoprism/`.

Core git operations:

```bash
git rev-parse --show-toplevel
git rev-parse --git-dir
git rev-parse <ref>
git cat-file -e <sha>^{commit}
git ls-tree -r -z <ref>
git diff --name-status --find-renames <from> <to>
git diff --patch <from> <to>
git show <ref>:<path>
git write-tree
```

For normal committed refs, `git ls-tree` supplies every file path and git blob SHA. Those blob SHAs drive cache lookup and determine which source files need parsing.

For staged mode, use the git index as the head-side tree. For unstaged mode, read changed tracked files from the working tree and synthesize semantic nodes for their current disk content. Untracked-file support can be added after the first working-tree implementation.

## Storage Layout

The default cache is committed `.isoprism/`. This gives every engineer a warm cache after clone, requires no infrastructure, and creates an audit trail of semantic graph evolution.

```text
.isoprism/
  objects/
    nodes/
      <node-sha256>
    index/
      incoming_links/
        <node-sha256>
      blob_to_nodes/
        <git-blob-sha>
  annotations/
    <from-commit-sha>..<to-commit-sha>/
      diff_summary
      node_changes/
        <node-sha256>
```

There is intentionally no `.isoprism/refs/` directory. Git already owns branch and commit state.

## Node Objects

Each file under `.isoprism/objects/nodes/<node-sha256>` stores one semantic node:

```json
{
  "type": "function",
  "full_name": "package.Type.Method",
  "filepath": "internal/service/foo.go",
  "git_blob_sha": "abc123",
  "line_start": 42,
  "line_end": 79,
  "outgoing_links": [
    {
      "relation_type": "calls",
      "target": "internal/service/bar.go::package.OtherFunction"
    }
  ]
}
```

Schema:

```text
type          enum: function | class | method | interface | constant | test
full_name     string
filepath      string
git_blob_sha  string | null
line_start    int | null
line_end      int | null
outgoing_links array of {
                relation_type  enum: calls | imports | uses_type | owns_method | references
                target         semantic reference in "<filepath>::<full_name>" format
              }
```

Rules:

- Internal nodes have a `git_blob_sha`.
- External or unresolved nodes have `git_blob_sha: null`.
- `filepath` is repo-relative for internal nodes.
- `filepath` is an import path or module path for external/unresolved nodes.
- `outgoing_links[]` unifies resolved and unresolved relationships.
- Each `outgoing_links[]` entry records the relation type and the target node's filepath plus full name, formatted as `<filepath>::<full_name>`.
- `relation_type` should reuse the semantic edge kinds the graph engine already produces, starting with `calls`, `owns_method`, and `uses_type`; `imports` and `references` are available for language/module relationships that are not function calls.
- Resolved and unresolved links use the same target reference format. Resolution happens by looking up that semantic reference in the active graph's node index; unresolved/external node objects still have `git_blob_sha: null`.

`node-sha256` must be deterministic and version-aware. The hash input should include the parser schema version plus stable semantic identity fields. It should not include AI annotations.

Initial hash input:

```text
isoprism-node-v1
type
full_name
filepath
git_blob_sha
```

This means a changed file version creates new internal node identities through the new git blob SHA. If later product behavior needs stable logical identity across versions, add a separate `logical_node_id` rather than weakening the content-addressed object model.

## Indexes

### `blob_to_nodes`

Path:

```text
.isoprism/objects/index/blob_to_nodes/<git-blob-sha>
```

Value:

```json
{
  "package.Type.Method": "node-sha256",
  "package.OtherFunction": "node-sha256"
}
```

Meaning: all semantic nodes contained in this exact file blob, keyed by `full_name`.

Conceptually:

```text
<git-blob-sha>
  {full_name} -> node sha256
  {full_name} -> node sha256
```

Use:

- cache hit for unchanged git blobs
- fast full-name-to-node lookup inside a known git blob
- reuse git's tree/blob storage as the primary file-version index
- no AST work when the blob has already been parsed

### `incoming_links`

Path:

```text
.isoprism/objects/index/incoming_links/<node-sha256>
```

Value:

```json
[
  {
    "relation_type": "calls",
    "source": "internal/service/foo.go::package.Type.Method"
  },
  {
    "relation_type": "uses_type",
    "source": "internal/service/bar.go::package.OtherFunction"
  }
]
```

Meaning: typed incoming relationships for nodes that import, call, use, own, or reference this node. `source` is the source node's semantic reference, formatted as `<filepath>::<full_name>`.

Use:

- include incoming linked nodes around changed nodes
- compute impact neighborhoods efficiently
- support `serve` expansion without scanning every node object

Update behavior:

- On a blob cache miss, parse the file and write its node objects.
- Write `blob_to_nodes/<git-blob-sha>` as a `{full_name: node_sha256}` map.
- For every `outgoing_links[]` relationship, resolve `target` to the target node object for the active graph and add `{relation_type, source}` to `incoming_links/<target-node-sha256>`.
- When rebuilding a blob that already has cached nodes for the same blob SHA, skip all writes.
- Because git blob SHAs are immutable, `blob_to_nodes` does not need mutation for an existing blob; a changed file gets a new git blob SHA and therefore a new full-name map.
- Because `incoming_links` stores source semantic references and relation types instead of source node SHAs, link indexes do not need to be rewritten every time the source node's file content changes. The active git tree plus `blob_to_nodes` resolves those references to the current node objects.

If parser schema changes, Isoprism should either store schema-versioned indexes or require `isoprism diff --rebuild-cache`.

## Diff Annotations

Annotations are separate from semantic node objects so human and AI reasoning does not invalidate cache objects.

Path:

```text
.isoprism/annotations/<from-commit-sha>..<to-commit-sha>/
```

`diff_summary`:

```json
{
  "issue_link": "string | null",
  "pr_link": "string | null",
  "reason_for_change": "string",
  "expected_outcome": "string",
  "alternatives_considered": "string | null",
  "known_gaps": ["string"],
  "test_assertions": [
    {
      "description": "string",
      "node_sha256": "string"
    }
  ]
}
```

`node_changes/<node-sha256>`:

```json
{
  "description": "string",
  "reasoning": "string",
  "confidence": "high",
  "risks": "string | null",
  "follow_up": "string | null"
}
```

Annotation commands:

```bash
isoprism annotate diff \
  --issue-link "..." \
  --pr-link "..." \
  --reason-for-change "..." \
  --expected-outcome "..." \
  --alternatives-considered "..." \
  --known-gap "..."

isoprism annotate node <node-sha256> \
  --description "..." \
  --reasoning "..." \
  --confidence high \
  --risks "..." \
  --follow-up "..."

isoprism annotate test \
  --node <node-sha256> \
  --description "..."
```

The static renderer surfaces:

- diff summary in the overview panel
- issue and PR links in the overview panel when present
- node change annotations in the selected node detail panel
- known gaps and risks near the relevant changed nodes
- test assertions in the test changes section

## `isoprism diff` Generation Flow

1. Resolve repository root and git dir.
2. Resolve the requested comparison using git.
3. Load changed files, rename metadata, and patches from git.
4. Load both compared trees with `git ls-tree`.
5. For every supported source blob in each tree:
   - check `objects/index/blob_to_nodes/<git-blob-sha>`
   - parse and write node objects on cache miss
   - load node objects on cache hit
6. Compare node sets between from/to sides:
   - added nodes
   - deleted nodes
   - modified nodes
   - renamed nodes when git rename metadata or body/hash matching supports it
7. Build semantic neighborhoods:
   - changed nodes as seeds
   - direct typed outgoing relationships from `outgoing_links[]`
   - incoming typed relationships from `objects/index/incoming_links/`
8. Build graph payload using the same shape as the existing frontend where practical.
9. Attach annotations from `.isoprism/annotations/<from>..<to>/`.
10. Attach PR/issue context from `gh`, Linear, Jira, or commit messages when available.
11. Generate a self-contained static HTML file.
12. Open it in the browser unless `--no-open` is set.

Warm-cache target: under 5 seconds for normal branch diffs.

Performance comes from:

- git blob SHA cache hits
- no full repo reparsing when only a few blobs changed
- index lookup for incoming relationships
- bounded graph rendering for static diff mode

## Static HTML Output

The static output embeds all data needed to review the bounded diff:

- graph nodes
- graph edges
- file diffs
- test changes
- source snippets for changed nodes
- diff summary annotations
- node change annotations
- PR and issue context
- generation metadata

The output must not require a running Isoprism process. It should be safe to attach to Slack, email, GitHub Actions artifacts, or GitHub Pages.

The static viewer reuses the current React graph UI with a static data source. Dynamic expansion is not required for v1 static output; the file should include the relevant bounded neighborhood up front.

## `isoprism serve` Local Server

`serve` reuses the same graph engine but serves data dynamically.

Local API shape:

```http
GET  /api/repo/programs
GET  /api/repo/programs/{nodeID}/graph
POST /api/repo/graph/expand
GET  /api/nodes/{nodeID}/code
GET  /api/diff
```

Behavior:

- Load the default branch or current working tree graph from local git/cache.
- Show entrypoint/program nodes first.
- Load bounded graph neighborhoods on demand.
- Expand direct callers/callees on node click.
- Keep type/class contents in the panel first, then materialize selected contents into the graph.
- Watch files and refresh affected blobs as files change.
- Notify the browser via SSE or WebSocket when graph data changes.

This mode is the local precursor to the eventual hosted SaaS graph browser.

## Shared Frontend

The current Next.js graph UI should be extracted into a reusable viewer package.

Targets:

- hosted prototype
- static HTML diff viewer
- local `serve` browser
- future SaaS app

Implementation approach:

- Keep graph components framework-light where possible.
- Replace direct `apiFetch` assumptions with a data source interface.
- Provide a `StaticGraphDataSource` for embedded HTML data.
- Provide a `LocalServerGraphDataSource` for `isoprism serve`.
- Keep the existing visual behavior: weighted graph layout, graph node cards, node detail panel, test changes, file diffs, markdown PR/issue rendering, and dynamic expansion.

## AI Skill Contract

The free CLI should be paired with a short AI skill for Codex and Claude.

Primary flows:

1. Pre-push review:
   - after meaningful changes or before push, run `isoprism diff`
   - open the generated HTML
   - inspect semantic changes before pushing
   - write annotations when the reasoning is not already captured elsewhere

2. PR review:
   - check out/fetch the PR branch
   - run `isoprism diff <base> <head>` or `isoprism diff`
   - use the semantic graph as the primary review surface
   - document findings against semantic nodes where possible

Entire detection:

```bash
git show-ref --verify --quiet refs/remotes/origin/entire/checkpoints/v1
git show-ref --verify --quiet refs/heads/entire/checkpoints/v1
```

If Entire checkpoints exist, Isoprism should read associated checkpoints for the compared commits and surface that reasoning in the static diff. The AI skill should not duplicate reasoning unless the checkpoint is missing or insufficient.

If Entire is absent, the skill should write Isoprism annotations using:

```bash
isoprism annotate diff ...
isoprism annotate node <node-sha256> ...
isoprism annotate test ...
```

Positioning:

- Entire captures why the code was written.
- Isoprism shows what the code did to the system.
- Isoprism should integrate with Entire, not compete with it.

## GitHub Actions Integration

Phase 2 adds a CI workflow that generates static HTML for every PR.

Example:

```yaml
name: Isoprism Semantic Diff

on:
  pull_request:

jobs:
  isoprism:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
      actions: write
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - uses: isoprism/setup-isoprism@v1
      - run: isoprism diff origin/${{ github.base_ref }} HEAD --output isoprism-diff.html --no-open
      - uses: actions/upload-artifact@v5
        with:
          name: isoprism-diff
          path: isoprism-diff.html
      - uses: actions/github-script@v8
        with:
          script: |
            // Create or update an Isoprism PR comment with the artifact link.
```

Initial sharing uses GitHub Actions artifacts. GitHub Pages can provide stable links without Isoprism cloud. Later, `isoprism diff --share` uploads payloads to Isoprism SaaS and returns a permanent URL.

## Migration From Current Prototype

Reusable pieces:

- tree-sitter parser
- Go, TypeScript, TSX, JavaScript, and JSX support
- call graph extraction
- receiver/type ownership relationships
- type usage relationships
- PR changed-node classification
- component diff hunk generation
- graph weighting and bounded selection
- dynamic graph expansion logic
- React Flow graph canvas
- graph node cards
- node detail panel
- test changes and file diff presentation
- markdown rendering for PR/issue context

Replaced pieces:

- GitHub OAuth as required user entrypoint
- GitHub App as required source reader
- Supabase/Postgres as required graph cache
- webhook-driven default-branch indexing as the primary update mechanism
- hosted API as the only graph data provider

Sequencing:

1. Extract shared parser and semantic graph packages.
2. Add local git reader and ref/diff resolver.
3. Implement `.isoprism/objects/nodes`.
4. Implement `blob_to_nodes` and `incoming_links` indexes.
5. Implement `isoprism diff <ref1> <ref2> --json`.
6. Generate static HTML from embedded JSON.
7. Extract/reuse graph frontend for static viewer.
8. Add staged and unstaged modes.
9. Add annotations.
10. Add Entire checkpoint ingestion.
11. Add `isoprism serve`.
12. Add GitHub Actions integration.
13. De-emphasize hosted OAuth prototype after CLI review is usable.

## Shortest Path To A Working Diff

The first useful milestone is:

```bash
isoprism diff <base> <head> --output isoprism.html
```

Minimum behavior:

- committed refs only
- supported languages only
- parse changed blobs and cache by git blob SHA
- generate changed semantic node list
- include one-hop outgoing and incoming graph context
- render static HTML with current graph cards and detail panel
- include file patches from git
- no annotations, Entire, Linear, Jira, sharing, or dynamic expansion yet

This proves the new architecture without waiting for the full local server or AI skill loop.

## Current Implementation Status

The repo now includes an initial local CLI implementation under `api/cmd/isoprism`:

```bash
cd api
go run ./cmd/isoprism diff
go run ./cmd/isoprism diff <ref>
go run ./cmd/isoprism diff <ref1> <ref2>
go run ./cmd/isoprism diff staged
go run ./cmd/isoprism diff <ref1> <ref2> --json
go run ./cmd/isoprism diff <ref1> <ref2> --output /tmp/isoprism.html --no-open
go run ./cmd/isoprism serve --port 3717
go run ./cmd/isoprism annotate diff --reason-for-change "..." --expected-outcome "..."
go run ./cmd/isoprism annotate node <node-sha256> --description "..." --reasoning "..." --confidence high
go run ./cmd/isoprism annotate test --node <node-sha256> --description "..."
```

Implemented behavior:

- `diff` resolves the repository root from local git.
- `diff` detects the default branch from `origin/HEAD`, `git remote show origin`, `main`, then `master`.
- `diff <ref>` and `diff <ref1> <ref2>` work for committed refs.
- `diff staged` compares `HEAD` against the current git index without calling `git write-tree`, so it works in restricted environments that cannot create `.git/index.lock`.
- `--json` emits a `ReviewGraphPayload` that wraps the existing website `GraphResponse` shape.
- `--output` writes self-contained static HTML with the embedded review payload and file patches.
- `--open` is on by default, and `--no-open` disables browser launch.
- `.isoprism/objects/nodes` and `.isoprism/objects/index/blob_to_nodes` cache parsed semantic nodes by git blob SHA.
- `--rebuild-cache` removes `.isoprism/objects` and rebuilds parser-derived objects without deleting annotations.
- `serve` binds to `127.0.0.1:3717` by default and exposes the initial local API endpoints:
  - `GET /`
  - `GET /api/diff`
  - `GET /api/repo/programs`
  - `GET /api/repo/programs/{nodeID}/graph`
  - `POST /api/repo/graph/expand`
  - `GET /api/nodes/{nodeID}/code`
- `annotate` writes documented annotation JSON under `.isoprism/annotations/<base-sha>..<head-sha>/`.
- When the index contains staged changes, `annotate` targets the same `HEAD..index` range as `diff staged`; otherwise it targets the default `default-branch..HEAD` range.

Current intentional gaps:

- `diff unstaged` is not implemented yet. It needs a working-tree overlay model that can parse tracked disk contents, represent deleted and renamed files, and decide whether untracked supported files are included or reported as deferred.
- `serve` returns the generated review payload dynamically, but it does not yet watch files, push SSE/WebSocket refresh events, or maintain a persistent full-repo browsing graph independent of the active diff.
- `GET /api/nodes/{nodeID}/code` returns source for nodes included in the active review payload. Full-repo source lookup for nodes outside the bounded diff graph belongs with the persistent `serve` graph.
- The static HTML is a self-contained readable artifact with embedded JSON, but it does not yet boot the extracted React graph viewer. Achieving website visual parity requires the frontend extraction described below.
- Incoming link indexes are not persisted yet. The first implementation computes one-hop context from the active base/head graphs during payload generation.

These gaps are documented explicitly because pretending to have full website parity here would make the CLI contract less reliable, not more.

## Missing Or Underspecified Work

The plan is directionally complete, but several implementation contracts need to be specified before the CLI can behave like the website.

### Review Graph Payload Contract

The CLI must emit the same canonical graph payload shape used by the website, not a similar-looking CLI-only shape. This payload is an ephemeral view model, not a separate graph storage layer. Git commits and trees remain the source of truth for graph state at any ref; `.isoprism/` only caches semantic node/index objects that are derived from git blobs.

Define a shared `ReviewGraphPayload` contract that can be converted into the current frontend types:

- repo metadata
- diff metadata
- nodes
- edges
- file diffs
- test changes
- test context
- annotations
- issue/PR context
- source/code segments used by the detail panel

The payload should preserve current website semantics:

- changed production nodes appear in the graph
- changed tests appear in `test_changes`
- test helpers can be present for focused test views without becoming normal production graph nodes
- docs/config/other files stay in file diff sections
- type/class nodes show contents first, then materialize selected contents into the graph
- relation kinds stay visible as `calls`, `owns_method`, `uses_type`, `imports`, or `references`

The payload is generated on demand by combining:

- git's before/after commit or tree state
- `.isoprism` node objects and link indexes for the blobs in those trees
- `git diff` file changes and patches
- Isoprism annotations
- optional PR/issue context

### Node Identity And Link Resolution

The storage model needs exact rules for turning semantic references into current node objects:

- semantic reference format is `<filepath>::<full_name>`
- `blob_to_nodes/<git-blob-sha>` maps `full_name` to node SHA for one file version
- resolving a semantic reference requires the active git tree to map `filepath` to `git_blob_sha`, then `blob_to_nodes` maps `full_name` to the current node SHA
- if the active tree does not contain that path/full name, the link resolves to an external/unresolved node with `git_blob_sha: null`
- duplicate `full_name` values inside one file blob must fail loudly unless the parser can produce a deterministic disambiguator

This should be implemented as a shared resolver used by both `diff` and `serve`.

Important: Isoprism does not need to store a complete review graph payload for every commit. To determine the graph at a commit, read that commit's git tree, map each path to a blob SHA, read `blob_to_nodes/<git-blob-sha>`, and load those node objects. Links are resolved relative to that active tree. This reuses git's storage model for file/version indexing and keeps `.isoprism` focused on semantic objects.

### Changed Node Classification

Node changes are computed on the fly for each diff. The CLI first gets the git diff, then resolves the node sets before and after the change, writes any missing semantic objects for blobs it has not seen before, and derives the review view from those before/after node sets plus annotations.

The plan needs parity rules for added, modified, deleted, and renamed nodes:

- unchanged: same semantic reference and same body hash
- modified: same semantic reference, different body hash
- added: present only on head side
- deleted: present only on base side
- renamed: paired by git rename metadata and either matching body hash or conservative parser-supported identity evidence

Line-range overlap alone must not classify renames.

Diff view generation should be specified as:

1. Use git to resolve the before and after trees.
2. Use `git diff` to get changed files, rename metadata, and patches.
3. For every supported blob in the before/after trees, load nodes from `blob_to_nodes` or parse and write them if missing.
4. Build before-node and after-node maps keyed by semantic reference `<filepath>::<full_name>`.
5. Classify node changes from those two maps.
6. Use changed nodes as graph seeds.
7. Add outgoing context from `outgoing_links[]` and incoming context from `incoming_links`.
8. Attach annotations and external context.
9. Serialize the resulting `ReviewGraphPayload` for JSON or static HTML.

### Source And Diff Detail Behavior

The website has important detail-panel behavior that the CLI must preserve:

- modified and renamed nodes should show base and head component source when both sides exist
- added nodes should show head source
- deleted nodes should show base source
- if source reconstruction is incomplete, fall back to the persisted component/file patch
- file-level patches from git are the source of truth for documentation/config/other changes

The static HTML file must embed enough source/code segments to make the detail panel work without network calls.

### Cache Versioning And Writes

The storage layer needs operational rules:

- every object/index file should include or be scoped by a parser/storage schema version
- writes should be atomic: write temp file, fsync where practical, then rename
- concurrent CLI runs should use a lock file around writes to `.isoprism/objects`
- `--rebuild-cache` should remove or ignore stale versioned objects without deleting annotations
- malformed cache objects should fail loudly with the object path and recovery command

### Working Tree Modes

`staged` and `unstaged` need precise tree semantics:

- `staged` compares `HEAD` to the git index tree
- `unstaged` compares `HEAD` to tracked working-tree file contents
- mixed staged plus unstaged changes should be reported clearly
- deleted, renamed, mode-only, binary, unsupported, and untracked files need explicit behavior
- untracked supported source files should be either included or explicitly deferred with a warning

### `serve` Update Semantics

The local server needs a concrete invalidation model:

- file watcher debounces changes
- changed tracked files recompute only their current blob-equivalent working-tree node set
- deleted files remove their visible working-tree nodes
- branch checkout invalidates the active graph and reloads from git
- browser clients receive graph-refresh notices through SSE or WebSocket

### External Context Failure Behavior

Adjacent tools must be optional:

- missing `gh`, Linear, Jira, Entire, or network access must not block graph generation
- failures should be captured as non-fatal metadata in the payload
- `issue_link` and `pr_link` should prefer explicit annotation values, then `gh`, then inferred commit/branch references

## CLI Parity Test Framework

The CLI should be tested against the same contract the website uses. The goal is not just parser correctness; it is that `isoprism diff` and `isoprism serve` produce graph data and UI behavior that are almost indistinguishable from the current website for the same repo state.

### Test Principle

Use one shared normalized graph contract for both products:

```text
website API response -> normalize -> ReviewGraphPayload
CLI JSON output      -> normalize -> ReviewGraphPayload
```

Normalize only volatile fields:

- database UUIDs
- timestamps
- generated file paths
- provider-specific metadata ordering

Do not normalize semantic fields:

- full names
- file paths
- line ranges
- node types
- relation types
- change types
- diff hunks
- file patch grouping
- graph depth, weight, degree, and boundary flags
- test changes

If normalized payloads differ, the test should fail with a semantic diff that names the node, edge, file, or annotation that drifted.

### Fixture Repositories

Create fixture git repositories under `testdata/repos/` or generate them in temp dirs during tests. Each fixture should contain real git commits and branches.

Minimum fixture matrix:

- Go function call changed
- Go method call changed
- Go struct `uses_type` edge changed
- Go receiver `owns_method` relationship
- TypeScript function and import graph
- TSX component with adjacent helper
- changed test entrypoint
- changed test helper
- docs-only PR
- config/package metadata PR
- renamed file with unchanged node
- renamed function with unchanged body
- deleted node
- added node
- staged-only change
- unstaged-only change
- unsupported file plus supported file
- unresolved external import/call

Each fixture should define:

- base ref
- head ref
- expected changed files
- expected semantic nodes
- expected relation types
- expected graph section membership

### Backend Parity Tests

Keep the current website backend as the oracle while migrating.

For each fixture:

1. Run the existing website graph pipeline through a local adapter or recorded GitHub fixture data.
2. Capture the current API-style graph response.
3. Run `isoprism diff <base> <head> --json`.
4. Normalize both into `ReviewGraphPayload`.
5. Compare exact semantic equality.

This specifically protects:

- parser output
- semantic edges
- changed-node classification
- PR/file diff grouping
- test-change behavior
- graph selection and boundary flags
- node detail source/diff data

Once the CLI becomes the source of truth, keep these parity fixtures as regression tests for the hosted/SaaS path.

### Storage Tests

Storage tests should run without the web stack:

- `blob_to_nodes` stores `{full_name: node_sha256}` for each parsed blob
- unchanged blob SHA is a cache hit and performs no parse
- changed file creates a new blob map without mutating the old one
- `outgoing_links[]` stores typed targets
- `incoming_links/<node-sha256>` stores typed source references
- incoming links resolve through the active git tree plus `blob_to_nodes`
- unresolved/external targets produce node objects with `git_blob_sha: null`
- parser/storage version mismatch fails with a useful recovery message
- concurrent cache writes do not corrupt object files

### CLI Behavior Tests

Run the compiled CLI against fixture repos:

```bash
isoprism diff --json
isoprism diff <ref>
isoprism diff <ref1> <ref2> --json
isoprism diff staged --json
isoprism diff unstaged --json
isoprism diff --output out.html --no-open
```

Assert:

- default branch detection
- ref resolution order
- reserved keyword behavior
- correct exit codes
- useful error messages
- no network requirement
- deterministic JSON across repeated runs
- warm-cache run parses zero unchanged blobs
- generated HTML contains the embedded review graph payload

### Frontend Parity Tests

The shared graph UI should run against two data sources:

- website/API data source
- static CLI data source

Use the same fixture `ReviewGraphPayload` for both. Browser tests should verify:

- graph node count and labels
- edge count and relation styles
- changed-node pills
- PR/diff overview sections
- file patch rendering
- issue and PR links
- node detail overview
- code/diff panel behavior
- type contents panel behavior
- test changes section
- graph expansion behavior in server mode

Prefer DOM and semantic assertions first. Add screenshot tests for the highest-risk visual surfaces: graph canvas, overview panel, node detail panel, and code/diff panel.

### Static HTML Tests

For `isoprism diff --output`:

- generate HTML from fixtures
- load it in Playwright
- assert the app boots without network calls
- assert embedded payload checksum matches CLI JSON output
- assert selecting changed nodes opens the same detail content as the website fixture
- assert file can be opened directly from disk or served as a static artifact

### `serve` Tests

Start `isoprism serve` against fixture repos and test local APIs:

- programs endpoint matches expected entrypoints
- program graph matches bounded website behavior
- expansion endpoint returns the same newly visible nodes and edges as the website expansion contract
- node code endpoint returns correct base/head segments
- file edits trigger cache updates and browser refresh events
- branch checkout reloads active graph

### CI Test Tiers

Use three tiers so the suite stays fast:

1. Unit: parser, storage, git resolver, payload normalizer.
2. Contract: fixture repos, CLI JSON, website parity payloads.
3. Browser: static HTML and `serve` parity through Playwright.

Every CLI change should run unit and contract tests. Browser parity should run in CI and before releases.

## Documentation Updates

As implementation lands, keep documentation source-of-truth aligned:

- `cli.md`: CLI architecture, storage, and implementation plan.
- `graph.md`: graph selection, node model, static vs serve behavior.
- `ai.md`: local annotation and Entire integration contract.
- `engineering_design_doc.md`: local-first product architecture.
- `AGENTS.md` and `CLAUDE.md`: AI assistant workflow with `isoprism diff`.
- `web/README.md`: frontend extraction and local viewer development.

## Open Decisions

- Whether `node-sha256` should include `git_blob_sha` permanently or whether stable logical node IDs should be introduced alongside versioned node objects.
- Exact static frontend build system: Vite bundle, Next export, or a dedicated viewer package.
- How much working-tree and untracked-file support is required in the first `unstaged` implementation.
- Whether `.isoprism/` committed cache should be mandatory or configurable per repo via `.isoprism/config`.
