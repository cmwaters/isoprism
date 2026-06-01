# Cloud AI

## Purpose

AI is used only for cloud pull request analysis. The frontend never calls a model provider directly. The Go API calls the provider, validates structured output, and persists results for the UI.

Base repository summaries are intentionally out of scope.

## Provider

| Area | Target |
|---|---|
| Provider | Google Gemini API |
| Model | `gemini-2.5-flash` |
| Entrypoint | `api/internal/ai/enricher.go` |
| Runtime config | `GEMINI_API_KEY` |
| Output | JSON object |

## Tasks

`Enricher.EnrichPRChanges` produces:

- per-changed-production-node summaries
- test assertion summaries
- PR-level summary
- numeric `risk_score` from 1 to 10

It is triggered by:

- GitHub PR opened
- GitHub PR synchronized
- GitHub PR reopened
- GitHub PR marked ready for review
- `POST /debug/prs/{prID}/reprocess`
- `POST /debug/prs/{prID}/reprocess/ai`

Repository indexing (`RepoInit`) does not call AI.

## Model Context

The prompt may include:

- PR title
- PR body/description
- changed node names
- file paths
- component-scoped diff hunks
- change type
- test diffs
- non-code changed files that affect behavior, operations, schemas, dependencies, deployment, or docs
- small graph-adjacent context when needed

Avoid sending broad unchanged repository context by default.

## Output Shape

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
  "pr_summary": "Two to three sentences describing the overall PR.",
  "risk_score": 5
}
```

## Persistence

AI output is stored in:

- `pr_node_changes.change_summary`
- `pr_analyses.summary`
- `pr_analyses.risk_score`
- `pr_analyses.ai_model`
- `pr_analyses.analysis_payload`
- `pr_analyses.prompt_version`
- `pr_analyses.generated_at`

## Failure Behavior

If the model call fails, PR processing should continue where possible and record the failure. If JSON parsing fails, node summaries may be empty and the PR summary should default to `PR analysis unavailable.`

## Boundaries

Do not send:

- Supabase auth tokens
- GitHub installation tokens
- database credentials
- user emails
- entire repositories as one payload
- static base repository summaries

Diffs can contain secrets if the source repo contains secrets. Treat AI enrichment as code processing and disclose that code excerpts are sent to the configured model provider.
