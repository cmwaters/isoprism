---
name: isoprism
description: Use Isoprism for semantic code-review workflows, including generating local semantic diffs with `isoprism diff`, browsing a repository graph with `isoprism serve`, reviewing PRs through changed nodes, tests, docs, and other files, writing Isoprism annotations, and working on the Isoprism source repo while preserving its current docs, graph, CLI, AI, and deployment contracts.
---

# Isoprism

Use this skill when a task should be grounded in Isoprism's semantic graph instead of only raw file diffs.

Isoprism turns repository changes into a function/type-level graph. The usual agent job is to generate the graph, inspect changed production nodes and their direct semantic context, account for tests and non-code files, and leave concise annotations or findings that help a human reviewer.

## First Checks

1. Confirm the current directory is a git repository.
2. Prefer an installed `isoprism` binary when available.
3. If working inside the Isoprism source checkout and no binary is installed, use the source CLI from `api/`:

```bash
go run ./cmd/isoprism diff
go run ./cmd/isoprism serve --port 3717
```

4. If the user wants browser inspection, generate or open the HTML viewer only when the environment permits it. In headless or restricted environments, use `--json` or `--no-open` and summarize the graph findings.
5. Treat missing optional tools such as `gh`, Linear, Jira, Entire checkpoints, or network access as non-fatal unless the user specifically asked for that integration.

## Choose The Workflow

### Pre-Push Review

Use when the user asks to review their branch, staged changes, current work, or pre-push state.

```bash
isoprism diff --output /tmp/isoprism-diff.html
isoprism diff staged --output /tmp/isoprism-staged.html --no-open
isoprism diff <base> <head> --json
```

Review the result in this order:

1. Changed production nodes and their `change_type`.
2. Direct callers, callees, `owns_method`, and `uses_type` context.
3. Changed tests and what behavior they assert.
4. Documentation, config, migrations, generated files, dependency files, and deployment files.
5. Any annotations or PR/issue context already present.

Do not reduce the review to "files changed." Isoprism's value is the semantic relationship between changed nodes and the system around them.

### PR Review

Use when reviewing an existing pull request or comparing two refs.

```bash
isoprism diff <base-ref> <head-ref> --output /tmp/isoprism-pr.html --no-open
isoprism diff <base-ref> <head-ref> --json
```

If the checked-out branch already represents the PR head and the default branch is correct, `isoprism diff` is acceptable. Prefer explicit refs when there is ambiguity.

When writing findings, anchor them to semantic nodes where possible: function, method, type, test entrypoint, or file patch. Include raw file/line references only when the graph output is insufficient or the issue belongs to docs/config/other files.

### Local Repository Browser

Use when the user wants to explore a codebase, understand entrypoints, or browse the graph.

```bash
isoprism serve
isoprism serve --port 3717
isoprism serve --no-web
```

The default local viewer is `http://127.0.0.1:3717/local`. It should work in any git repo without Node.js or a checkout of the Isoprism source. Use `--web-dir` or `ISOPRISM_WEB_DIR` only for Isoprism frontend development.

### Annotation Workflow

Before writing annotations, check whether Entire checkpoints exist:

```bash
git show-ref --verify --quiet refs/remotes/origin/entire/checkpoints/v1
git show-ref --verify --quiet refs/heads/entire/checkpoints/v1
```

If Entire checkpoints exist and contain adequate reasoning, prefer surfacing that context instead of duplicating it. If Entire is absent or insufficient, write focused Isoprism annotations:

```bash
isoprism annotate diff --reason-for-change "..." --expected-outcome "..."
isoprism annotate node <node-sha256> --description "..." --reasoning "..." --confidence high
isoprism annotate test --node <node-sha256> --description "..."
```

Keep annotations factual and review-oriented. Describe what changed, why it matters, expected outcome, risks, gaps, and test assertions. Do not store secrets or credentials in annotations.

## Working On Isoprism Itself

If the user is modifying the Isoprism repo, read the relevant repo docs before editing. The docs are meant to be source of truth, not decorative wallpaper.

Core docs:

- `AGENTS.md` and `CLAUDE.md` for workflow, deployment, debug endpoints, and DB access.
- `cli.md` for local-first CLI behavior, cache layout, annotations, local viewer, and current implementation gaps.
- `graph.md` for graph selection, expansion, PR overview grouping, tests, type contents, and semantic edge behavior.
- `ai.md` for Gemini PR-only analysis and output schema.
- `engineering_design_doc.md`, `product_design_brief.md`, `settings.md`, `ui_brief.md`, and `web/README.md` for product and implementation contracts.

Project rules to preserve:

- Develop on `main` for now; do not create feature branches or PRs unless the user explicitly changes the workflow.
- Run local web from `web/` with `NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev`.
- Keep docs updated with code changes.
- For API changes, run local Go checks when possible and remember tree-sitter requires CGO.
- For migrations, update `api/internal/db.RequiredMigrationVersion` with the migration.
- Use debug reprocess endpoints narrowly: graph-only for graph repair, AI-only for AI repair, full reprocess only when both should refresh.

## References

Load only the reference needed for the task:

- `references/project-reference.md`: Current Isoprism product, CLI, graph, AI, API, local viewer, and repo maintenance contracts.
