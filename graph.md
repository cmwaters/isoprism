# Graph Layout Spec

## Goal

Create a graph layout that works for small PRs and large systems by keeping connected nodes physically close, preventing overlap, preserving readable density, and lazily loading more graph context as users explore.

The layout is based on a weighted seed set, not a single focal node.

## Current Implementation

The current implementation uses the weighted seed set, depth-2 loading, a 150-node visible budget, boundary-node marking, and deterministic hex-grid placement.

The API includes layout metadata on each node:

```text
weight
degree
graph_depth
boundary
```

Clustering and interactive boundary expansion are still design targets; the current client marks boundary nodes through metadata and keeps them on the outer hex ring, but it does not yet request incremental expansion from a clicked boundary node.

The canonical graph remains function-level. Production nodes and call edges are extracted by the API parser with tree-sitter. Call edge resolution is conservative: unresolved external calls, ambiguous names, and selector/member calls with unknown receiver types are omitted instead of guessed.

The API serves the canonical function-level graph directly.

## Views

### Repo View

The repo page does not load the repo-wide graph by default. It opens with the PR queue and an empty graph canvas so large repositories do not pay the base-graph query/render cost before the user chooses a PR.

The repo graph API still supports repo-wide graph responses for future exploration surfaces.

Repo-wide graph seed nodes are entrypoint functions.

For Go, the first entrypoint signal is `main`.

Future entrypoints can include HTTP handlers, CLI commands, workers, exported APIs, and scheduled jobs.

### PR View

Seed nodes are all changed nodes.

Changed nodes with larger changes and more graph connectivity appear closer to the center.

Context nodes are loaded around the changed seed set.

Code edges are directional semantic relationships with `source_id`, `destination_id`, and `edge_kind`. Call edges use `edge_kind = 'calls'` and are built from a conservative resolver index. Tree-sitter identifies call expressions and declarations, then the resolver adds safe semantic facts such as Go receiver types, parameter types, struct field types, and import aliases. Go imported selector calls match the imported repository path and selector name without assuming the declared package name equals the import path basename, so aliases such as `grpccore.StartGRPCServer(...)` can resolve to `rpc/grpc:coregrpc.StartGRPCServer`. Field-chain calls like `blockAPI.env.EventBus.Unsubscribe(...)` resolve only when every field hop maps to exactly one known type and the final method exists as a graph node. Ambiguous receiver types, unknown fields, external packages, and selector-name-only matches are intentionally skipped rather than drawn as misleading edges.

Go receiver ownership is also persisted as `edge_kind = 'owns_method'`. The receiver type is the source and the method is the destination, so `rpc/grpc:coregrpc.BlockAPI` links to `rpc/grpc:coregrpc.BlockAPI.Stop` even when the struct declaration itself is unchanged.

The PR summary panel uses GitHub's Pull Request Files API as the file-diff source of truth. Every changed file returned by GitHub is available with its status, previous filename for renames, additions, deletions, and patch when GitHub provides one. This file list captures non-semantic changes such as docs, config, package metadata, global variables, and unsupported/generated files even when those changes do not produce graph nodes.

Semantic graph changes are stored separately in `pr_node_changes`. Functions, methods, structs, interfaces, and type declarations can be `added`, `modified`, `deleted`, or `renamed`; renamed nodes keep `old_full_name` and `old_file_path` so the PR view can show the previous symbol/file alongside the current one.

Parsed components also preserve adjacent source comments in `code_nodes.doc_comment`. Only contiguous `//`, `/* ... */`, or `/** ... */` blocks immediately above the component are captured; comments separated by a blank line are ignored. Graph responses expose the raw `doc_comment` and prepend it to the displayed `summary`, while AI enrichment remains unchanged.

Rename detection is intentionally conservative. A new component is considered renamed only when the processor can pair it with a base component by rename metadata or matching body hash. Line-range overlap alone does not create a rename, because insertions can shift existing components and make unrelated functions overlap. In ambiguous cases the head component remains `added` and any unmatched base component remains `deleted`.

When a renamed component is opened, the detail endpoint uses the old symbol/file identity for the base version and the current identity for the head version. This keeps the rendered component diff aligned with the `+/-` counts shown on the PR cards and graph nodes. The UI reconstructs a full component diff from source only when the relevant sides are available: both base and head for modified or renamed components, head for added components, and base for deleted components. If a source lookup is incomplete, the UI falls back to the persisted `pr_node_changes.diff_hunk` instead of treating the visible side as a whole-component add or delete.

The UI renders symbol names as context plus title: package and receiver/type context appears in the pink metadata label, while the black title shows only the function or method name. For example, `rpc/grpc:coregrpc.BlockAPI.Stop` renders as `grpc.BlockAPI` above `Stop`.

Changed test functions are also stored in `pr_node_changes`, but the PR graph endpoint returns them in `test_changes[]` instead of rendering them as graph nodes. Test changes use the same labels and component diff fields as graph changes.

The PR overview only lists changed test entrypoints (`is_test = true` and `is_entrypoint = true`) under Test changes. Changed test helpers are still returned in `test_changes[]` and test-to-test edges are included in the graph payload so selecting a test entrypoint can show the focused test tree with helper components such as setup helpers or channel-fill helpers.

After the PR description, the PR overview is grouped into:

1. **Graph changes**: all changed production components shown in the graph.
2. **Test changes**: all changed test functions returned by `test_changes[]`.
3. **Documentation changes**: Markdown file diffs from the GitHub file list.
4. **Other changes**: remaining file diffs not captured by graph, tests, or documentation.

Clicking any row opens a resizable middle detail panel between the PR overview and the graph. Graph/test rows show the component overview first and the diff/code view below it; documentation/other rows open the file-level patch. Opening or resizing the panel refits the graph into the remaining canvas.

## Node Weight

Use a simple score:

```text
node_weight =
  lines_changed
+ caller_count
+ callee_count
```

Where:

```text
lines_changed = lines_added + lines_removed
caller_count = number of visible or known nodes that call this node
callee_count = number of visible or known nodes this node calls
```

For unchanged context nodes:

```text
lines_changed = 0
```

This lets highly connected context nodes rank above isolated low-impact changed nodes, while changed nodes generally dominate because they have non-zero line changes.

Optional normalization later:

```text
node_weight =
  lines_changed
+ log2(1 + caller_count)
+ log2(1 + callee_count)
```

The first implementation should use the plain additive version.

## Neighborhood Loading

The graph is loaded by degree radius.

Default:

```text
depth = 2
```

For a seed set `S`, load:

```text
degree 0: seed nodes
degree 1: nodes directly connected to seeds
degree 2: nodes connected to degree-1 nodes
```

A node is a boundary node if it has additional connected nodes outside the loaded neighborhood.

When a user selects or expands a boundary node:

```text
new_seed_set = previous_seed_set + boundary_node
```

The client requests missing nodes needed to restore the same depth around the expanded seed set.

## Visible Graph Budget

The layout should not render every node as a full card.

Suggested initial budgets:

```text
max_full_cards = 60
max_visible_nodes = 150
```

If the depth-2 neighborhood exceeds the visible budget:

1. Keep all high-weight seed nodes.
2. Keep direct neighbors of high-weight seed nodes.
3. Cluster lower-priority nodes by file or package.
4. Represent each cluster as a compact continuation node.

Cluster labels:

```text
main.go
17 nodes
```

or:

```text
internal/store
42 nodes
```

## Placement Model

Use a proximity-based objective function.

The layout should minimize:

```text
total_cost =
  edge_distance_cost
+ center_weight_cost
+ overlap_cost
+ density_cost
+ boundary_cost
```

### Edge Distance Cost

Connected nodes should be close:

```text
edge_distance_cost =
  sum_edges (distance(u, v) - ideal_edge_length)^2
```

Default:

```text
ideal_edge_length = 1 grid step
```

### Center Weight Cost

High-weight nodes should be closer to the center:

```text
center_weight_cost =
  sum_nodes node_weight(v) * distance(v, center)^2
```

In PR view, this naturally pulls larger changed nodes and more connected changed nodes toward the middle.

### Overlap Cost

Nodes cannot overlap:

```text
overlap_cost = very large penalty if node rectangles overlap
```

For a hex-grid implementation, this becomes simpler:

```text
one node or cluster per occupied hex slot
```

### Density Cost

The graph should maintain consistent visual density:

```text
density_cost =
  penalty for cells that are too sparse or too crowded
```

Practical rule:

```text
prefer placing nodes in the nearest available hex cell
avoid leaving holes in inner rings while using outer rings
```

### Boundary Cost

Boundary nodes should sit near the outer edge of the visible graph:

```text
boundary_cost =
  penalty if boundary nodes are placed too close to the center
```

This makes clustered nodes feel like continuation points.

## Hex Placement

Use a hexagonal grid as the final placement surface.

Reasons:

- predictable density
- no overlap
- natural rings around the center
- good fit for degree-based neighborhoods
- easy lazy expansion from boundary nodes

Placement steps:

1. Compute node weights.
2. Sort nodes by priority.
3. Reserve central hex cells for highest-weight seed nodes.
4. Place remaining seed nodes in nearby rings.
5. Place degree-1 context around the seed nodes they connect to.
6. Place degree-2 context farther out.
7. Place boundary nodes on the outer ring.
8. Run local swaps to reduce total edge length.

Priority order:

```text
selected node
high-weight changed nodes
other changed nodes
degree-1 context nodes
degree-2 context nodes
boundary nodes
clusters
```

If there is no selected node, start from high-weight seeds.

## PR View Behavior

Given 100 changed nodes:

1. Compute `node_weight` for all changed nodes.
2. Place the highest-weight changed nodes closest to center.
3. Place other changed nodes in surrounding rings.
4. Load callers/callees up to depth 2.
5. If too many nodes are visible, cluster low-weight nodes by file/package.
6. Boundary nodes indicate hidden continuation.

This means the PR view is centered on the largest and most connected changes, not on an arbitrary first changed node.

## Repo View Behavior

Given a repo graph:

1. Find entrypoint seeds.
2. For Go, use `main`.
3. Load depth-2 neighborhood from entrypoints.
4. Place entrypoints near center.
5. Place direct callees in ring 1.
6. Place second-degree callees/callers in ring 2.
7. Show boundary nodes for unexplored graph regions.

## Incremental Layout

When new nodes are lazy-loaded:

1. Existing nodes keep their current hex positions where possible.
2. New nodes appear near the boundary node or seed that loaded them.
3. Run a local layout relaxation only around the affected area.
4. Avoid global reshuffling unless the user explicitly resets layout.

This preserves the user's mental map.

## Acceptance Criteria

- No node cards overlap.
- Connected nodes are generally closer than unconnected nodes.
- Highest-weight changed nodes appear near the center in PR view.
- Repo view starts from entrypoint nodes.
- The graph remains readable at 10 nodes and at 1,000 total repo nodes.
- Rendering does not require loading the full graph.
- Expanding a boundary node restores the configured degree depth.
- Existing visible node positions remain stable during lazy expansion.
