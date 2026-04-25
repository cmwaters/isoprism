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
import { GraphResponse, GraphNode as APIGraphNode } from "@/lib/types";
import GraphNodeComponent from "./graph-node";
import NodeDetailPanel from "./node-detail-panel";

const nodeTypes = { graphNode: GraphNodeComponent };
const edgeTypes = { smartBezier: SmartBezierEdge };

type Point = { x: number; y: number };
type Rect = Point & { width: number; height: number };
type Segment = { a: Point; b: Point; normal: Point };
type MeasuredNode = {
  internals: { positionAbsolute: Point };
  measured: { width?: number; height?: number };
  width?: number;
  height?: number;
};

function nodeRect(node: MeasuredNode): Rect {
  return {
    x: node.internals.positionAbsolute.x,
    y: node.internals.positionAbsolute.y,
    width: node.measured.width ?? node.width ?? NODE_W,
    height: node.measured.height ?? node.height ?? 120,
  };
}

function rectCenter(rect: Rect): Point {
  return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
}

function rectSegments(rect: Rect): Segment[] {
  const x1 = rect.x;
  const x2 = rect.x + rect.width;
  const y1 = rect.y;
  const y2 = rect.y + rect.height;
  return [
    { a: { x: x1, y: y1 }, b: { x: x2, y: y1 }, normal: { x: 0, y: -1 } },
    { a: { x: x2, y: y1 }, b: { x: x2, y: y2 }, normal: { x: 1, y: 0 } },
    { a: { x: x2, y: y2 }, b: { x: x1, y: y2 }, normal: { x: 0, y: 1 } },
    { a: { x: x1, y: y2 }, b: { x: x1, y: y1 }, normal: { x: -1, y: 0 } },
  ];
}

function closestPointOnSegment(point: Point, segment: Segment): Point {
  const dx = segment.b.x - segment.a.x;
  const dy = segment.b.y - segment.a.y;
  const lengthSq = dx * dx + dy * dy;
  if (lengthSq === 0) return segment.a;

  const t = Math.max(0, Math.min(1, ((point.x - segment.a.x) * dx + (point.y - segment.a.y) * dy) / lengthSq));
  return { x: segment.a.x + t * dx, y: segment.a.y + t * dy };
}

function distanceSq(a: Point, b: Point): number {
  const dx = a.x - b.x;
  const dy = a.y - b.y;
  return dx * dx + dy * dy;
}

function isVertical(segment: Segment): boolean {
  return segment.a.x === segment.b.x;
}

function rangeOverlap(a1: number, a2: number, b1: number, b2: number): [number, number] | null {
  const start = Math.max(Math.min(a1, a2), Math.min(b1, b2));
  const end = Math.min(Math.max(a1, a2), Math.max(b1, b2));
  return start <= end ? [start, end] : null;
}

function closestPointsBetweenSegments(source: Segment, target: Segment): { source: Point; target: Point } {
  const sourceVertical = isVertical(source);
  const targetVertical = isVertical(target);

  if (sourceVertical && targetVertical) {
    const overlap = rangeOverlap(source.a.y, source.b.y, target.a.y, target.b.y);
    if (overlap) {
      const y = (overlap[0] + overlap[1]) / 2;
      return { source: { x: source.a.x, y }, target: { x: target.a.x, y } };
    }
  }

  if (!sourceVertical && !targetVertical) {
    const overlap = rangeOverlap(source.a.x, source.b.x, target.a.x, target.b.x);
    if (overlap) {
      const x = (overlap[0] + overlap[1]) / 2;
      return { source: { x, y: source.a.y }, target: { x, y: target.a.y } };
    }
  }

  const vertical = sourceVertical ? source : target;
  const horizontal = sourceVertical ? target : source;
  const xOverlap = rangeOverlap(vertical.a.x, vertical.b.x, horizontal.a.x, horizontal.b.x);
  const yOverlap = rangeOverlap(vertical.a.y, vertical.b.y, horizontal.a.y, horizontal.b.y);
  if (xOverlap && yOverlap) {
    const point = { x: vertical.a.x, y: horizontal.a.y };
    return { source: point, target: point };
  }

  const candidates = [
    { source: source.a, target: closestPointOnSegment(source.a, target) },
    { source: source.b, target: closestPointOnSegment(source.b, target) },
    { source: closestPointOnSegment(target.a, source), target: target.a },
    { source: closestPointOnSegment(target.b, source), target: target.b },
  ];

  return candidates.reduce((best, candidate) => (
    distanceSq(candidate.source, candidate.target) < distanceSq(best.source, best.target) ? candidate : best
  ));
}

function unitVector(from: Point, to: Point): Point {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const distance = Math.hypot(dx, dy);
  return distance === 0 ? { x: 1, y: 0 } : { x: dx / distance, y: dy / distance };
}

function closestBorderConnection(sourceRect: Rect, targetRect: Rect) {
  const sourceCenter = rectCenter(sourceRect);
  const targetCenter = rectCenter(targetRect);
  const centerDirection = unitVector(sourceCenter, targetCenter);

  let best:
    | { source: Point; target: Point; sourceNormal: Point; targetNormal: Point; distance: number; alignment: number }
    | null = null;

  for (const sourceSegment of rectSegments(sourceRect)) {
    for (const targetSegment of rectSegments(targetRect)) {
      const points = closestPointsBetweenSegments(sourceSegment, targetSegment);
      const distance = distanceSq(points.source, points.target);
      const alignment =
        sourceSegment.normal.x * centerDirection.x +
        sourceSegment.normal.y * centerDirection.y -
        targetSegment.normal.x * centerDirection.x -
        targetSegment.normal.y * centerDirection.y;

      if (!best || distance < best.distance - 0.01 || (Math.abs(distance - best.distance) <= 0.01 && alignment > best.alignment)) {
        best = {
          source: points.source,
          target: points.target,
          sourceNormal: sourceSegment.normal,
          targetNormal: targetSegment.normal,
          distance,
          alignment,
        };
      }
    }
  }

  return best!;
}

function smartBezierPath(connection: ReturnType<typeof closestBorderConnection>): string {
  const distance = Math.sqrt(connection.distance);
  const controlDistance = Math.max(36, Math.min(180, distance * 0.36));
  const sourceControl = {
    x: connection.source.x + connection.sourceNormal.x * controlDistance,
    y: connection.source.y + connection.sourceNormal.y * controlDistance,
  };
  const targetControl = {
    x: connection.target.x + connection.targetNormal.x * controlDistance,
    y: connection.target.y + connection.targetNormal.y * controlDistance,
  };

  return `M ${connection.source.x},${connection.source.y} C ${sourceControl.x},${sourceControl.y} ${targetControl.x},${targetControl.y} ${connection.target.x},${connection.target.y}`;
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

  const path = sourceNode && targetNode
    ? smartBezierPath(closestBorderConnection(nodeRect(sourceNode), nodeRect(targetNode)))
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
function InnerCanvas({ graph, repoID }: { graph: GraphResponse; repoID: string }) {
  const { fitView } = useReactFlow();
  const [selectedNode, setSelectedNode] = useState<APIGraphNode | null>(null);

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

  const onPaneClick = useCallback(() => setSelectedNode(null), []);

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

export default function GraphCanvas({ graph, repoID }: { graph: GraphResponse; repoID: string }) {
  return (
    <ReactFlowProvider>
      <InnerCanvas graph={graph} repoID={repoID} />
    </ReactFlowProvider>
  );
}
