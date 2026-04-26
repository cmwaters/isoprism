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
import { GraphResponse, GraphNode as APIGraphNode, NodeCodeResponse } from "@/lib/types";
import GraphNodeComponent from "./graph-node";
import NodeDetailPanel from "./node-detail-panel";

const nodeTypes = { graphNode: GraphNodeComponent };
const edgeTypes = { smartBezier: SmartBezierEdge };

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

// ── Concentric ring layout ────────────────────────────────────────────────────
const NODE_W = 280;
const BASE_RADIUS = 380;
const MIN_SPACING = 300;
const PANEL_MIN_WIDTH = 260;
const PANEL_MAX_WIDTH = 620;
const PANEL_DEFAULT_WIDTH = 320;

function concentricLayout(nodes: Node[], edges: Edge[], graphNodes: APIGraphNode[]): Node[] {
  const changedIDs = new Set(graphNodes.filter((n) => n.node_type === "changed").map((n) => n.id));

  const neighbors = new Map<string, string[]>();
  nodes.forEach((n) => neighbors.set(n.id, []));
  edges.forEach((e) => {
    neighbors.get(e.source)?.push(e.target);
    neighbors.get(e.target)?.push(e.source);
  });

  const levels = new Map<string, number>();
  const queue: string[] = [];

  if (changedIDs.size > 0) {
    changedIDs.forEach((id) => { levels.set(id, 0); queue.push(id); });
  } else {
    nodes.forEach((n) => { levels.set(n.id, 0); queue.push(n.id); });
  }

  let head = 0;
  while (head < queue.length) {
    const curr = queue[head++];
    const level = levels.get(curr)!;
    for (const nb of (neighbors.get(curr) ?? [])) {
      if (!levels.has(nb)) { levels.set(nb, level + 1); queue.push(nb); }
    }
  }

  const maxLevel = Math.max(0, ...Array.from(levels.values()));
  nodes.forEach((n) => { if (!levels.has(n.id)) levels.set(n.id, maxLevel + 1); });

  const byLevel = new Map<number, Node[]>();
  nodes.forEach((n) => {
    const lvl = levels.get(n.id) ?? 0;
    if (!byLevel.has(lvl)) byLevel.set(lvl, []);
    byLevel.get(lvl)!.push(n);
  });

  const positions = new Map<string, { x: number; y: number }>();

  const hasOuterRings = byLevel.size > 1;

  byLevel.forEach((levelNodes, level) => {
    const count = levelNodes.length;
    if (level === 0 && hasOuterRings && count <= 3) {
      // Few changed nodes with outer context: place in a tight row at centre
      const spacing = NODE_W + 60;
      const totalW = count * spacing - 60;
      const startX = -totalW / 2;
      levelNodes.forEach((n, i) => {
        positions.set(n.id, { x: startX + i * spacing, y: 0 });
      });
    } else {
      // All other cases (including sole ring): circle arrangement
      if (count === 1) {
        positions.set(levelNodes[0].id, { x: 0, y: 0 });
      } else {
        const minR = level === 0 ? 180 : level * BASE_RADIUS;
        const radius = Math.max(minR, (count * MIN_SPACING) / (2 * Math.PI));
        levelNodes.forEach((n, i) => {
          const angle = (2 * Math.PI * i) / count - Math.PI / 2;
          positions.set(n.id, {
            x: Math.cos(angle) * radius,
            y: Math.sin(angle) * radius,
          });
        });
      }
    }
  });

  return nodes.map((n) => ({
    ...n,
    position: positions.get(n.id) ?? { x: 0, y: 0 },
  }));
}

// ── Inner canvas ──────────────────────────────────────────────────────────────
function InnerCanvas({ graph, repoID, token }: { graph: GraphResponse; repoID: string; token: string }) {
  const { fitView } = useReactFlow();
  const [selectedNode, setSelectedNode] = useState<APIGraphNode | null>(null);
  const [panelMode, setPanelMode] = useState<PanelMode>("overview");
  const [panelWidth, setPanelWidth] = useState(PANEL_DEFAULT_WIDTH);
  const [nodeCodeCache, setNodeCodeCache] = useState<Record<string, NodeCodeResponse>>({});

  const initialNodes: Node[] = useMemo(() => graph.nodes.map((n) => ({
    id: n.id,
    type: "graphNode",
    data: { node: n },
    position: { x: 0, y: 0 },
  })), [graph.nodes]);

  const baseEdges: Edge[] = useMemo(() => graph.edges.map((e, idx) => ({
    id: `e${idx}`,
    source: e.caller_id,
    target: e.callee_id,
    type: "smartBezier",
  })), [graph.edges]);

  const [nodes, , onNodesChange] = useNodesState(
    concentricLayout(initialNodes, baseEdges, graph.nodes)
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
    setTimeout(() => fitView({ padding: 0.15 }), 50);
  }, [fitView]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_, node) => {
      const apiNode = graph.nodes.find((n) => n.id === node.id) ?? null;
      setSelectedNode(apiNode);
    },
    [graph.nodes]
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

  const totalNodes = graph.nodes.length;
  const maxNodes = 20;

  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw" }}>
      <NodeDetailPanel
        node={selectedNode}
        allNodes={graph.nodes}
        edges={graph.edges}
        onSelectNode={(id) => {
          const n = graph.nodes.find((n) => n.id === id) ?? null;
          setSelectedNode(n);
        }}
        repoID={repoID}
        pr={graph.pr}
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

export default function GraphCanvas({ graph, repoID, token }: { graph: GraphResponse; repoID: string; token: string }) {
  return (
    <ReactFlowProvider>
      <InnerCanvas graph={graph} repoID={repoID} token={token} />
    </ReactFlowProvider>
  );
}
