"use client";

import { GraphEdge, GraphNode, GraphPR } from "@/lib/types";
import DiffBlock from "./diff-block";
import { useState } from "react";

interface Props {
  node: GraphNode | null;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  repoID: string;
  pr: GraphPR;
}

export default function NodeDetailPanel({ node, allNodes, edges, onSelectNode, pr }: Props) {
  const [showDiff, setShowDiff] = useState(false);

  return (
    <div
      style={{
        width: 280,
        minWidth: 280,
        maxWidth: 280,
        background: "#DCDCDC",
        borderRight: "1px solid #E4E4E4",
        height: "100vh",
        overflowY: "auto",
        display: "flex",
        flexDirection: "column",
      }}
    >
      {!node ? (
        <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <p style={{ color: "#999999", fontSize: 14 }}>Select a node to inspect it.</p>
        </div>
      ) : (
        <NodeDetail
          node={node}
          allNodes={allNodes}
          edges={edges}
          onSelectNode={onSelectNode}
          showDiff={showDiff}
          onToggleDiff={() => setShowDiff((v) => !v)}
        />
      )}
    </div>
  );
}

function NodeDetail({
  node,
  allNodes,
  edges,
  onSelectNode,
  showDiff,
  onToggleDiff,
}: {
  node: GraphNode;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  showDiff: boolean;
  onToggleDiff: () => void;
}) {
  const pkgPrefix = pkgLabel(node);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      {/* File path */}
      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 8, wordBreak: "break-all" }}>
        {node.file_path}
      </p>

      {/* Package label */}
      {pkgPrefix && (
        <p style={{ fontSize: 11, color: "#EF5DA8", marginBottom: 4, fontWeight: 500 }}>
          {pkgPrefix}
        </p>
      )}

      {/* Function name */}
      <h2 style={{ fontSize: 22, fontWeight: 600, color: "#111111", margin: "0 0 12px 0" }}>
        {node.name}
      </h2>

      {/* Signature */}
      <pre style={{ fontSize: 11, color: "#555555", background: "#CFCFCF", borderRadius: 4, padding: "6px 8px", overflow: "auto", marginBottom: 12, whiteSpace: "pre-wrap", wordBreak: "break-all", fontFamily: "'JetBrains Mono', monospace" }}>
        {node.signature}
      </pre>

      {/* Description */}
      {node.summary && (
        <p style={{ fontSize: 14, color: "#555555", lineHeight: 1.6, marginBottom: node.change_summary ? 16 : 20 }}>
          {node.summary}
        </p>
      )}

      {/* What's Changed card */}
      {node.change_summary && (
        <div style={{
          background: "#F0FFF4",
          borderLeft: "3px solid #BBF7D0",
          borderRadius: 8,
          padding: 12,
          marginBottom: 16,
        }}>
          <p style={{ fontSize: 12, fontWeight: 600, color: "#166534", marginBottom: 6 }}>What&apos;s Changed?</p>
          <p style={{ fontSize: 13, color: "#333333", lineHeight: 1.6, margin: 0 }}>{node.change_summary}</p>

          {node.diff_hunk && (
            <button
              onClick={onToggleDiff}
              style={{ background: "none", border: "none", color: "#166534", fontSize: 12, cursor: "pointer", padding: 0, marginTop: 8, textDecoration: "underline" }}
            >
              {showDiff ? "Hide diff" : "Show diff"}
            </button>
          )}
          {showDiff && node.diff_hunk && (
            <div style={{ marginTop: 8 }}>
              <DiffBlock patch={node.diff_hunk} />
            </div>
          )}
        </div>
      )}

      {/* Calls section */}
      <RelationSection
        label="Calls"
        nodeIDs={calleesOf(node.id, edges)}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
      />

      {/* Is Called By section */}
      <RelationSection
        label="Is Called By"
        nodeIDs={callersOf(node.id, edges)}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
      />
    </div>
  );
}

function RelationSection({
  label,
  nodeIDs,
  allNodes,
  onSelectNode,
}: {
  label: string;
  nodeIDs: string[];
  allNodes: GraphNode[];
  onSelectNode: (id: string) => void;
}) {
  if (nodeIDs.length === 0) return null;
  const nodes = nodeIDs.map((id) => allNodes.find((n) => n.id === id)).filter(Boolean) as GraphNode[];

  return (
    <div style={{ marginTop: 20 }}>
      <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
        {label}
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {nodes.map((n) => {
          const pkg = pkgLabel(n);
          return (
            <button
              key={n.id}
              onClick={() => onSelectNode(n.id)}
              style={{
                background: "none",
                border: "none",
                padding: "4px 0",
                cursor: "pointer",
                textAlign: "left",
                display: "flex",
                alignItems: "center",
                gap: 8,
              }}
            >
              <div>
                {pkg && <span style={{ fontSize: 11, color: "#EF5DA8" }}>{pkg}.</span>}
                <span style={{ fontSize: 13, color: "#222222" }}>{n.name}</span>
              </div>
              {n.change_type === "added" && (
                <span style={{ background: "#DCFCE7", color: "#16A34A", borderRadius: 4, padding: "0 4px", fontSize: 10 }}>Added</span>
              )}
              {n.change_type === "deleted" && (
                <span style={{ background: "#FEE2E2", color: "#EF4444", borderRadius: 4, padding: "0 4px", fontSize: 10 }}>Deleted</span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function pkgLabel(node: GraphNode): string {
  const parts = node.file_path.split("/");
  const pkg = parts.length >= 2 ? parts[parts.length - 2] : "";
  if (node.kind === "method" || node.full_name.includes(".")) {
    const prefix = node.full_name.split(".").slice(0, -1).join(".");
    return pkg ? `${pkg}.${prefix}` : prefix;
  }
  return pkg;
}

function calleesOf(nodeID: string, edges: GraphEdge[]): string[] {
  return edges.filter((e) => e.caller_id === nodeID).map((e) => e.callee_id);
}

function callersOf(nodeID: string, edges: GraphEdge[]): string[] {
  return edges.filter((e) => e.callee_id === nodeID).map((e) => e.caller_id);
}
