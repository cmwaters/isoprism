# CLI Overview

## Purpose

The Isoprism CLI is a local-first way to visualize changes that AI agents have made to a codebase. Its primary job is to make agent-authored changes easier for developers to understand than raw diffs: what changed, which semantic components were affected, and how those components connect to the rest of the system.

The local runtime is important because it lowers the adoption barrier. A developer can run Isoprism inside a git checkout without asking for GitHub repository permissions, installing a GitHub App, or sending the repository through a hosted workflow. It also lets Isoprism live beside AI coding interfaces such as Codex, Claude Code, and similar tools, where a native or integrated browser panel can show the graph while the developer reviews the agent's work.

The CLI is not a thin clone of the cloud product. It has its own runtime model, storage model, and API surface. The shared product surface is the React graph/repo UI and the graph data contracts those components consume.

## Product

The CLI should answer two local questions:

- How does this system work?
- How do these changes to the system solve the previous problem?

The initial CLI surface is:

```bash
isoprism serve
isoprism diff
isoprism annotate
```

`isoprism serve` starts a local daemon server and browser viewer for interactive graph exploration. This is the primary tool used in reviewing as it can dynamically load and display the graph from storage. It supports the review flow by making it easy to keep Isoprism open beside the coding agent and inspect surrounding code as needed.

`isoprism diff` creates a semantic diff for a ref range, staged changes, or unstaged changes. This is a standalone snapshot primarily useful as a sharable attachment (could be used in C.I).

`isoprism annotate` records human or AI-provided context about the problem, intended outcome, changed nodes, and test assertions. Annotations let tools such as Codex or Claude Code leave reviewable breadcrumbs that the graph UI can show alongside the semantic diff.

Local review contexts include current checkout changes and may also come from the `gh` CLI. The daemon exposes local review cards for uncommitted changes (`HEAD -> worktree`) and for the current worktree as it would appear in a GitHub PR against the default branch (`worktree -> origin/<default-branch>`). When available, the daemon also uses `gh` to discover pull request title, body, URL, base branch, head branch, author, and diff stats. Those items share one Review section in the React workspace. Selecting a GitHub PR fetches the PR head into a hidden local ref and visualizes the semantic base..head graph in the same PR review surface used by the cloud product. A local repo with no active review context may have an empty or omitted review item list, and the UI should simply show repo/program graph browsing.

## User Journey

There are two primary journeys:

### Reviewing your own AI generated work

1. The user works in a git checkout with an AI coding tool such as Codex or Claude Code.
2. The AI tool changes code.
3. The user runs `isoprism serve` open in an integrated browser panel.
4. The CLI detects the repo root, relevant git state, and cache directory. It builds the relevant semantic graph.
5. The browser UI shows a graph-first view of the changed components and their surrounding context.
6. The user expands nodes, opens source details, and compares the semantic graph against the raw diff only when needed.
7. If a local review context exists, the UI may show review items such as a PR discovered through `gh` or a local/staged diff.

### Reviewing the AI generated work of others

1. Someone else lands agent-authored changes on a branch and opens a pull request (on GitHub or another host `gh` can talk to).
2. From a checkout of the repository, the reviewer runs `isoprism serve` (or opens a snapshot produced earlier with `isoprism diff <base> <head>`).
4. The CLI detects the repository root and builds or reuses a semantic graph for the PR’s base..head range.
5. When `gh` is installed and authenticated, the daemon discovers open pull requests for the current repository and exposes them as review items with title, body, author, base/head refs, diff stats, and a link back to GitHub. The reviewer does not need to run `gh pr checkout`; selecting a PR fetches `pull/<number>/head` into `refs/isoprism/pr/<number>/head` without touching the working tree. Without `gh`, the reviewer can still inspect the same changes by passing explicit refs to `isoprism diff` or using the manual compare controls in the local browser.
6. The reviewer selects that review item in the browser UI. The graph workspace loads a review graph centered on changed semantic components (functions, types, methods) rather than a flat file list.
7. The reviewer walks the graph: which components were added or modified, how they connect to callers and callees, and which tests moved with the change. They expand nodes and open source details when the card summary is not enough; they drop to the raw diff or GitHub only when they need line-level approval detail.
8. If the author (or their agent) ran `isoprism annotate`, those breadcrumbs appear alongside the graph so the reviewer can see stated intent, expected outcome, and per-node reasoning without inferring it from the diff alone.
9. When satisfied, the reviewer finishes review on GitHub—approve, request changes, or comment. Isoprism does not post review decisions; it is a local understanding layer in front of the normal PR workflow.

For teams that already use the hosted product, the same graph-first review experience exists at `https://isoprism.com` with webhook-driven indexing and optional cloud AI summaries. The CLI path is for reviewers who want that mental model inside a checkout, without granting repository access to a hosted indexer.

## Product Boundaries

The CLI should not require:

- a Supabase session
- GitHub App installation
- Postgres
- production API access
- Next.js during normal installed use
- a fake cloud repository ID

The CLI may integrate with:

- local git
- `gh` CLI, when installed and authenticated
- local `.isoprism/` cache files
- static embedded frontend assets

## Shared Surface

The CLI shares these concepts with cloud:

- semantic graph nodes and edges
- programs/entrypoints
- graph expansion
- node code retrieval
- optional review items
- common React graph and repo workspace components

The CLI does not need to share cloud route names, auth behavior, database identifiers, webhook behavior, or PR queue ranking.
