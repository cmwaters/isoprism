# AI Usage

Last verified against the codebase on 2026-04-28.

Isoprism uses AI only in the Go API. The frontend never calls a model provider directly. AI output enriches the function graph with plain-language summaries, PR change summaries, and queue-level PR risk signals.

## Current Provider

| Area | Current implementation |
|---|---|
| Provider | Anthropic Messages API |
| Endpoint | `https://api.anthropic.com/v1/messages` |
| Model | `claude-sonnet-4-6` |
| API version header | `2023-06-01` |
| Max tokens per call | `4096` |
| Timeout | `120s` |
| Code entrypoint | `api/internal/ai/enricher.go` |
| Runtime config | `ANTHROPIC_API_KEY` |

The model is currently hard-coded in `api/internal/ai/enricher.go`. `OPENAI_API_KEY` exists in config but is not used by any current AI call site.

If `ANTHROPIC_API_KEY` is unset, AI enrichment becomes a no-op: the API still indexes repositories and PRs, but generated summaries and risk fields may be empty.

## Where AI Is Used

### 1. Base Code Node Summaries

**Function:** `Enricher.EnrichNodes`

**Triggered by:**

- `RepoInit`, after source files have been fetched, parsed, and inserted into `code_nodes`
- Authenticated `POST /api/v1/repos/{repoID}/index`
- Development `POST /debug/repos/{repoID}/reindex`

**Purpose:** Generate a concise summary for each production code node in the base repository graph.

**Context sent to the model:**

- `full_name`
- raw function/type body text
- nodes are batched in groups of 30 by `RepoInit`
- test code is excluded before AI enrichment

**Current prompt template:**

```text
For each function below, write exactly 2 plain-English sentences describing what the function does. Be specific and concise. Return a JSON array: [{"full_name": "...", "summary": "..."}]

Functions:

--- {full_name} ---
{body}
```

The `--- {full_name} ---` block is repeated for each node in the batch.

**Expected output:**

```json
[
  {
    "full_name": "package.Type.Method",
    "summary": "Exactly two plain-English sentences describing what the node does."
  }
]
```

**Stored in:** `code_nodes.summary`

**Displayed in:** graph node detail panels as the "what it does" style summary.

**Failure behavior:** If the Anthropic call fails, `RepoInit` logs the error and continues. If JSON parsing fails, the current implementation returns an empty summary map without failing the indexing job.

### 2. PR Change Summaries and PR-Level Analysis

**Function:** `Enricher.EnrichPRChanges`

**Triggered by:**

- `OpenPR`, after changed functions have been detected and component-scoped diffs have been built
- GitHub `pull_request` webhook actions: `opened`, `synchronize`, `reopened`, `ready_for_review`
- Development `POST /debug/prs/{prID}/reprocess`

**Purpose:** Explain what changed inside each added or modified production node, then produce one PR-level summary and a simple risk assessment for the queue.

**Context sent to the model:**

- changed node `full_name`
- component-scoped `diff_hunk`
- deleted nodes are persisted as deleted changes but are not sent to the model

**Current prompt template:**

```text
You are analysing a pull request. For each changed function, write exactly 2 sentences describing what specifically changed (not what the function does, but what changed in this PR). Return a JSON object:
{
  "changes": [{"full_name": "...", "change_summary": "..."}],
  "pr_summary": "one sentence describing the overall PR",
  "risk_score": 5,
  "risk_label": "medium"
}
risk_score is 1-10; risk_label is "low" (1-3), "medium" (4-6), or "high" (7-10).

Changed functions and their diffs:

--- {full_name} ---
Diff:
{diff_hunk}
```

The changed function block is repeated for each added or modified node in the PR.

**Expected output:**

```json
{
  "changes": [
    {
      "full_name": "package.Type.Method",
      "change_summary": "Exactly two sentences describing what changed in this PR."
    }
  ],
  "pr_summary": "One sentence describing the overall PR.",
  "risk_score": 5,
  "risk_label": "medium"
}
```

**Stored in:**

- `pr_node_changes.change_summary`
- `pr_analyses.summary`
- `pr_analyses.risk_score`
- `pr_analyses.risk_label`
- `pr_analyses.ai_model`
- `pr_analyses.generated_at`

**Displayed in:**

- PR queue summary rows
- PR summary panel
- selected node detail panel for changed nodes

**Failure behavior:** If the Anthropic call fails, `OpenPR` logs the error and continues. If JSON parsing fails, node change summaries are empty and the PR summary defaults to `PR analysis unavailable.` in memory; persisted fields may be null depending on the returned object.

## Model Support Policy

The product should support a small model registry instead of scattering model names through the code. Each AI job should declare its task type, default model, fallback model, and output schema.

| Task | Default model | Fallback model | Why |
|---|---|---|---|
| Base code node summaries | `claude-sonnet-4-6` | smaller Anthropic fast/low-cost model when available | Needs reliable code understanding, but each node summary is short and can tolerate a cheaper fallback if output stays structured. |
| PR change summaries | `claude-sonnet-4-6` | none by default | This is the highest-value AI task because it interprets diffs and feeds the reviewer-facing graph. Prefer quality and consistency over cost. |
| PR-level summary and risk | `claude-sonnet-4-6` | same model used for PR change summaries | The current implementation generates these fields in the same call as change summaries so the model sees the same diff context. |
| Embeddings or semantic search | not currently implemented | not currently implemented | Add a dedicated embedding model only when the product has a search/retrieval feature that needs it. |
| Chat or review agent | not currently implemented | not currently implemented | Do not introduce conversational models until there is an explicit user-facing workflow for asking questions about a graph or PR. |

Current code supports only the first three rows, all through Anthropic. OpenAI is configured but unused.

## Trigger Matrix

| Trigger | Event | AI calls made | Notes |
|---|---|---|---|
| User selects a repo and starts indexing | `RepoInit` | `EnrichNodes` | Called by `POST /api/v1/repos/{repoID}/index`; summaries are generated after parsing and graph edge extraction. |
| Developer reindexes a repo | `RepoInit` | `EnrichNodes` | Called by `POST /debug/repos/{repoID}/reindex`; same AI path as normal indexing. |
| GitHub PR opened | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.opened`. |
| GitHub PR updated with new commits | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.synchronize`; old `pr_node_changes` are cleared before reprocessing. |
| GitHub PR reopened | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.reopened`. |
| GitHub PR marked ready for review | `OpenPR` | `EnrichPRChanges` | Triggered by `pull_request.ready_for_review`. |
| Developer reprocesses a PR | `OpenPR` | `EnrichPRChanges` | Called by `POST /debug/prs/{prID}/reprocess`; same AI path as normal PR processing. |
| GitHub PR merged | `MergePR` | none | Advances `repositories.main_commit_sha`; no AI call in the current implementation. |

## Data Boundaries

AI calls may send repository source snippets and PR diff hunks to Anthropic. The current implementation does not send:

- user emails
- Supabase auth tokens
- GitHub installation tokens
- database credentials
- entire repositories as a single payload
- test code as code nodes

The source snippets and diffs can still contain secrets if the indexed repository contains secrets in source code. Isoprism should treat AI enrichment as a code-processing feature and disclose that code excerpts are sent to the configured model provider.

## Implementation Notes

- Model responses are expected to be JSON, but the implementation accepts text containing a JSON object or array and extracts the first JSON block.
- There is no schema validation beyond `json.Unmarshal`.
- There is no retry logic, streaming, tool use, cache table, prompt version, or per-call telemetry yet.
- `pr_analyses.ai_model` records the hard-coded model name for PR analysis. Base node summaries do not currently store the model that generated `code_nodes.summary`.
- `OpenPR` counts only non-deleted nodes in `pr_analyses.nodes_changed`.

## Recommended Next Changes

1. Move the model name into config, with a typed task registry in the AI package.
2. Store `ai_model` and a prompt version for `code_nodes.summary`.
3. Add strict JSON schema validation and fail closed for malformed model output.
4. Add retry/backoff for transient Anthropic errors.
5. Add per-call logging for task type, model, input count, latency, and parse success without logging source code.
6. Remove `OPENAI_API_KEY` from config until used, or document and implement the OpenAI task it supports.
