"use client";

import { useCallback, useMemo } from "react";
import {
  ReactFlow,
  Node,
  Edge,
  Background,
  ConnectionMode,
  MarkerType,
  PanOnScrollMode,
  ReactFlowProvider,
  useReactFlow,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { QueuePR, RepoGraphResponse } from "@/lib/types";
import PRQueue from "@/components/queue/pr-queue";
import { concentricLayout, edgeTypes, nodeTypes } from "./graph-canvas";

const PANEL_WIDTH = 360;

function InnerRepoGraphCanvas({
  graph,
  prs,
  repoID,
}: {
  graph: RepoGraphResponse;
  prs: QueuePR[];
  repoID: string;
}) {
  const { fitView } = useReactFlow();

  const initialNodes: Node[] = useMemo(() => graph.nodes.map((n) => ({
    id: n.id,
    type: "graphNode",
    data: { node: n },
    position: { x: 0, y: 0 },
  })), [graph.nodes]);

  const edges: Edge[] = useMemo(() => graph.edges.map((e, idx) => ({
    id: `repo-e${idx}`,
    source: e.caller_id,
    target: e.callee_id,
    type: "smartBezier",
    style: { stroke: "#888888", strokeWidth: 1 },
    markerEnd: { type: MarkerType.ArrowClosed, width: 10, height: 10, color: "#888888" },
  })), [graph.edges]);

  const [nodes, , onNodesChange] = useNodesState(
    concentricLayout(initialNodes, edges, graph.nodes)
  );

  const onEdgesChange = useCallback(() => {}, []);

  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw", background: "#EBE9E9" }}>
      <aside style={{
        width: PANEL_WIDTH,
        minWidth: PANEL_WIDTH,
        background: "#DCDCDC",
        borderRight: "1px solid #E4E4E4",
        height: "100vh",
        overflowY: "auto",
        padding: 20,
      }}>
        <p style={{ color: "#888888", fontSize: 12, margin: "0 0 6px 0", wordBreak: "break-all" }}>
          {graph.repo.full_name}
        </p>
        <h1 style={{ color: "#111111", fontSize: 18, fontWeight: 600, margin: "0 0 8px 0" }}>
          Pull requests
        </h1>
        <p style={{ color: "#666666", fontSize: 13, lineHeight: 1.5, margin: "0 0 18px 0" }}>
          Open PRs ranked by wait time and impact.
        </p>

        <PRQueue prs={prs} repoID={repoID} />

        {prs.length === 0 && (
          <div style={{ color: "#888888", fontSize: 13, textAlign: "center", padding: "48px 0" }}>
            No pull requests with graph data yet.
          </div>
        )}
      </aside>

      <main style={{ flex: 1, minWidth: 0, position: "relative" }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
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

        <button
          type="button"
          aria-label="Fit graph"
          title="Fit graph"
          onClick={() => fitView({ padding: 0.15 })}
          style={{
            position: "absolute",
            right: 24,
            bottom: 24,
            zIndex: 10,
            width: 36,
            height: 36,
            background: "#FFFFFF",
            border: "1px solid #E4E4E4",
            borderRadius: 6,
            cursor: "pointer",
            color: "#444444",
            fontSize: 12,
          }}
        >
          ⊡
        </button>
      </main>
    </div>
  );
}

export default function RepoGraphCanvas({
  graph,
  prs,
  repoID,
}: {
  graph: RepoGraphResponse;
  prs: QueuePR[];
  repoID: string;
}) {
  return (
    <ReactFlowProvider>
      <InnerRepoGraphCanvas graph={graph} prs={prs} repoID={repoID} />
    </ReactFlowProvider>
  );
}
