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
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import dagre from "@dagrejs/dagre";
import { GraphResponse, GraphNode as APIGraphNode } from "@/lib/types";
import GraphNodeComponent from "./graph-node";
import NodeDetailPanel from "./node-detail-panel";

const nodeTypes = { graphNode: GraphNodeComponent };

// ── Package color table ───────────────────────────────────────────────────────
export function packageColor(fullName: string): string {
  const prefix = fullName.split(".")[0].toLowerCase();
  if (prefix.includes("types")) return "#3B82F6";
  if (prefix.includes("crypto")) return "#06B6D4";
  if (prefix.includes("consensus")) return "#EC4899";
  if (prefix.includes("p2p")) return "#F59E0B";
  if (prefix.includes("rpc")) return "#8B5CF6";
  return "#6B7280";
}

// ── Dagre layout ──────────────────────────────────────────────────────────────
const NODE_W = 240;
const NODE_H = 120;

function layoutNodes(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: "TB", ranksep: 80, nodesep: 40 });

  nodes.forEach((n) => g.setNode(n.id, { width: NODE_W, height: NODE_H }));
  edges.forEach((e) => g.setEdge(e.source, e.target));
  dagre.layout(g);

  return nodes.map((n) => {
    const pos = g.node(n.id);
    return { ...n, position: { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 } };
  });
}

// ── Inner canvas (needs ReactFlowProvider context) ────────────────────────────
function InnerCanvas({ graph, repoID }: { graph: GraphResponse; repoID: string }) {
  const { fitView } = useReactFlow();
  const [selectedNode, setSelectedNode] = useState<APIGraphNode | null>(null);

  // Build React Flow nodes
  const initialNodes: Node[] = graph.nodes.map((n) => ({
    id: n.id,
    type: "graphNode",
    data: { node: n },
    position: { x: 0, y: 0 },
  }));

  // Build React Flow edges
  const nodeColorMap: Record<string, string> = {};
  graph.nodes.forEach((n) => {
    nodeColorMap[n.id] = packageColor(n.full_name);
  });

  const initialEdges: Edge[] = graph.edges.map((e, idx) => ({
    id: `e${idx}`,
    source: e.caller_id,
    target: e.callee_id,
    type: "bezier",
    style: { stroke: nodeColorMap[e.caller_id] ?? "#6B7280", strokeWidth: 1 },
    markerEnd: { type: "arrow" as any, width: 8, height: 8, color: nodeColorMap[e.caller_id] ?? "#6B7280" },
  }));

  const [nodes, setNodes, onNodesChange] = useNodesState(layoutNodes(initialNodes, initialEdges));
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);

  useEffect(() => {
    setTimeout(() => fitView({ padding: 0.2 }), 50);
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
      {/* Detail panel */}
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

      {/* Graph canvas */}
      <div style={{ flex: 1, background: "#EBE9E9", position: "relative" }}>
        {/* Top bar */}
        <div style={{
          position: "absolute",
          top: 0, left: 0, right: 0,
          height: 48,
          background: "#0A0A0A",
          borderBottom: "1px solid #242424",
          display: "flex",
          alignItems: "center",
          padding: "0 20px",
          gap: 16,
          zIndex: 10,
        }}>
          <a href={`/repos/${repoID}`} style={{ color: "#555555", fontSize: 13, textDecoration: "none" }}>
            ← Back
          </a>
          <span style={{ color: "#555555" }}>·</span>
          <span style={{ color: "#888888", fontSize: 13 }}>#{graph.pr.number}</span>
          <span style={{ color: "#F0F0F0", fontSize: 14, fontWeight: 500 }}>{graph.pr.title}</span>
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
            fitView
            panOnScroll
            panOnScrollMode={PanOnScrollMode.Free}
            minZoom={0.2}
            maxZoom={2}
            proOptions={{ hideAttribution: true }}
          >
            <Background color="#D8D6D6" gap={20} size={1} />
          </ReactFlow>

          {/* Zoom controls */}
          <div style={{
            position: "absolute",
            bottom: 24,
            right: 24,
            display: "flex",
            flexDirection: "column",
            gap: 4,
            zIndex: 10,
          }}>
            <ZoomControls onFit={() => fitView({ padding: 0.2 })} />
          </div>

          {/* Node count notice */}
          {totalNodes >= maxNodes && (
            <div style={{
              position: "absolute",
              bottom: 24,
              left: 24,
              color: "#AAAAAA",
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

// ── Public export ─────────────────────────────────────────────────────────────

export default function GraphCanvas({ graph, repoID }: { graph: GraphResponse; repoID: string }) {
  return (
    <ReactFlowProvider>
      <InnerCanvas graph={graph} repoID={repoID} />
    </ReactFlowProvider>
  );
}
