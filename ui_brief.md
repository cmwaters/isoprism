# Isoprism — UI Brief

> For AI generation and Figma design | Updated: 2026-04-23

---

## Design Language

**Typeface:** Inter (or equivalent geometric sans-serif). All body text 14–15px. Headings 20–28px. Monospace (JetBrains Mono or similar) for function signatures and code.

**Palette:**
- Background: `#EBE9E9` (warm off-white)
- Surface: `#E1E1E1` (sidebars, panels)
- Surface raised: `#D8D8D8` (selected states)
- Border: `#D4D4D4`
- Text primary: `#111111`
- Text secondary: `#666666`
- Text muted: `#888888`
- Text faint: `#AAAAAA`
- Accent (changed nodes / CTA): `#6366F1` (indigo-500)
- Accent dim (caller/callee nodes): `#C7D2FE` (indigo-200, border) with `#6366F1` (text)
- Success: `#22C55E`
- Destructive: `#EF4444`

**Aesthetic:** Light, minimal, instrument-like. High contrast between text and background. No gradients. No shadows except subtle `box-shadow: 0 1px 4px rgba(0,0,0,0.08)` for card borders.

**Spacing system:** 4px base unit. Common values: 8, 12, 16, 20, 24, 32, 48px.

**Motion:** Subtle only. 150ms ease-out for state transitions. No entrance animations.

---

## Screen 1 — Login

**Layout:** Full viewport. Vertically and horizontally centred content column, max-width 360px.

**Content (top to bottom):**
1. Isoprism logo mark — a small abstract graph icon (3 nodes connected by 2 edges), 32×32px, `#111111`
2. Product name "Isoprism" in 20px semibold, `#111111`, 12px below the logo
3. 48px gap
4. Headline: "Understand what your PRs actually change." — 28px, semibold, `#111111`, centered, max 2 lines
5. Subheading: "A graph view of every function affected. Plain-language summaries. No diffs." — 15px, `#666666`, centered, 12px below headline
6. 40px gap
7. **GitHub sign-in button** — full width, 48px height, `#111111` background, white text, 8px border-radius. GitHub Octocat icon (20px) left of text "Continue with GitHub". On hover: `#333333` background.
8. 48px gap below button
9. Fine print: "By signing in you authorise read access to your repositories." — 12px, `#888888`, centered

**Background:** Solid `#EBE9E9`. No image, no pattern.

---

## Screen 2 — Repo Selection

**Layout:** Full viewport. Two zones:
- Left sidebar (240px wide, full height, `#E1E1E1` background, `#D4D4D4` right border)
- Main content area (remaining width, `#EBE9E9` background)

**Left Sidebar:**
- Isoprism logo mark + "Isoprism" wordmark at top, 20px padding

**Main Content (centred column, max-width 560px, vertically centred in viewport):**
1. Heading: "Select a repository" — 24px semibold, `#111111`
2. Subheading: "Isoprism will index this repository's pull requests." — 15px, `#666666`, 8px below heading
3. 24px gap
4. **Search input** — full width, 44px height, white background, `#D4D4D4` border, 6px border-radius. Placeholder: "Search repositories…" in `#AAAAAA`. Magnifier icon on left inside.
5. 16px gap
6. **Repository list** — scrollable, max-height ~400px. Each row:
   - Height: 56px
   - White background, `#D4D4D4` border, 6px border-radius, 1px gap between rows
   - Left: repo name in 14px semibold `#111111` + org/owner prefix in `#888888` 13px
   - Right: branch badge (e.g. "main") in `#EBE9E9` with `#D4D4D4` border, 11px `#888888` text
   - Selected state: `#D8D8D8` background, `#6366F1` left border (3px), repo name in `#6366F1`
   - Hover state: `#F0F0F0` background
7. 24px gap
8. **Continue button** — right-aligned, 180px wide, 44px height, `#6366F1` background, white text "Index repository →", 6px border-radius. Disabled (opacity 0.4) until a repo is selected.

---

## Screen 3 — Indexing State (transient)

**Layout:** Same two-zone layout as Screen 2. Main content centred column, max-width 480px.

**Content:**
1. Repo name + GitHub icon in small badge at top: `acme/backend`
2. 32px gap
3. Animated progress indicator — a horizontal bar, `#D4D4D4` track, `#6366F1` fill that animates from 0 to ~70% over 3 seconds then pulses. Width: full column. Height: 3px.
4. 16px gap
5. Status label — 14px, `#666666`, left-aligned under the bar. Cycles through:
   - "Fetching pull requests…"
   - "Analysing changed functions…"
   - "Building call graphs…"
   (Each phase lasts ~1.5s, fades between them)
6. When complete, transition immediately to Screen 4 (PR Queue).

---

## Screen 4 — PR Queue

**Layout:** Same two-zone layout. Main content column max-width 720px, top padding 48px.

**Content:**
1. Breadcrumb: small text `acme/backend` in `#888888`, 13px, at top
2. 8px gap
3. Heading: "Pull requests" — 22px semibold, `#111111`
4. Subheading: "Top 5 open PRs ranked by wait time and impact." — 14px, `#666666`, 8px below
5. 24px gap
6. **PR List** — up to 5 rows. Each row is a card:
   - Height: auto, min 72px
   - White background, `#D4D4D4` border, 8px border-radius
   - 12px gap between cards
   - Hover state: `#F5F5F5` background, cursor pointer
   - **Left edge accent bar** (4px wide, full height): `#6366F1` for the highest urgency PR, `#C7D2FE` for the rest
   
   **Card layout (16px internal padding):**
   - Row 1: PR number `#AAAAAA · #42` and title `#111111` 15px semibold, inline
   - Row 2 (8px below): One-line AI summary — 14px `#666666`
   - Row 3 (10px below): Three small badges inline:
     - Time open: clock icon + "3 days" — `#F0F0F0` bg, `#D4D4D4` border, `#666666` text, 11px
     - Functions affected: graph-node icon + "12 functions" — same style
     - Risk: coloured dot + "Medium risk" — dot colour matches risk level (green/amber/red), `#666666` text
   - Far right: chevron `›` in `#AAAAAA`, vertically centred

---

## Screen 5 — PR Graph View

This is the primary screen. It has two zones:

```
┌──────────────────┬───────────────────────────────────────┐
│                  │                                       │
│  Node Detail     │  Graph Canvas                        │
│  Panel           │                                       │
│  (~280px)        │  (remaining width)                   │
│                  │                                       │
└──────────────────┴───────────────────────────────────────┘
```

![Graph View Reference](graph-view-reference.png)

**Both the Node Detail Panel and Graph Canvas use a light theme** (panel `#DCDCDC`, canvas `#EBE9E9`; dark text).

---

### Node Detail Panel (~280px wide, `#DCDCDC` background, right border `1px #E4E4E4`)

This panel updates when the user clicks a node in the graph. Default state (no node selected) shows:

**Default state:**
- Centred placeholder text: "Select a node to inspect it." — 14px `#999999`

**Node selected state (top to bottom, 20px padding):**

1. **File path** — `path/to/file.go` — 11px, `#AAAAAA`, at very top.

2. **Package label** — e.g. `types.MockPV` — 11px, `#EF5DA8` (fuchsia), 8px below file path.

3. **Function name** — 22px semibold, `#111111`, 4px below package label.

4. **Description** — 14px, `#555555`, line-height 1.6, 12px below name.

5. **"What's Changed?" card** (only for directly modified functions) — 16px below description:
   - Container: `#F0FFF4` background, `#BBF7D0` left border (3px), 8px border-radius, 12px padding.
   - Label: "What's Changed?" — 12px semibold, `#166534`, margin-bottom 6px.
   - Body: 13px, `#333333`, line-height 1.6.

6. **"Calls" section** — 20px below the changed card (or description if no change card):
   - Label: "Calls" — 11px uppercase, `#AAAAAA`, letter-spacing 0.08em.
   - **Call rows**: package label (11px, package color) + function name (13px, `#222222`).
   - If no calls, omit section.

7. **"Is Called By" section** — 16px below Calls, same row structure.

**Status badges:**
- **Deleted** — red pill: `#FEE2E2` background, `#EF4444` text, 10px, 4px border-radius.
- **Added** — green pill: `#DCFCE7` background, `#16A34A` text, 10px, 4px border-radius.

---

### Graph Canvas (remaining width, `#EBE9E9` background)

**Top bar** (`#E1E1E1` background, `#D4D4D4` bottom border, 48px height):
- Left: "← Back" link in `#888888`, separator `·`, PR number in `#888888`, PR title in `#111111` semibold.
- Right: "View on GitHub →" link in `#6366F1`.

**Graph layout:** Concentric rings. Changed nodes sit at the centre; BFS assigns each connected node a ring level. Level 0 nodes spread horizontally; outer rings are evenly distributed around a circle whose radius adapts to node count (`max(level × 300px, count × 220px / 2π)`). Pan and zoom freely.

**Node color by kind:**

| Kind | Background |
|---|---|
| `function`, `method` | `#D5E7EB` (steel blue-grey) |
| `struct`, `type` | `#CBCCE5` (lavender) |
| `interface` | `#E5C8DC` (rose) |
| Selected (any kind) | `#F5F5F5` |

**Node anatomy** (colored card, `box-shadow: 0 1px 4px rgba(0,0,0,0.12)`, 8px border-radius, 10px padding, monospace font):
- **Package label** — 11px, `#EF5DA8` (fuchsia), top of card. Format: `pkg` for functions; `pkg.ReceiverType` for methods (package inferred from file path directory).
- **Function name** — 13px semibold, `#111111`.
- **Parameters** — 11px per line: param name in `#444444`, type in `#0088FF` (blue).
- **Return types** — 11px, `#FF383C` (red), below a 1px `rgba(0,0,0,0.1)` divider.
- **Status badge**: "Deleted" red pill or "Added" green pill, inline after returns.

**Diff pills** (changed nodes only) — rendered **below** the card, not inside:
- Added lines: `#EF5DA8` background, white text, 12px border-radius, `+N` format.
- Removed lines: `#E08D8D` background, white text, 12px border-radius, `-N` format.

**Node states:**
- **Default:** kind-colored card, standard shadow.
- **Selected:** `#F5F5F5` background, stronger shadow `0 4px 16px rgba(0,0,0,0.18)`.

**Handles:** All 4 sides (Top target, Left target, Right source, Bottom source), opacity 0. `ConnectionMode.Loose` enabled.

**Edges:**
- Bezier curves, 1.5px stroke, `MarkerType.ArrowClosed` triangle arrowhead.
- Color matches the **source node's kind color** (same table as node backgrounds).

**Zoom controls:** bottom-right corner. `+` / `−` / `⊡ fit` buttons, 36px each, white bg, `#E4E4E4` border, 6px border-radius, `#444444` icon text.

**Node count cap:** Maximum 20 nodes. If exceeded, show "Showing 20 of {n} affected functions" in `#888888` 12px, bottom-left of canvas.

---

## Responsive Behaviour

Designed for desktop only (1280px+ wide screens). No mobile layout required.

---

## Interaction Summary

| Action | Result |
|---|---|
| Click PR card | Navigate to PR Graph View for that PR |
| Click graph node | Update Node Detail Panel with that node's data |
| Click "Show diff" | Expand inline diff inside Node Detail Panel |
| Click node chip in "Calls" / "Called by" | Select that node |
| Click empty canvas | Deselect node, panel shows default state |
| Click "View on GitHub →" | Open GitHub PR URL in new tab |
| Click back breadcrumb | Return to PR Queue |
| Zoom controls | Zoom graph canvas in/out or fit to screen |

---

## Component Inventory

| Component | Description |
|---|---|
| `LoginPage` | Full-screen login with GitHub button |
| `RepoSelector` | Searchable repo list with single-select |
| `IndexingState` | Animated progress bar with status messages |
| `PRQueue` | List of PR cards with urgency ordering |
| `PRCard` | Single PR row: title, summary, badges, chevron |
| `GraphCanvas` | Dagre-layout graph canvas (pan/zoom/click), `#EBE9E9` bg |
| `GraphNode` | White card node: package label, function name, params, return types, optional diff badges |
| `NodeDetailPanel` | Light side panel (`#DCDCDC` bg): file path, package label, function name, description, What's Changed card, Calls/Called-by rows |
| `DiffBlock` | Inline unified diff renderer with green/red line highlighting |
| `TopBar` | PR breadcrumb, title, and GitHub link (`#E1E1E1` background) |
| `AppSidebar` | Narrow left sidebar (`#E1E1E1` background) with logo |
