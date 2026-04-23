"use client";

import { useCallback, useState, useEffect } from "react";
import {
  ReactFlow,
  Node,
  Edge,
  Background,
  useNodesState,
  useEdgesState,
  useReactFlow,
  ReactFlowProvider,
  NodeMouseHandler,
  PanOnScrollMode,
  ConnectionMode,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { GraphResponse, GraphNode as APIGraphNode } from "@/lib/types";
import GraphNodeComponent from "./graph-node";
import NodeDetailPanel from "./node-detail-panel";

const nodeTypes = { graphNode: GraphNodeComponent };

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
const NODE_W = 220;
const BASE_RADIUS = 300;
const MIN_SPACING = 220;

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

  const initialNodes: Node[] = graph.nodes.map((n) => ({
    id: n.id,
    type: "graphNode",
    data: { node: n },
    position: { x: 0, y: 0 },
  }));

  const initialEdges: Edge[] = graph.edges.map((e, idx) => {
    const src = graph.nodes.find((n) => n.id === e.caller_id);
    const color = src ? cardColorByKind(src.kind) : "#9CA3AF";
    return {
      id: `e${idx}`,
      source: e.caller_id,
      target: e.callee_id,
      type: "default",
      style: { stroke: color, strokeWidth: 1.5 },
      markerEnd: { type: MarkerType.ArrowClosed, width: 10, height: 10, color },
    };
  });

  const [nodes, , onNodesChange] = useNodesState(
    concentricLayout(initialNodes, initialEdges, graph.nodes)
  );
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);

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
        {/* Top bar */}
        <div style={{
          position: "absolute",
          top: 0, left: 0, right: 0,
          height: 48,
          background: "#E1E1E1",
          borderBottom: "1px solid #D4D4D4",
          display: "flex",
          alignItems: "center",
          padding: "0 20px",
          gap: 16,
          zIndex: 10,
        }}>
          <a href={`/repos/${repoID}`} style={{ color: "#888888", fontSize: 13, textDecoration: "none" }}>
            ← Back
          </a>
          <span style={{ color: "#AAAAAA" }}>·</span>
          <span style={{ color: "#888888", fontSize: 13 }}>#{graph.pr.number}</span>
          <span style={{ color: "#111111", fontSize: 14, fontWeight: 500 }}>{graph.pr.title}</span>
          <div style={{ flex: 1 }} />
          <a
            href={graph.pr.html_url}
            target="_blank"
            rel="noopener noreferrer"
            style={{ color: "#6366F1", fontSize: 13, textDecoration: "none" }}
          >
            View on GitHub →
          </a>
        </div>

        <div style={{ position: "absolute", inset: 0, top: 48 }}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
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
