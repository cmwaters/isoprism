# AI Usage

Last updated on 2026-05-11.

This document describes the target AI contract for Isoprism. AI should be used only for pull request analysis: explaining changed code and producing a PR-level summary plus numeric risk score. Base repository code node summaries are intentionally out of scope.

The frontend must never call a model provider directly. AI calls belong in the Go API, and the API should persist structured outputs for the web app to display.

## Target Provider

| Area | Target implementation |
|---|---|
| Provider | Google Gemini API |
| Model | `gemini-2.5-flash` |
| Task | PR change summaries and PR-level analysis |
| Output mode | JSON object |
| Code entrypoint | `api/internal/ai/enricher.go` |
| Runtime config | `GEMINI_API_KEY` |

Gemini 2.5 Flash is the default target because the PR analysis job is high-volume, latency-sensitive, and needs reliable structured output over code diffs. Do not introduce separate models for base repository summaries.

## Where AI Is Used

### PR Change Summaries and PR-Level Analysis

**Function:** `Enricher.EnrichPRChanges`

**Triggered by:**

- `OpenPR`, after changed functions have been detected and component-scoped diffs have been built
- GitHub `pull_request` webhook actions: `opened`, `synchronize`, `reopened`, `ready_for_review`
- Development `POST /debug/prs/{prID}/reprocess`
- Development `POST /debug/prs/{prID}/reprocess/ai`

**Purpose:** Explain what changed inside each added or modified production node, capture relevant non-code changes, summarize what tests assert, then produce one PR-level summary and a numeric risk score for the queue.

**Context sent to the model:**

- PR title
- PR description/body, preserving author-provided markdown when available
- changed node `full_name`
- component-scoped `diff_hunk`
- additional changed files that are not represented as production code nodes, such as documentation, config, migrations, generated API contracts, dependency manifests, or build/deployment files
- test changes, passed as a separate context group
- deleted nodes are persisted as deleted changes but are not sent to the model

**Prompt template:**

```text
You are analysing a pull request. For each changed production function, write up to 2 sentences describing what specifically changed in this PR. Do not describe what the function generally does unless that is necessary to explain the change.

For tests, describe what behavior is being asserted. Do not summarize test implementation mechanics unless they are important to the assertion.

Use documentation, config, migrations, generated contracts, dependency manifests, build files, or deployment files as context for the PR summary and risk score. Do not produce per-file summaries for these other changes.

Use the PR title and description as intent/context, but ground your output in the diffs. If the PR description conflicts with the diff, trust the diff.

Return only a JSON object with this shape:
{
  "changes": [{"full_name": "...", "change_summary": "..."}],
  "test_assertions": [{"name": "...", "assertion_summary": "..."}],
  "pr_summary": "two to three sentence describing the overall PR with an emphasis on what this changes and why this change is necessary",
  "risk_score": 5
}

risk_score is an integer from 1 to 10.

PR title:
{pr_title}

PR description:
{pr_description}

Changed functions and their diffs:

--- {full_name} ---
Diff:
{diff_hunk}

Test changes:

--- {test_name_or_path} ---
Diff:
{test_diff_hunk}

Other changed files:

--- {path} ---
Diff:
{file_diff_hunk}
```

The changed function block is repeated for each added or modified production node in the PR. Test and other-file blocks are repeated for each relevant changed file or test node.

**Expected output:**

```json
{
  "changes": [
    {
      "full_name": "package.Type.Method",
      "change_summary": "Up to two sentences describing what changed in this PR."
    }
  ],
  "test_assertions": [
    {
      "name": "package.TestName",
      "assertion_summary": "Up to two sentences describing the behavior this test asserts."
    }
  ],
  "pr_summary": "Up to three sentences describing the overall PR.",
  "risk_score": 5
}
```

**Stored in:**

- `pr_node_changes.change_summary`
- test assertion summaries in a dedicated PR-test-change field/table, or folded into PR analysis until a dedicated schema exists
- `pr_analyses.summary`
- `pr_analyses.risk_score`
- `pr_analyses.ai_model`
- `pr_analyses.analysis_payload`
- `pr_analyses.prompt_version`
- `pr_analyses.generated_at`

**Displayed in:**

- PR summary panel as the overview description
- selected node detail panel for changed nodes
- test assertions section when tests changed

**Failure behavior:** If the model call fails, `OpenPR` should log the error and continue. If JSON parsing fails, node change summaries should be empty and the PR summary should default to `PR analysis unavailable.`.

## Model Support Policy

The product should keep model selection simple. The only default AI task is PR analysis.

| Task | Default model | Fallback model | Why |
|---|---|---|---|
| PR change summaries | `gemini-2.5-flash` | none by default | This is the core AI task: it interprets diffs and feeds the reviewer-facing graph. Keep output consistent before adding fallback complexity. |
| Test assertion summaries | `gemini-2.5-flash` | same model used for PR change summaries | Keep test changes in the same PR-level call so the model can compare asserted behavior against production changes without treating tests as production logic. |
| PR-level summary and risk score | `gemini-2.5-flash` | same model used for PR change summaries | Generate these fields in the same call as change summaries so the model sees the same diff context. |
| Base code node summaries | not implemented | not implemented | Isoprism should focus AI spend and UI attention on PR changes, not static summaries of the default branch. |
| Embeddings or semantic search | not implemented | not implemented | Add a dedicated embedding model only when the product has a search/retrieval feature that needs it. |
| Chat or review agent | not implemented | not implemented | Do not introduce conversational models until there is an explicit user-facing workflow for asking questions about a graph or PR. |

## Trigger Matrix

| Trigger | Event | AI calls made | Notes |
|---|---|---|---|
| User selects a repo and starts indexing | `RepoInit` | none | Repository indexing should build the structural graph without asking AI to summarize base code nodes. |
| Developer reindexes a repo | `RepoInit` | none | `POST /debug/repos/{repoID}/reindex` should rebuild graph data without AI base summaries. |
| GitHub PR opened | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.opened`. |
| GitHub PR updated with new commits | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.synchronize`; old `pr_node_changes` are cleared before reprocessing. |
| GitHub PR reopened | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.reopened`. |
| GitHub PR marked ready for review | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.ready_for_review`. |
| Developer fully reprocesses a PR | `OpenPR` | `EnrichPRChanges` | Called by `POST /debug/prs/{prID}/reprocess`; rebuilds graph data first, then reruns AI. |
| Developer reprocesses only PR graph data | `ReprocessPRGraph` | none | Called by `POST /debug/prs/{prID}/reprocess/graph`; rebuilds `pr_node_changes`, PR call edges, changed test nodes, and processing metadata while preserving existing AI summaries for unchanged node IDs. Use full `/debug/prs/{prID}/reprocess` when graph data and AI output should both be refreshed. |
| Developer reprocesses only PR AI output | `ReprocessPRAI` | `EnrichPRChanges` | Called by `POST /debug/prs/{prID}/reprocess/ai`; requires existing `pr_node_changes` and updates only AI summaries, PR summary, risk score, model, payload, and prompt version. |
| GitHub PR merged | `MergePR` | none | Advances repository state; no AI call. |

## Data Boundaries

AI calls may send PR titles, PR descriptions, PR diff hunks, and changed function names to Gemini. The target implementation should not send:

- user emails
- Supabase auth tokens
- GitHub installation tokens
- database credentials
- entire repositories as a single payload
- base branch code nodes for standalone summaries
- test code as code nodes unless PR diff support explicitly includes test nodes

Diffs can still contain secrets if the indexed repository contains secrets in source code. Isoprism should treat AI enrichment as a code-processing feature and disclose that code excerpts are sent to the configured model provider.

## Context Selection Policy

Prefer high-signal PR context over large raw tree context. The default model input should include:

- PR title and description
- changed node names
- component-scoped diff hunks
- change status for each node, such as added, modified, deleted, or renamed when available
- file paths for changed nodes
- test diffs grouped separately from production code diffs
- non-code changed files that affect user behavior, operations, schema, dependencies, deployment, or documentation
- directly connected callers/callees from the existing graph when they help explain likely impact

Avoid sending large parts of the tree by default. Full files, broad directory listings, or unchanged base-branch function bodies usually add cost and noise without improving PR summaries. Add larger context only behind a deliberate retrieval step, such as when a changed function has many indirect dependents, the diff references symbols outside the hunk, or the model needs a nearby caller/callee body to explain impact.

A good first expansion is one-hop graph context: changed node, file path, direct callers, direct callees, and whether each connected node is production or test code. If that is not enough, add only the specific adjacent function bodies needed to understand the diff.

Keep tests in the same PR analysis call, but separate them in the prompt and output schema. Splitting tests into a separate model call loses useful context about whether the tests cover the production changes. Mixing tests into the production `changes` array is also noisy because test summaries should answer a different question: what behavior is being asserted?

## Implementation Alignment

The implementation follows this PR-only contract:

1. Gemini 2.5 Flash is the only active model/provider path.
2. `RepoInit` builds structural graph data only; it does not call AI.
3. `risk_label` is not part of the model prompt or expected output.
4. Any UI risk label is derived from `risk_score` in application code.
5. `pr_analyses.ai_model` records `gemini-2.5-flash`.
6. `Enricher.EnrichPRChanges` receives PR title and PR description/body.
7. Test assertions and other changed files are separate prompt sections, while only test assertions are parsed as dedicated structured output.
8. Anthropic and OpenAI provider config are not active AI call paths.

## Implementation Notes

- Model responses must be JSON objects matching the PR analysis schema above.
- Add strict schema validation before persisting model output.
- Add retry/backoff for transient provider errors.
- Add per-call logging for task type, model, changed node count, latency, and parse success without logging source code.
- `OpenPR` should count only non-deleted nodes in `pr_analyses.nodes_changed`.
