# Isoprism — Product Design Brief

> Validation Prototype | Updated: 2026-04-21

---

## 1. What We Are Validating

**Hypothesis:** A graph representation of software changes — showing the functions affected, their signatures, and what changed about each — is faster and more effective than reading a code diff when trying to understand a pull request.

This prototype exists to test that hypothesis with real users and real repos. Nothing else.

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

The prototype has exactly three screens. Every design and engineering decision should serve this flow.

---

### Screen 1 — Login

The user lands on the site and signs in with GitHub. No email, no password, no form. One button.

---

### Screen 2 — Repo Selection

After login, the user sees a list of their GitHub repositories. They select **one**. That's it.

The backend begins indexing that repo — tracking function-level changes on the `main` branch.

---

### Screen 3 — PR Queue

Once indexing is complete, the user sees a list of the **top five open pull requests** for that repo, ranked by urgency (a combination of wait time, change size, and risk).

Each PR in the list shows:
- PR title
- PR number
- Time open
- Number of functions affected
- A one-line summary of what the PR does

The user clicks a PR to enter the graph view.

---

### Screen 4 — PR Graph View

This is the core of the prototype.

The screen shows an **interactive graph** where each node is a **function** in the call path affected by the PR. The graph reveals not just what files changed, but which functions changed, what those functions do, and how they connect.

**Each node contains:**
- Function name
- Function signature (the full signature: name, parameters, return type)
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
- Return to the queue from a breadcrumb at the top

---

## 4. What Success Looks Like

A user should be able to:

1. Open a PR they have never seen before
2. Understand which functions changed and how they relate to each other
3. Form a mental model of the change's intent and risk

— in under **two minutes**, without reading a single line of raw diff.

That is the metric. Everything else is a means to that end.

---

## 5. What This Prototype Is Not

- Not a code review tool. Users cannot comment, approve, or request changes.
- Not a team tool. There are no orgs, members, or permissions in the prototype.
- Not an analytics product. There are no insights, charts, or trend views.
- Not multi-repo. One repo per session.

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
