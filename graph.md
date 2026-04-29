# Graph Layout Spec

## Goal

Create a graph layout that works for small PRs and large systems by keeping connected nodes physically close, preventing overlap, preserving readable density, and lazily loading more graph context as users explore.

The layout is based on a weighted seed set, not a single focal node.

## Current Implementation

The first implementation uses the weighted seed set, depth-2 loading, a 150-node visible budget, boundary-node marking, and deterministic hex-grid placement.

The API includes layout metadata on each node:

```text
weight
degree
graph_depth
boundary
```

Clustering and interactive boundary expansion are still design targets; the current client marks boundary nodes through metadata and keeps them on the outer hex ring, but it does not yet request incremental expansion from a clicked boundary node.

## Views

### Repo-Wide View

Seed nodes are entrypoints.

For Go, the first entrypoint is `main`.

Future entrypoints can include HTTP handlers, CLI commands, workers, exported APIs, and scheduled jobs.

### PR View

Seed nodes are all changed nodes.

Changed nodes with larger changes and more graph connectivity appear closer to the center.

Context nodes are loaded around the changed seed set.

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
4. Represent each cluster as a compact expandable node.

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

This makes expandable nodes feel like continuation points.

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
