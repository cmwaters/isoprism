# Common React UI

## Purpose

Common React components are the primary shared surface between CLI and cloud. The products keep separate APIs, storage, and runtime behavior, while the UI consumes normalized repo, review, graph, and code contracts.

## Workspace Model

```ts
type RepoWorkspaceModel = {
  repo: RepoInfo;
  programs: GraphProgram[];
  reviewItems?: ReviewItem[];
  graphClient: GraphClient;
};
```

`reviewItems` may be empty or omitted. Shared components must treat that as a normal repo-browsing state.

## Review Items

```ts
type ReviewItem = {
  id: string;
  kind: "pull_request" | "local_diff" | "staged_diff" | "unstaged_diff";
  source: "cloud" | "gh" | "git";
  title: string;
  number?: number;
  summary?: string;
  author?: string;
  url?: string;
  baseRef?: string;
  headRef?: string;
  additions?: number;
  deletions?: number;
  status?: string;
  openedAt?: string;
  riskScore?: number;
};
```

Cloud currently maps PR queue rows to `ReviewItem`. Local may initially provide no review items, then later populate PRs through `gh`.

## Graph Client

```ts
interface GraphClient {
  getProgramGraph(programID: string): Promise<RepoGraphResponse>;
  getReviewGraph(reviewItemID: string): Promise<GraphResponse>;
  expand(req: GraphExpansionRequest): Promise<GraphExpansionResponse>;
  getNodeCode(nodeID: string, context?: CodeContext): Promise<NodeCodeResponse>;
  getIssue?(ref: GitHubIssueReference): Promise<GitHubIssueDescription>;
}
```

Cloud and local each provide their own adapter. Shared components should call the interface, not hardcode cloud or local route shapes.

## Shared Components

Common components should include:

- `RepoWorkspace`: main repo/graph layout.
- `RepoPanel`: repo metadata, programs, and optional review items.
- `GraphWorkspace`: graph canvas and detail panel orchestration.
- `GraphCanvas`: React Flow graph surface.
- `GraphNode`: rendered semantic node card.
- `NodeDetailPanel`: overview, contents, code, tests, and relations.
- `ComponentChangePanel`: focused diff/file detail panel.
- Graph controls: zoom, fit, selection, expansion.

## Product-Specific Shells

Cloud shell owns auth, settings, onboarding, admin, and feedback.

Local shell owns daemon loading, local error states, and local graph client creation.

Neither shell should fork the graph card, layout, or node detail experience without a product-specific reason.

## Visual Direction

The shared graph UI should remain calm and instrument-like:

- Inter or equivalent sans-serif for UI text.
- Monospace for code and symbol details.
- Light warm background.
- Minimal borders.
- No decorative gradients or ornamental effects.
- High contrast for selected/changed state.

## Node Cards

Node cards display:

- package/receiver context
- function/type title
- inputs
- outputs
- kind-specific color
- selected state
- diff pills below the card when changed

Edges attach to the raw card body and exclude diff pills from geometry.

## Optional Review UI

When `reviewItems` is absent or empty, the shared panel should show repo/program browsing only. It should not render a cloud-specific empty PR queue.
