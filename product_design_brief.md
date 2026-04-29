# Isoprism — Product Design Brief

> Validation Prototype | Updated: 2026-04-29

---

## 1. What We Are Validating

**Hypothesis:** A graph representation of software changes — showing the functions affected, their inputs and outputs, and what changed about each — is faster and more effective than reading a code diff when trying to understand a pull request.

This prototype exists to test that hypothesis with invited beta testers, real repos, and real PR review work. Nothing else.

### Beta Tester Contract

The prototype loop is invite-only.

Each beta tester receives a unique access link containing an invitation token. That token grants permission to start the Isoprism beta flow. The tester then connects GitHub, installs or authorizes the Isoprism GitHub App, selects exactly one repository, and uses Isoprism for one week while reviewing PRs in that repository.

During the trial week, the tester should be prompted to request features and report bugs whenever they hit friction. At the end of the week, Isoprism should ask them to complete a short questionnaire about whether the graph view helped them review PRs faster and with more confidence.

---

## 2. The Problem

Code review is bottlenecked on comprehension, not tooling.

When a developer opens a PR, they are handed a raw diff: lines added, lines removed. To understand what the change *means*, they must mentally reconstruct:

- which functions were touched
- what those functions do
- how they relate to each other
- what actually changed in their behaviour

This reconstruction is slow, error-prone, and scales poorly with PR size or codebase complexity.

**The insight:** the diff is the right *source of truth* but the wrong *interface for understanding*. What engineers need is a semantic view — the functions affected, the call paths they sit in, and plain-language explanations of what changed and why.

---

## 3. The Prototype Flow

The prototype has one narrow invited-tester flow. Every design and engineering decision should serve this flow.

### Entry — Invite Link

The tester starts from a link containing a unique token, for example:

```text
https://isoprism.com/beta/{token}
```

The token must be valid, unused or active, and tied to one tester record. Invalid, expired, or already-completed tokens should show a clear access state and should not allow GitHub connection.

---

### Screen 1 — Login

The tester lands on the site through their invite link. The page sets expectations before GitHub connection: Isoprism is a prototype, not a fully fledged product; the beta exists to answer whether there is a better way to understand and review code changes.

The page explains that beta testers should use the prototype where possible while reviewing PRs, connect GitHub, select a single repository, submit feature requests and bug reports through the product footer, trial it for one week, and complete a short questionnaire at the end. No email, no password, no form. One button: "Connect GitHub".

Direct visits to `isoprism.com` should not start the beta unless there is an active invite token associated with the session or account. If the GitHub OAuth callback shows the signed-in tester has not connected Isoprism yet, the tester is sent to install the GitHub App and grant repository permissions.

---

### Screen 2 — Repo Selection

After login and GitHub App permission, the tester sees the repositories available through the GitHub App installation. They select **one** repository for the beta trial. That's it.

The product should make the one-repo rule explicit: the selected repository is the repository Isoprism will use for the one-week trial. Changing repositories is out of scope unless done manually by the operator during beta support.

The backend begins indexing that repo — tracking function-level changes on the `main` branch.

---

### Screen 3 — Indexing State

After the tester selects one repository, Isoprism indexes that repository and shows progress until the graph is ready or indexing fails.

---

### Screen 4 — PR Queue

Once indexing is complete, the user sees a list of the **top five open pull requests targeting `main`** for that repo, ranked by urgency (a combination of wait time, change size, and risk). During beta, Isoprism hides PRs whose base branch is not `main` or whose base SHA does not match the currently indexed main graph.

PR graphs show production code only. Tests remain indexed as evidence for the affected production nodes, but they appear in node details rather than as separate graph cards.

Each PR in the list shows:
- PR title
- PR number
- Time open
- Number of functions affected
- A one-line summary of what the PR does

The user views a repository at the same path GitHub uses (`/{owner}/{repo}`), with the PR list and repository graph on one page. Clicking a PR keeps the user on that same URL, swaps the graph in place, and adds PR-specific change summaries to the same review interface.

---

### Screen 5 — Graph Workspace

This is the core of the prototype.

The screen shows an **interactive graph** where each node is a **function** in the repository or in the call path affected by the selected PR. The graph reveals not just what files changed, but which functions changed, what those functions do, and how they connect. Repo browsing and PR review happen in the same mounted workspace at `/{owner}/{repo}`.

**Each node contains:**
- Function name
- Function full name, inputs, and outputs, with linked types when the type is present in the graph
- A two-sentence summary of what the function does
- A two-sentence summary of what changed in this function (or "unchanged — called by a changed function" if it was not directly modified)

**Nodes are visually distinguished by type:**
- **Directly changed** — functions that were modified in the PR (primary colour, full opacity)
- **Callers** — functions that call a changed function (secondary colour, slightly muted)
- **Callees** — functions called by a changed function (secondary colour, slightly muted)

**Edges** represent call relationships: an edge from A → B means A calls B.

The user can:
- Pan and zoom the graph freely
- Click any node to expand it into a focused detail panel showing the full function body diff alongside the summary
- Navigate between nodes using keyboard arrows or by clicking edges
- Switch between the whole-system repo graph and PR-specific diff graph without changing pages
- Submit a feature request or bug report from the authenticated product shell during the trial week

The repo and PR graph views include a black footer banner:

```text
This is a beta version of Isoprism. Report a problem - Request a feature.
```

Both actions open a centered feedback panel and submit a GitHub issue labelled `bug` or `feature`. The issue should reference the tester's unique beta ID and include repository, PR, node, browser path, app commit, and source commit context.

---

### Screen 6 — End-of-Week Questionnaire

At the end of the seven-day trial window, Isoprism should ask the tester to complete a short questionnaire before or alongside continued use.

The questionnaire should capture:

- Whether Isoprism helped them understand PRs faster
- Whether the graph made review risk clearer
- Which PR review moments felt confusing or missing
- Bugs encountered
- Feature requests
- Whether they would keep using Isoprism for PR review

---

### Admin — Beta Tester Console

Operators need a simple admin page for managing the beta loop.

The admin page should allow an operator to enter a beta tester by name, generate a unique beta ID, generate a token and invite link, and monitor whether the invite has been used. It should also show which repository the tester has set up and their questionnaire answers once submitted.

Raw invite tokens should only be shown when generated. After that, the admin page should show the beta ID, invite status, and link state, not the raw token.

---

## 4. What Success Looks Like

A user should be able to:

1. Open a PR they have never seen before
2. Understand which functions changed and how they relate to each other
3. Form a mental model of the change's intent and risk

— in under **two minutes**, without reading a single line of raw diff.

That is the core product metric. The beta loop also succeeds when the tester completes the one-week trial, reports bugs or feature requests as they arise, and submits the end-of-week questionnaire.

---

## 5. What This Prototype Is Not

- Not a code review tool. Users cannot comment, approve, or request changes.
- Not a team tool. There are no orgs, members, or permissions in the prototype.
- Not an analytics product. There are no insights, charts, or trend views.
- Not multi-repo. One repo per beta tester.
- Not open signup. Access requires a valid beta invite token.

These features may follow if the hypothesis proves true. For now, they are out of scope.

---

## 6. Design Principles

### Clarity over completeness

Show the most important functions, not all of them. If a PR touches 40 functions, show the 10–15 most central. A focused graph is more useful than a complete one.

### Text as a first-class UI element

The summaries on each node are the product. They should be readable at a glance, written in plain English, and specific enough to be useful. Avoid vague AI boilerplate.

### Zero learning curve

The graph metaphor should be immediately intuitive. Nodes are functions. Edges are calls. Changed nodes are highlighted. No legend required.

### Calm and precise

The aesthetic should feel like a read-only instrument panel: high contrast where it matters, quiet everywhere else. No animations, no gradients, no noise.
