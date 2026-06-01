# Cloud Overview

## Purpose

The cloud product is the hosted GitHub App experience at `https://isoprism.com`. It validates whether a semantic graph representation of pull requests helps developers understand code changes faster than reading raw diffs.

Unlike the CLI, cloud owns identity, GitHub authorization, repository indexing, pull request processing, webhooks, pilot administration, feedback, deployment, and AI enrichment.

## Product

The current cloud product is an invite-only pilot:

1. A prospective user registers at `/pilot/register`.
2. An admin reviews the registration and sends an invite link.
3. The user signs in with GitHub.
4. The user installs or authorizes the Isoprism GitHub App.
5. The user selects a repository.
6. The API indexes the default branch and processes matching open PRs.
7. The user reviews PRs in the graph workspace for one week.
8. The user reports bugs/features through the product.
9. The user completes an end-of-pilot review form.

## Core Hypothesis

A graph representation of software changes showing affected functions, types, call relationships, tests, and summaries is faster and more effective than reading a raw code diff when trying to understand a pull request.

## Cloud User Journey

### Registration

Users register at:

```text
/pilot/register
```

Registration collects software experience, code review workflow, AI review usage, pain points, languages, and optional public repository details.

### Invite And Login

Admins send selected users an invite link. Invited users connect GitHub and install or authorize the GitHub App.

### Repository Selection

The user chooses one repository as the current review workspace. Pilot users can have only one active indexed repository; selecting another schedules the old indexed data for cleanup.

### Indexing

The backend indexes the repository default branch and reports status through:

```text
GET /api/v1/repos/{repoID}/status
```

### Review Workspace

The repository page shows repository programs and a PR review list. Selecting a program loads a repo graph. Selecting a PR loads a PR graph in the same mounted graph workspace.

During beta, the PR queue includes open, non-draft PRs targeting the repository default branch whose base SHA matches the indexed default branch SHA.

### Feedback And Review

Authenticated product views can submit bug reports and feature requests. At the end of the trial, the user completes the review questionnaire at `/pilot/review/{token}`.

## Out Of Scope

- open signup
- teams and organizations
- billing
- code review comments or approvals
- analytics dashboards
- multi-repo pilot workspaces
- direct frontend model-provider calls
