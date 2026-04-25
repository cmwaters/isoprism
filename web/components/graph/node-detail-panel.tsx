"use client";

import { GraphEdge, GraphNode, GraphPR, NodeCodeResponse, NodeCodeSegment } from "@/lib/types";
import { apiFetch } from "@/lib/api";
import type { CodeViewMode, PanelMode } from "./graph-canvas";
import { useEffect, useState, type CSSProperties } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

interface Props {
  node: GraphNode | null;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  repoID: string;
  pr: GraphPR;
  token: string;
  mode: PanelMode;
  codeViewMode: CodeViewMode;
  onModeChange: (mode: PanelMode) => void;
  onCodeViewModeChange: (mode: CodeViewMode) => void;
  onViewCode: (mode: CodeViewMode) => void;
}

export default function NodeDetailPanel({
  node,
  allNodes,
  edges,
  onSelectNode,
  repoID,
  pr,
  token,
  mode,
  codeViewMode,
  onModeChange,
  onCodeViewModeChange,
  onViewCode,
}: Props) {
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
      {!node || mode === "overview" ? (
        !node ? (
        <PRSummaryPanel pr={pr} allNodes={allNodes} repoID={repoID} onSelectNode={onSelectNode} />
        ) : (
        <NodeDetail
          node={node}
          allNodes={allNodes}
          edges={edges}
          onSelectNode={onSelectNode}
          onBackToOverview={() => {
            onSelectNode("");
            onModeChange("overview");
            onCodeViewModeChange("plain");
          }}
          mode={mode}
          onModeChange={onModeChange}
          onViewCode={onViewCode}
        />
        )
      ) : (
        <CodePanel
          node={node}
          repoID={repoID}
          prID={pr.id}
          token={token}
          viewMode={codeViewMode}
          onViewModeChange={onCodeViewModeChange}
          onBackToOverview={() => onModeChange("overview")}
          onBackToPR={() => {
            onSelectNode("");
            onModeChange("overview");
            onCodeViewModeChange("plain");
          }}
        />
      )}
    </div>
  );
}

function PRSummaryPanel({
  pr,
  allNodes,
  repoID,
  onSelectNode,
}: {
  pr: GraphPR;
  allNodes: GraphNode[];
  repoID: string;
  onSelectNode: (id: string) => void;
}) {
  const changedNodes = allNodes.filter((n) => n.node_type === "changed");
  const totalAdded = changedNodes.reduce((s, n) => s + (n.lines_added || 0), 0);
  const totalRemoved = changedNodes.reduce((s, n) => s + (n.lines_removed || 0), 0);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <BackControl href={`/repos/${repoID}`} />

      {/* PR number + title */}
      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 4 }}>#{pr.number}</p>
      <h2 style={{ fontSize: 15, fontWeight: 600, color: "#111111", margin: "0 0 12px 0", lineHeight: 1.4 }}>
        {pr.title}
      </h2>

      {/* Author + diff stats */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
        {pr.author_login && (
          <span style={{
            background: "#F0F0F0", border: "1px solid #D4D4D4",
            borderRadius: 12, padding: "2px 10px", fontSize: 11, color: "#555555",
          }}>
            {pr.author_login}
          </span>
        )}
        {totalAdded > 0 && (
          <span style={{ background: "#DCFCE7", color: "#16A34A", borderRadius: 12, padding: "2px 8px", fontSize: 11, fontWeight: 500 }}>
            +{totalAdded}
          </span>
        )}
        {totalRemoved > 0 && (
          <span style={{ background: "#FEE2E2", color: "#EF4444", borderRadius: 12, padding: "2px 8px", fontSize: 11, fontWeight: 500 }}>
            -{totalRemoved}
          </span>
        )}
      </div>

      {/* PR body (description) */}
      {pr.body && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
            Description
          </p>
          <div className="pr-description-markdown">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
              {pr.body}
            </ReactMarkdown>
          </div>
        </>
      )}

      {/* Changes list */}
      {changedNodes.length > 0 && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
            Changes
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 4, marginBottom: 20 }}>
            {changedNodes.map((n) => {
              const pkg = pkgLabel(n);
              return (
                <button
                  key={n.id}
                  onClick={() => onSelectNode(n.id)}
                  style={{
                    background: "#F0F0F0", border: "none", borderRadius: 4,
                    padding: "4px 8px", cursor: "pointer", textAlign: "left",
                    display: "flex", alignItems: "center", gap: 4,
                  }}
                >
                  {pkg && <span style={{ fontSize: 11, color: "#EF5DA8" }}>{pkg}.</span>}
                  <span style={{ fontSize: 13, color: "#222222" }}>{n.name}</span>
                </button>
              );
            })}
          </div>
        </>
      )}

      {/* View on GitHub */}
      <a
        href={pr.html_url}
        target="_blank"
        rel="noopener noreferrer"
        style={{ fontSize: 13, color: "#6366F1", textDecoration: "none" }}
      >
        View on GitHub →
      </a>
    </div>
  );
}

function NodeDetail({
  node,
  allNodes,
  edges,
  onSelectNode,
  onBackToOverview,
  mode,
  onModeChange,
  onViewCode,
}: {
  node: GraphNode;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  onBackToOverview: () => void;
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
  onViewCode: (mode: CodeViewMode) => void;
}) {
  const pkgPrefix = pkgLabel(node);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <BackControl onClick={onBackToOverview} />
      <ModeToggle mode={mode} onModeChange={onModeChange} canShowCode />

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
          {/* Header row: label + stat pills */}
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
            <p style={{ fontSize: 12, fontWeight: 600, color: "#166534", margin: 0 }}>What&apos;s Changed?</p>
            {node.lines_added > 0 && (
              <span style={{ background: "#DCFCE7", color: "#16A34A", borderRadius: 12, padding: "1px 7px", fontSize: 11, fontWeight: 500 }}>
                +{node.lines_added}
              </span>
            )}
            {node.lines_removed > 0 && (
              <span style={{ background: "#FEE2E2", color: "#EF4444", borderRadius: 12, padding: "1px 7px", fontSize: 11, fontWeight: 500 }}>
                -{node.lines_removed}
              </span>
            )}
          </div>
          <p style={{ fontSize: 13, color: "#333333", lineHeight: 1.6, margin: 0 }}>{node.change_summary}</p>

          {node.diff_hunk && (
            <button
              onClick={() => onViewCode("diff")}
              style={{ background: "none", border: "none", color: "#166534", fontSize: 12, cursor: "pointer", padding: 0, marginTop: 8 }}
            >
              View Diff →
            </button>
          )}
        </div>
      )}

      <button
        onClick={() => onViewCode(node.diff_hunk ? "diff" : "plain")}
        style={{
          background: "#CFCFCF",
          border: "none",
          borderRadius: 4,
          color: "#222222",
          cursor: "pointer",
          fontSize: 12,
          marginBottom: 4,
          padding: "8px 10px",
          textAlign: "left",
        }}
      >
        {node.diff_hunk ? "Open code/diff view →" : "Open code view →"}
      </button>

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

function CodePanel({
  node,
  repoID,
  prID,
  token,
  viewMode,
  onViewModeChange,
  onBackToOverview,
  onBackToPR,
}: {
  node: GraphNode;
  repoID: string;
  prID: string;
  token: string;
  viewMode: CodeViewMode;
  onViewModeChange: (mode: CodeViewMode) => void;
  onBackToOverview: () => void;
  onBackToPR: () => void;
}) {
  const [code, setCode] = useState<NodeCodeResponse | null>(null);
  const [error, setError] = useState<{ nodeID: string; message: string } | null>(null);
  const pkgPrefix = pkgLabel(node);

  useEffect(() => {
    let cancelled = false;

    apiFetch<NodeCodeResponse>(
      `/api/v1/repos/${repoID}/prs/${prID}/nodes/${node.id}/code`,
      token
    )
      .then((response) => {
        if (!cancelled) setCode(response);
      })
      .catch((err: Error) => {
        if (!cancelled) setError({ nodeID: node.id, message: err.message });
      });

    return () => {
      cancelled = true;
    };
  }, [node.id, prID, repoID, token]);

  const codeForNode = code?.node_id === node.id ? code : null;
  const errorForNode = error?.nodeID === node.id ? error.message : null;
  const loading = !codeForNode && !errorForNode;
  const canShowDiff = Boolean(node.diff_hunk || codeForNode?.diff_hunk);
  const plainSegment = codeForNode?.head ?? codeForNode?.base;

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <BackControl onClick={onBackToPR} />
      <ModeToggle mode="code" onModeChange={(mode) => {
        if (mode === "overview") onBackToOverview();
      }} canShowCode />

      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 8, wordBreak: "break-all" }}>
        {node.file_path}
      </p>
      {pkgPrefix && (
        <p style={{ fontSize: 11, color: "#EF5DA8", marginBottom: 4, fontWeight: 500 }}>
          {pkgPrefix}
        </p>
      )}
      <h2 style={{ fontSize: 20, fontWeight: 600, color: "#111111", margin: "0 0 12px 0" }}>
        {node.name}
      </h2>

      <CodeModeToggle viewMode={viewMode} onViewModeChange={onViewModeChange} canShowDiff={canShowDiff} />

      {loading && (
        <p style={{ color: "#777777", fontSize: 13, marginTop: 16 }}>Loading code…</p>
      )}

      {errorForNode && (
        <div style={{ background: "#FEE2E2", borderRadius: 6, color: "#991B1B", fontSize: 12, lineHeight: 1.5, marginTop: 16, padding: 10 }}>
          {errorForNode}
        </div>
      )}

      {!loading && !errorForNode && viewMode === "diff" && canShowDiff && (
        <UnifiedDiffViewer patch={codeForNode?.diff_hunk ?? node.diff_hunk ?? ""} />
      )}

      {!loading && !errorForNode && viewMode === "diff" && !canShowDiff && (
        <p style={{ color: "#777777", fontSize: 13, marginTop: 16 }}>No diff is available for this node.</p>
      )}

      {!loading && !errorForNode && viewMode === "plain" && plainSegment && (
        <SourceViewer segment={plainSegment} />
      )}

      {!loading && !errorForNode && viewMode === "plain" && !plainSegment && codeForNode?.diff_hunk && (
        <UnifiedDiffViewer patch={codeForNode.diff_hunk} />
      )}
    </div>
  );
}

function ModeToggle({
  mode,
  onModeChange,
  canShowCode,
}: {
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
  canShowCode: boolean;
}) {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 4, marginBottom: 16 }}>
      {(["overview", "code"] as PanelMode[]).map((item) => {
        const active = mode === item;
        return (
          <button
            key={item}
            type="button"
            disabled={item === "code" && !canShowCode}
            onClick={() => onModeChange(item)}
            style={{
              background: active ? "#111111" : "#CFCFCF",
              border: "none",
              borderRadius: 4,
              color: active ? "#FFFFFF" : "#333333",
              cursor: item === "code" && !canShowCode ? "not-allowed" : "pointer",
              fontSize: 12,
              padding: "7px 8px",
              textTransform: "capitalize",
            }}
          >
            {item}
          </button>
        );
      })}
    </div>
  );
}

function CodeModeToggle({
  viewMode,
  onViewModeChange,
  canShowDiff,
}: {
  viewMode: CodeViewMode;
  onViewModeChange: (mode: CodeViewMode) => void;
  canShowDiff: boolean;
}) {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 4, marginBottom: 12 }}>
      {(["plain", "diff"] as CodeViewMode[]).map((item) => {
        const active = viewMode === item;
        const disabled = item === "diff" && !canShowDiff;
        return (
          <button
            key={item}
            type="button"
            disabled={disabled}
            onClick={() => onViewModeChange(item)}
            style={{
              background: active ? "#111111" : "#CFCFCF",
              border: "none",
              borderRadius: 4,
              color: active ? "#FFFFFF" : "#333333",
              cursor: disabled ? "not-allowed" : "pointer",
              fontSize: 12,
              padding: "7px 8px",
              textTransform: "capitalize",
            }}
          >
            {item}
          </button>
        );
      })}
    </div>
  );
}

function SourceViewer({ segment }: { segment: NodeCodeSegment }) {
  const lines = segment.source.split("\n");

  return (
    <pre style={{
      background: "#F4F4F4",
      border: "1px solid #CFCFCF",
      borderRadius: 6,
      color: "#222222",
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 11,
      lineHeight: 1.55,
      margin: 0,
      overflow: "auto",
      padding: "10px 0",
      whiteSpace: "pre",
    }}>
      {lines.map((line, index) => (
        <span key={index} style={{ display: "grid", gridTemplateColumns: "42px 1fr", minWidth: 0 }}>
          <span style={{ color: "#999999", paddingRight: 10, textAlign: "right", userSelect: "none" }}>
            {segment.start_line + index}
          </span>
          <span style={{ paddingRight: 10 }}>{line || " "}</span>
        </span>
      ))}
    </pre>
  );
}

function UnifiedDiffViewer({ patch }: { patch: string }) {
  const lines = patch.split("\n");

  return (
    <pre style={{
      background: "#F4F4F4",
      border: "1px solid #CFCFCF",
      borderRadius: 6,
      color: "#222222",
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 11,
      lineHeight: 1.55,
      margin: 0,
      overflow: "auto",
      padding: "10px 0",
      whiteSpace: "pre",
    }}>
      {lines.map((line, index) => {
        const added = line.startsWith("+") && !line.startsWith("+++");
        const removed = line.startsWith("-") && !line.startsWith("---");
        const hunk = line.startsWith("@@");
        const background = added ? "#A7E7A4" : removed ? "#E58A8A" : hunk ? "#E8E8FF" : "transparent";
        const color = hunk ? "#4F46E5" : "#222222";

        return (
          <span key={index} style={{ background, color, display: "block", padding: "0 10px" }}>
            {line || " "}
          </span>
        );
      })}
    </pre>
  );
}

const backControlStyle: CSSProperties = {
  fontSize: 13,
  color: "#888888",
  textDecoration: "none",
  fontFamily: "inherit",
  fontWeight: 400,
  marginBottom: 16,
  display: "inline-block",
  background: "none",
  border: "none",
  padding: 0,
  cursor: "pointer",
  textAlign: "left",
};

function BackControl({
  href,
  onClick,
}: {
  href?: string;
  onClick?: () => void;
}) {
  if (href) {
    return (
      <a href={href} style={backControlStyle}>
        ← Back
      </a>
    );
  }

  return (
    <button type="button" onClick={onClick} style={backControlStyle} aria-label="Back to PR overview">
      ← Back
    </button>
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
                <span style={{ background: "#DCFCE7", color: "#16A34A", borderRadius: 4, padding: "0 5px", fontSize: 10, fontWeight: 500, whiteSpace: "nowrap" }}>
                  Added {n.lines_added > 0 ? `+${n.lines_added}` : ""}
                </span>
              )}
              {n.change_type === "deleted" && (
                <span style={{ background: "#FEE2E2", color: "#EF4444", borderRadius: 4, padding: "0 5px", fontSize: 10, fontWeight: 500 }}>
                  Deleted
                </span>
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
  const pkg = parts.length >= 2
    ? parts[parts.length - 2]
    : parts[0].replace(/\.[^.]+$/, "");
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

const markdownComponents: Components = {
  a: ({ children, href }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
};
