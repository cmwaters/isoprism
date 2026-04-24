# Aperture — Product Design Brief

## 1. Product Overview

**Aperture** is a visual interface for understanding how software evolves.

It helps teams see:

- what is changing
- where attention is needed
- how work flows through pull requests
- how changes impact the system

Aperture does not replace GitHub, Linear, or other tools.

Instead, it provides a **new lens** on top of them:

> GitHub shows what changed. Aperture shows what it means.

---

## 2. Core Problem

Modern software development — especially with AI — produces:

- more pull requests
- larger and more complex changes
- faster iteration cycles
- increased cognitive load during reviews

The bottleneck is no longer writing code.

The bottleneck is:

- understanding changes
- reviewing efficiently
- coordinating across the system

Existing tools show:

- events (notifications)
- artifacts (PRs, commits)

But they do not show:

- system state
- flow health
- change meaning

Aperture solves this by making the system **visible and interpretable**.

---

## 3. Target Users

### Primary Users: Engineers

Engineers use Aperture to:

- understand what is happening across PRs
- quickly assess changes before reviewing
- identify risky or complex work
- reduce cognitive load when reviewing code

---

### Secondary Users: Team Leads / Engineering Managers

They use Aperture to:

- understand workflow health
- identify bottlenecks
- detect systemic issues (not individual performance)
- guide process improvements

---

## 4. Product Principles

### 1. Observability, not control

Aperture reveals what is happening.
It does not create, assign, or manage work.

---

### 2. System-first, not individual-first

Focus on:

- flow
- queues
- system behavior

Avoid:

- rankings
- performance scoring

---

### 3. Fast to read, not deep to analyze

The product should feel like:

- Activity Monitor
- Datadog
- a control panel

Users should understand the state in seconds.

---

### 4. Augment, don’t replace GitHub

Aperture sits alongside GitHub.

It helps users:

- decide what to look at
- understand what they are about to review

Code review happens within Aperture, with deep integration into GitHub for syncing changes, comments, and status.

---

### 5. Interpretation over presentation

Aperture does not just display data.

It interprets:

- what a change does
- where it impacts
- how risky it is

---

## 5. Core Product Surfaces

---

## Surface 1 — Activity View (Default)

### Purpose:

Show what is happening right now.

### Key Question:

> What needs attention?

---

### UI Sections:

#### A. Queue

A prioritized list of PRs answering:

> What should I review or act on next?

Each item shows:

- title
- time waiting
- current state (needs review / needs author / stalled)
- size indicator (small / medium / large)
- risk indicator (low / medium / high)
- impacted areas (services/modules)

The list is ordered by urgency, combining:

- wait time
- risk
- system impact

---

## Surface 2 — PR Intelligence Panel

### Purpose:

Provide a structured environment for understanding, reviewing, discussing, and acting on a pull request.

### Entry:

- from Activity View
- or via link from GitHub

---

### Sections:

#### A. Context

Provide a high-level understanding of the change.

Includes:

- plain-language summary of what the PR does
- explanation of why the change exists (if available)
- system impact (services/modules affected)
- key areas or files involved
- cross-boundary changes
- relevant context pulled from linked Linear, Jira, or GitHub issues

---

#### B. Changes

Allow users to explore the change at different levels of abstraction.

Default view:

- semantic overview of the change (grouped by intent or system area)

User can progressively narrow down to:

- file-level changes
- line-by-line diffs

---

#### C. Discussion

Centralized view of all comments and conversations.

---

#### D. Actions

Includes:

- approve
- comment
- request changes

---

## Surface 3 — Insights View (Team-Level)

### Purpose:

Reveal patterns over time.

---

### Example Insight Cards:

- “Review time increased from 6h → 12h this week”
- “Large PRs increased by 35%”
- “2 reviewers handled 70% of reviews”

---

## Surface 4 — Settings & Integrations

### Purpose:

Allow teams to configure Aperture, set up their environment, and connect external tools.

---

### Sections:

#### A. Team Setup

- create or join team
- invite members
- assign roles

---

#### B. Repository Setup

- connect GitHub
- select repositories

---

#### C. Integrations

Support:

- GitHub
- Linear
- Jira

---

#### D. Preferences

- PR size thresholds
- risk sensitivity
- stale definitions

---

#### E. Onboarding

Guided setup flow for first-time users.

---

## 8. Design Language

### Visual Style:

The aesthetic should be inspired by **Typeform**.

- lots of whitespace
- focused screens
- minimal UI
- smooth transitions

---

### Tone:

- calm
- minimal
- precise

---

### Avoid:

- clutter
- dashboards with too many charts
- gamification

---

## Final Product Statement

Aperture is:

> A visual interface for understanding how software evolves.

Or:

> See what your code changes actually mean.

