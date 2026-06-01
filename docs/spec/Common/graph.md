# Common Semantic Graph

## Purpose

The semantic graph is the shared core of CLI and cloud. Both products turn git repositories into semantic nodes and edges, then use common React components to browse and review those graphs.

The API routes and storage are product-specific. The graph concepts and payload shapes are common.

## Nodes

Graph nodes represent parsed source components:

- functions
- methods
- structs/types
- interfaces
- test functions and helpers, when relevant

Common node fields include:

- ID
- full name
- file path
- package path
- line range
- language
- kind
- inputs and outputs
- doc comment
- summary/change summary
- change type for review graphs
- weight, degree, graph depth, and boundary metadata
- related tests

## Edges

Common edge kinds:

- `calls`: caller to callee
- `owns_method`: type/receiver to method
- `uses_type`: type to referenced type

Call resolution is conservative. Ambiguous, external, unknown, or unsafe guesses should be omitted instead of drawn as misleading edges.

## Graph Construction

Both products follow the same conceptual flow:

1. enumerate supported files for a git tree or diff side
2. parse source with tree-sitter
3. identify semantic components
4. build resolver indexes
5. extract call edges
6. add type ownership and type-use edges
7. classify changed components for review contexts
8. rank and bound visible nodes
9. serve graph payloads for the UI

Cloud persists graph data in Postgres. CLI caches parse objects in `.isoprism/` and builds graph snapshots locally.

## Repo Graphs

Repo graphs begin from programs/entrypoints such as Go `main` functions. The initial graph should be bounded and expandable. It should not render the entire repository as cards.

Program graph responses include:

- the selected entrypoint
- direct callers/callees
- a second ring of context
- boundary metadata for hidden continuation

## Review Graphs

Review graphs begin from changed semantic components.

Cloud review graphs are currently PR graphs. Local review graphs may be local diffs, staged diffs, unstaged diffs, or future PRs discovered through `gh`.

Review graph payloads may include:

- changed production nodes in `nodes`
- semantic edges in `edges`
- file-level diffs in `files`
- changed tests in `test_changes`
- additional test context in `test_context`

## Expansion

Expansion is common:

```json
{
  "node_id": "expanded-node-id",
  "visible_node_ids": ["already-visible-id"],
  "graph_context": { "mode": "repo" }
}
```

Review context may include a review identifier:

```json
{
  "graph_context": { "mode": "pr", "pr_id": "pull-request-id" }
}
```

The response returns newly loaded nodes plus all visible edges whose endpoints are in the updated visible set:

```ts
type GraphExpansionResponse = {
  nodes: GraphNode[];
  edges: GraphEdge[];
  expanded_node_id: string;
  has_more: boolean;
  hidden_neighbor_count: number;
};
```

## Ranking

Node weight is:

```text
lines_changed + caller_count + callee_count
```

Changed nodes, entrypoints, high-degree nodes, and same-file/package neighbors should rank ahead of lower-signal context.

## Layout

The graph layout should keep connected nodes physically close, avoid overlap, keep text readable, and preserve positions during expansion.

Edges attach to the card body, not diff pills. Anchors stay away from card corners and separate multiple same-face anchors where possible.

## Tests

Tests are semantic evidence. Default production graphs filter test nodes from the canvas, but test relationships should be available in details and focused test views.
