"use client";

import { useCallback, useState, useEffect, useMemo } from "react";
import {
  ReactFlow,
  Node,
  Edge,
  Background,
  BaseEdge,
  EdgeProps,
  useNodesState,

  useReactFlow,
  ReactFlowProvider,
  NodeMouseHandler,
  PanOnScrollMode,
  ConnectionMode,
  MarkerType,
  useStore,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { GraphResponse, GraphNode as APIGraphNode, NodeCodeResponse, QueuePR, RepoGraphResponse, Repository } from "@/lib/types";
import { apiFetch } from "@/lib/api";
import GraphNodeComponent from "./graph-node";
import NodeDetailPanel from "./node-detail-panel";

export const nodeTypes = { graphNode: GraphNodeComponent };
export const edgeTypes = { smartBezier: SmartBezierEdge };

type Point = { x: number; y: number };
type Rect = Point & { width: number; height: number };
type AnchorSide = "top" | "right" | "bottom" | "left";
type Anchor = Point & { normal: Point; side: AnchorSide; offset: number };
type MeasuredNode = {
  internals: { positionAbsolute: Point };
  measured: { width?: number; height?: number };
  width?: number;
  height?: number;
  data?: { node?: APIGraphNode };
};
export type PanelMode = "overview" | "code";

const DIFF_PILLS_HEIGHT = 28;
const CARD_CORNER_MARGIN = 20;
const MIN_ANCHOR_SPACING = 20;

function cardRect(node: MeasuredNode): Rect {
  const measuredHeight = node.measured.height ?? node.height ?? 120;
  const hasDiffPills = Boolean(node.data?.node?.change_type);

  return {
    x: node.internals.positionAbsolute.x,
    y: node.internals.positionAbsolute.y,
    width: node.measured.width ?? node.width ?? NODE_W,
    height: Math.max(48, measuredHeight - (hasDiffPills ? DIFF_PILLS_HEIGHT : 0)),
  };
}

function rectCenter(rect: Rect): Point {
  return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
}

function anchorFromSide(rect: Rect, side: AnchorSide, offset: number): Anchor {
  const horizontal = side === "top" || side === "bottom";
  const length = horizontal ? rect.width : rect.height;
  const margin = Math.min(CARD_CORNER_MARGIN, length / 2);
  const clampedOffset = Math.max(margin, Math.min(length - margin, offset));

  switch (side) {
    case "top":
      return { x: rect.x + clampedOffset, y: rect.y, normal: { x: 0, y: -1 }, side, offset: clampedOffset };
    case "right":
      return { x: rect.x + rect.width, y: rect.y + clampedOffset, normal: { x: 1, y: 0 }, side, offset: clampedOffset };
    case "bottom":
      return { x: rect.x + clampedOffset, y: rect.y + rect.height, normal: { x: 0, y: 1 }, side, offset: clampedOffset };
    case "left":
      return { x: rect.x, y: rect.y + clampedOffset, normal: { x: -1, y: 0 }, side, offset: clampedOffset };
  }
}

function anchorAtAngle(rect: Rect, angle: number): Anchor {
  const center = rectCenter(rect);
  const dx = Math.cos(angle);
  const dy = Math.sin(angle);
  const scale = Math.min(
    dx === 0 ? Infinity : (rect.width / 2) / Math.abs(dx),
    dy === 0 ? Infinity : (rect.height / 2) / Math.abs(dy)
  );

  let x = center.x + dx * scale;
  let y = center.y + dy * scale;

  if (Math.abs(x - rect.x) < 0.01) {
    y = Math.max(rect.y, Math.min(rect.y + rect.height, y));
    return anchorFromSide(rect, "left", y - rect.y);
  } else if (Math.abs(x - (rect.x + rect.width)) < 0.01) {
    y = Math.max(rect.y, Math.min(rect.y + rect.height, y));
    return anchorFromSide(rect, "right", y - rect.y);
  } else if (Math.abs(y - rect.y) < 0.01) {
    x = Math.max(rect.x, Math.min(rect.x + rect.width, x));
    return anchorFromSide(rect, "top", x - rect.x);
  } else {
    x = Math.max(rect.x, Math.min(rect.x + rect.width, x));
    return anchorFromSide(rect, "bottom", x - rect.x);
  }
}

function faceCenterAnchor(rect: Rect, normal: Point): Anchor {
  if (Math.abs(normal.x) > Math.abs(normal.y)) {
    return anchorFromSide(rect, normal.x > 0 ? "right" : "left", rect.height / 2);
  }

  return anchorFromSide(rect, normal.y > 0 ? "bottom" : "top", rect.width / 2);
}

function edgeAngleFromNode(nodeID: string, nodeRect: Rect, edge: Edge, nodeRects: Map<string, Rect>): number {
  const otherID = edge.source === nodeID ? edge.target : edge.source;
  const otherRect = nodeRects.get(otherID);
  if (!otherRect) return 0;

  const from = rectCenter(nodeRect);
  const to = rectCenter(otherRect);
  return Math.atan2(to.y - from.y, to.x - from.x);
}

function lineAnchor(nodeID: string, rect: Rect, edge: Edge, nodeRects: Map<string, Rect>): Anchor {
  const otherID = edge.source === nodeID ? edge.target : edge.source;
  const otherRect = nodeRects.get(otherID);
  if (!otherRect) return faceCenterAnchor(rect, { x: 1, y: 0 });

  return anchorAtAngle(rect, edgeAngleFromNode(nodeID, rect, edge, nodeRects));
}

function spacedFaceAnchors(rect: Rect, anchors: Anchor[]): Anchor[] {
  if (anchors.length <= 1) return anchors;

  const side = anchors[0].side;
  const length = side === "top" || side === "bottom" ? rect.width : rect.height;
  const margin = Math.min(CARD_CORNER_MARGIN, length / 2);
  const minOffset = margin;
  const maxOffset = length - margin;
  const available = maxOffset - minOffset;
  const spacing = anchors.length > 1 ? Math.min(MIN_ANCHOR_SPACING, available / (anchors.length - 1)) : MIN_ANCHOR_SPACING;

  const byOffset = anchors
    .map((anchor, index) => ({ anchor, index }))
    .sort((a, b) => a.anchor.offset - b.anchor.offset || a.index - b.index);

  const offsets = byOffset.map(({ anchor }) => Math.max(minOffset, Math.min(maxOffset, anchor.offset)));

  for (let i = 1; i < offsets.length; i++) {
    offsets[i] = Math.max(offsets[i], offsets[i - 1] + spacing);
  }

  const overflow = offsets[offsets.length - 1] - maxOffset;
  if (overflow > 0) {
    for (let i = 0; i < offsets.length; i++) offsets[i] -= overflow;
  }

  for (let i = offsets.length - 2; i >= 0; i--) {
    offsets[i] = Math.min(offsets[i], offsets[i + 1] - spacing);
  }

  const underflow = minOffset - offsets[0];
  if (underflow > 0) {
    for (let i = 0; i < offsets.length; i++) offsets[i] += underflow;
  }

  const result = [...anchors];
  byOffset.forEach(({ index }, sortedIndex) => {
    result[index] = anchorFromSide(rect, side, offsets[sortedIndex]);
  });

  return result;
}

function endpointAnchor(nodeID: string, edgeID: string, rect: Rect, edges: Edge[], nodeRects: Map<string, Rect>): Anchor {
  const incidentEdges = edges.filter((edge) => edge.source === nodeID || edge.target === nodeID);

  if (incidentEdges.length <= 1) {
    const edge = incidentEdges[0];
    return edge ? lineAnchor(nodeID, rect, edge, nodeRects) : faceCenterAnchor(rect, { x: 1, y: 0 });
  }

  const anchorsByEdge = incidentEdges.map((edge) => ({ edge, anchor: lineAnchor(nodeID, rect, edge, nodeRects) }));
  const anchorsBySide = new Map<AnchorSide, { edge: Edge; anchor: Anchor }[]>();

  for (const item of anchorsByEdge) {
    const sideAnchors = anchorsBySide.get(item.anchor.side) ?? [];
    sideAnchors.push(item);
    anchorsBySide.set(item.anchor.side, sideAnchors);
  }

  for (const sideAnchors of anchorsBySide.values()) {
    const spacedAnchors = spacedFaceAnchors(rect, sideAnchors.map(({ anchor }) => anchor));
    sideAnchors.forEach((item, index) => {
      item.anchor = spacedAnchors[index];
    });
  }

  return anchorsByEdge.find(({ edge }) => edge.id === edgeID)?.anchor ?? lineAnchor(nodeID, rect, incidentEdges[0], nodeRects);
}

function smartBezierPath(sourceAnchor: Anchor, targetAnchor: Anchor): string {
  const distance = Math.hypot(targetAnchor.x - sourceAnchor.x, targetAnchor.y - sourceAnchor.y);
  const controlDistance = Math.max(36, Math.min(180, distance * 0.36));
  const sourceControl = {
    x: sourceAnchor.x + sourceAnchor.normal.x * controlDistance,
    y: sourceAnchor.y + sourceAnchor.normal.y * controlDistance,
  };
  const targetControl = {
    x: targetAnchor.x + targetAnchor.normal.x * controlDistance,
    y: targetAnchor.y + targetAnchor.normal.y * controlDistance,
  };

  return `M ${sourceAnchor.x},${sourceAnchor.y} C ${sourceControl.x},${sourceControl.y} ${targetControl.x},${targetControl.y} ${targetAnchor.x},${targetAnchor.y}`;
}

function SmartBezierEdge({
  id,
  source,
  target,
  sourceX,
  sourceY,
  targetX,
  targetY,
  markerEnd,
  style,
  interactionWidth,
}: EdgeProps) {
  const sourceNode = useStore((store) => store.nodeLookup.get(source));
  const targetNode = useStore((store) => store.nodeLookup.get(target));
  const flowEdges = useStore((store) => store.edges);
  const nodeLookup = useStore((store) => store.nodeLookup);

  const path = sourceNode && targetNode
    ? (() => {
        const nodeRects = new Map(
          Array.from(nodeLookup.entries()).map(([nodeID, node]) => [nodeID, cardRect(node)])
        );
        const sourceRect = cardRect(sourceNode);
        const targetRect = cardRect(targetNode);
        return smartBezierPath(
          endpointAnchor(source, id, sourceRect, flowEdges, nodeRects),
          endpointAnchor(target, id, targetRect, flowEdges, nodeRects)
        );
      })()
    : `M ${sourceX},${sourceY} C ${sourceX},${sourceY} ${targetX},${targetY} ${targetX},${targetY}`;

  return (
    <BaseEdge
      id={id}
      path={path}
      markerEnd={markerEnd}
      style={style}
      interactionWidth={interactionWidth}
    />
  );
}

// ── Card color by kind ────────────────────────────────────────────────────────
export function cardColorByKind(kind: string): string {
  switch (kind) {
    case "function":
    case "method":
      return "#D5E7EB";
    case "struct":
    case "type":
      return "#CBCCE5";
    case "interface":
      return "#E5C8DC";
    default:
      return "#D5E7EB";
  }
}

// ── Weighted hex-grid layout ──────────────────────────────────────────────────
const NODE_W = 280;
const HEX_X = 360;
const HEX_Y = 300;
const PANEL_MIN_WIDTH = 260;
const PANEL_MAX_WIDTH = 620;
const PANEL_DEFAULT_WIDTH = 320;

export function hexGridLayout(nodes: Node[], edges: Edge[], graphNodes: APIGraphNode[]): Node[] {
  type Hex = { q: number; r: number; ring: number; x: number; y: number };

  const graphByID = new Map(graphNodes.map((n) => [n.id, n]));
  const nodeByID = new Map(nodes.map((n) => [n.id, n]));
  const neighbors = new Map<string, string[]>();
  nodes.forEach((n) => neighbors.set(n.id, []));
  edges.forEach((e) => {
    if (!nodeByID.has(e.source) || !nodeByID.has(e.target)) return;
    neighbors.get(e.source)?.push(e.target);
    neighbors.get(e.target)?.push(e.source);
  });

  const ringOf = (q: number, r: number) => Math.max(Math.abs(q), Math.abs(r), Math.abs(-q - r));
  const hexToPoint = (q: number, r: number) => ({
    x: HEX_X * (q + r / 2),
    y: HEX_Y * r,
  });

  const maxDepth = Math.max(0, ...graphNodes.map((n) => n.graph_depth ?? 0));
  const outerRing = Math.max(maxDepth + 1, Math.ceil(Math.sqrt(nodes.length / 3)) + 1);
  const cells: Hex[] = [];
  for (let q = -outerRing; q <= outerRing; q++) {
    for (let r = -outerRing; r <= outerRing; r++) {
      const ring = ringOf(q, r);
      if (ring <= outerRing) cells.push({ q, r, ring, ...hexToPoint(q, r) });
    }
  }
  cells.sort((a, b) => a.ring - b.ring || a.y - b.y || a.x - b.x);

  const rank = (node: Node) => {
    const meta = graphByID.get(node.id);
    const seed = meta?.node_type === "changed" || meta?.node_type === "entrypoint";
    const typeRank = seed ? 0 : meta?.boundary ? 3 : (meta?.graph_depth ?? 2);
    return {
      typeRank,
      weight: meta?.weight ?? 0,
      degree: meta?.degree ?? (neighbors.get(node.id)?.length ?? 0),
      depth: meta?.graph_depth ?? 2,
      file: meta?.file_path ?? "",
    };
  };

  const ordered = [...nodes].sort((a, b) => {
    const ra = rank(a);
    const rb = rank(b);
    return ra.typeRank - rb.typeRank
      || rb.weight - ra.weight
      || ra.depth - rb.depth
      || rb.degree - ra.degree
      || ra.file.localeCompare(rb.file)
      || a.id.localeCompare(b.id);
  });

  const assigned = new Map<string, Hex>();
  const occupied = new Set<string>();
  const cellKey = (cell: Hex) => `${cell.q},${cell.r}`;
  const desiredRing = (id: string) => {
    const meta = graphByID.get(id);
    if (meta?.boundary) return outerRing;
    if (meta?.node_type === "changed" || meta?.node_type === "entrypoint") {
      return (meta.weight ?? 0) > 0 ? 0 : 1;
    }
    return Math.max(1, meta?.graph_depth ?? 2);
  };
  const dist = (a: Hex, b: Hex) => {
    const dx = a.x - b.x;
    const dy = a.y - b.y;
    return Math.sqrt(dx * dx + dy * dy);
  };

  ordered.forEach((node) => {
    const targetRing = desiredRing(node.id);
    let best = cells.find((cell) => !occupied.has(cellKey(cell))) ?? cells[0];
    let bestCost = Number.POSITIVE_INFINITY;
    cells.forEach((cell) => {
      if (occupied.has(cellKey(cell))) return;
      const ringCost = Math.abs(cell.ring - targetRing) * 5000;
      const centerCost = (graphByID.get(node.id)?.weight ?? 0) * cell.ring * cell.ring * 4;
      const edgeCost = (neighbors.get(node.id) ?? []).reduce((sum, nb) => {
        const placed = assigned.get(nb);
        return placed ? sum + dist(cell, placed) : sum;
      }, 0);
      const cost = ringCost + edgeCost + centerCost;
      if (cost < bestCost || (cost === bestCost && cell.ring < best.ring)) {
        best = cell;
        bestCost = cost;
      }
    });
    assigned.set(node.id, best);
    occupied.add(cellKey(best));
  });

  const edgeLengthScore = (id: string, cell: Hex) => (neighbors.get(id) ?? []).reduce((sum, nb) => {
    const other = assigned.get(nb);
    return other ? sum + dist(cell, other) : sum;
  }, 0);

  for (let pass = 0; pass < 3; pass++) {
    for (let i = 0; i < ordered.length; i++) {
      for (let j = i + 1; j < ordered.length; j++) {
        const a = ordered[i];
        const b = ordered[j];
        const cellA = assigned.get(a.id);
        const cellB = assigned.get(b.id);
        if (!cellA || !cellB) continue;
        if (Math.abs(cellA.ring - desiredRing(b.id)) > 1 || Math.abs(cellB.ring - desiredRing(a.id)) > 1) continue;
        const before = edgeLengthScore(a.id, cellA) + edgeLengthScore(b.id, cellB);
        const after = edgeLengthScore(a.id, cellB) + edgeLengthScore(b.id, cellA);
        if (after + 20 < before) {
          assigned.set(a.id, cellB);
          assigned.set(b.id, cellA);
        }
      }
    }
  }

  const positions = new Map<string, { x: number; y: number }>();
  assigned.forEach((cell, id) => positions.set(id, { x: cell.x, y: cell.y }));

  return nodes.map((n) => ({
    ...n,
    position: positions.get(n.id) ?? { x: 0, y: 0 },
  }));
}

// ── Inner canvas ──────────────────────────────────────────────────────────────
type UnifiedGraph = GraphResponse | RepoGraphResponse;

function isPRGraph(graph: UnifiedGraph): graph is GraphResponse {
  return "pr" in graph;
}

function InnerCanvas({
  graph,
  repoID,
  token,
  repo,
  prs = [],
}: {
  graph: UnifiedGraph;
  repoID: string;
  token: string;
  repo?: Repository;
  prs?: QueuePR[];
}) {
  const { fitView } = useReactFlow();
  const [activeGraph, setActiveGraph] = useState<UnifiedGraph>(graph);
  const [prGraphCache, setPRGraphCache] = useState<Record<number, GraphResponse>>({});
  const [loadingPRNumber, setLoadingPRNumber] = useState<number | null>(null);
  const [selectedNode, setSelectedNode] = useState<APIGraphNode | null>(null);
  const [panelMode, setPanelMode] = useState<PanelMode>("overview");
  const [panelWidth, setPanelWidth] = useState(PANEL_DEFAULT_WIDTH);
  const [nodeCodeCache, setNodeCodeCache] = useState<Record<string, NodeCodeResponse>>({});

  const initialNodes: Node[] = useMemo(() => activeGraph.nodes.map((n) => ({
    id: n.id,
    type: "graphNode",
    data: { node: n },
    position: { x: 0, y: 0 },
  })), [activeGraph.nodes]);

  const baseEdges: Edge[] = useMemo(() => activeGraph.edges.map((e, idx) => ({
    id: `e${idx}`,
    source: e.caller_id,
    target: e.callee_id,
    type: "smartBezier",
  })), [activeGraph.edges]);

  const [nodes, setNodes, onNodesChange] = useNodesState(
    hexGridLayout(initialNodes, baseEdges, activeGraph.nodes)
  );

  // Recompute edge styles whenever selection changes
  const edges: Edge[] = useMemo(() => {
    const selID = selectedNode?.id ?? null;
    return baseEdges.map((e) => {
      const isConnected = selID && (e.source === selID || e.target === selID);
      const isDimmed = selID && !isConnected;
      const color = isConnected ? "#333333" : isDimmed ? "#CCCCCC" : "#888888";
      const width = isConnected ? 2 : 1;
      return {
        ...e,
        style: { stroke: color, strokeWidth: width },
        markerEnd: { type: MarkerType.ArrowClosed, width: 10, height: 10, color },
      };
    });
  }, [baseEdges, selectedNode]);

  const onEdgesChange = useCallback(() => {}, []);

  useEffect(() => {
    setActiveGraph(graph);
  }, [graph]);

  useEffect(() => {
    setNodes(hexGridLayout(initialNodes, baseEdges, activeGraph.nodes));
    setSelectedNode(null);
    setPanelMode("overview");
    setTimeout(() => fitView({ padding: 0.15 }), 50);
  }, [activeGraph, baseEdges, fitView, initialNodes, setNodes]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_, node) => {
      const apiNode = activeGraph.nodes.find((n) => n.id === node.id) ?? null;
      setSelectedNode(apiNode);
    },
    [activeGraph.nodes]
  );

  const onPaneClick = useCallback(() => {
    setSelectedNode(null);
    setPanelMode("overview");
  }, []);

  const onResizePanel = useCallback((nextWidth: number) => {
    setPanelWidth(Math.max(PANEL_MIN_WIDTH, Math.min(PANEL_MAX_WIDTH, nextWidth)));
  }, []);

  const onCacheNodeCode = useCallback((nodeID: string, code: NodeCodeResponse) => {
    setNodeCodeCache((current) => current[nodeID] ? current : { ...current, [nodeID]: code });
  }, []);

  const onSelectPR = useCallback(async (prNumber: number) => {
    const cached = prGraphCache[prNumber];
    if (cached) {
      setActiveGraph(cached);
      return;
    }

    setLoadingPRNumber(prNumber);
    try {
      const prGraph = await apiFetch<GraphResponse>(`/api/v1/repos/${repoID}/prs/number/${prNumber}/graph`, token);
      setPRGraphCache((current) => ({ ...current, [prNumber]: prGraph }));
      setActiveGraph(prGraph);
    } finally {
      setLoadingPRNumber(null);
    }
  }, [prGraphCache, repoID, token]);

  const onBackToRepo = useCallback(() => {
    setActiveGraph(graph);
  }, [graph]);

  const totalNodes = activeGraph.nodes.length;
  const maxNodes = 20;

  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw" }}>
      <NodeDetailPanel
        node={selectedNode}
        allNodes={activeGraph.nodes}
        edges={activeGraph.edges}
        onSelectNode={(id) => {
          const n = activeGraph.nodes.find((n) => n.id === id) ?? null;
          setSelectedNode(n);
        }}
        repoID={repoID}
        repo={repo ?? (isPRGraph(activeGraph) ? fallbackRepo(repoID) : activeGraph.repo)}
        pr={isPRGraph(activeGraph) ? activeGraph.pr : undefined}
        prs={prs}
        loadingPRNumber={loadingPRNumber}
        onSelectPR={onSelectPR}
        onBackToRepo={onBackToRepo}
        token={token}
        nodeCodeCache={nodeCodeCache}
        onCacheNodeCode={onCacheNodeCode}
        width={panelWidth}
        minWidth={PANEL_MIN_WIDTH}
        maxWidth={PANEL_MAX_WIDTH}
        onResize={onResizePanel}
        mode={panelMode}
        onModeChange={setPanelMode}
        onViewCode={() => {
          setPanelMode("code");
        }}
      />

      <div style={{ flex: 1, background: "#EBE9E9", position: "relative" }}>
        <div style={{ position: "absolute", inset: 0 }}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeClick={onNodeClick}
            onPaneClick={onPaneClick}
            connectionMode={ConnectionMode.Loose}
            fitView
            panOnScroll
            panOnScrollMode={PanOnScrollMode.Free}
            minZoom={0.15}
            maxZoom={2}
            proOptions={{ hideAttribution: true }}
          >
            <Background color="#D8D6D6" gap={20} size={1} />
          </ReactFlow>

          <div style={{
            position: "absolute",
            bottom: 24,
            right: 24,
            display: "flex",
            flexDirection: "column",
            gap: 4,
            zIndex: 10,
          }}>
            <ZoomControls onFit={() => fitView({ padding: 0.15 })} />
          </div>

          {totalNodes >= maxNodes && (
            <div style={{
              position: "absolute",
              bottom: 24,
              left: 24,
              color: "#888888",
              fontSize: 12,
              zIndex: 10,
            }}>
              Showing {maxNodes} of {totalNodes} affected functions
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function ZoomControls({ onFit }: { onFit: () => void }) {
  const { zoomIn, zoomOut } = useReactFlow();
  const btnStyle: React.CSSProperties = {
    width: 36, height: 36,
    background: "#FFFFFF",
    border: "1px solid #E4E4E4",
    borderRadius: 6,
    cursor: "pointer",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: 18,
    color: "#444444",
    lineHeight: 1,
  };
  return (
    <>
      <button style={btnStyle} onClick={() => zoomIn()}>+</button>
      <button style={btnStyle} onClick={() => zoomOut()}>−</button>
      <button style={{ ...btnStyle, fontSize: 12 }} onClick={onFit}>⊡</button>
    </>
  );
}

export default function GraphCanvas({
  graph,
  repoID,
  token,
  repo,
  prs,
}: {
  graph: UnifiedGraph;
  repoID: string;
  token: string;
  repo?: Repository;
  prs?: QueuePR[];
}) {
  return (
    <ReactFlowProvider>
      <InnerCanvas graph={graph} repoID={repoID} token={token} repo={repo} prs={prs} />
    </ReactFlowProvider>
  );
}

function fallbackRepo(repoID: string): Repository {
  return {
    id: repoID,
    user_id: "",
    installation_id: "",
    github_repo_id: 0,
    full_name: "",
    default_branch: "main",
    index_status: "ready",
    is_active: true,
    created_at: "",
  };
}
