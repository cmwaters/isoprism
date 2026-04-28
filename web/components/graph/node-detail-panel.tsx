"use client";

import { GraphEdge, GraphNode, GraphPR, NodeCodeResponse, NodeCodeSegment, QueuePR, Repository } from "@/lib/types";
import { apiFetch } from "@/lib/api";
import type { PanelMode } from "./graph-canvas";
import { ArrowLeft, BookOpenText, Code2 } from "lucide-react";
import { useCallback, useEffect, useState, type CSSProperties, type PointerEvent as ReactPointerEvent } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

interface Props {
  node: GraphNode | null;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  repoID: string;
  repo: Repository;
  pr?: GraphPR;
  prs?: QueuePR[];
  token: string;
  nodeCodeCache: Record<string, NodeCodeResponse>;
  onCacheNodeCode: (nodeID: string, code: NodeCodeResponse) => void;
  width: number;
  minWidth: number;
  maxWidth: number;
  onResize: (width: number) => void;
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
  onViewCode: () => void;
}

export default function NodeDetailPanel({
  node,
  allNodes,
  edges,
  onSelectNode,
  repoID,
  repo,
  pr,
  prs,
  token,
  nodeCodeCache,
  onCacheNodeCode,
  width,
  minWidth,
  maxWidth,
  onResize,
  mode,
  onModeChange,
  onViewCode,
}: Props) {
  const startResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = width;

    const onMove = (moveEvent: PointerEvent) => {
      onResize(startWidth + moveEvent.clientX - startX);
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };

    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }, [onResize, width]);

  return (
    <div
      style={{
        width,
        minWidth,
        maxWidth,
        background: "#DCDCDC",
        borderRight: "1px solid #E4E4E4",
        height: "100vh",
        display: "flex",
        flexDirection: "column",
        position: "relative",
      }}
    >
      <div style={{ flex: 1, overflowY: "auto" }}>
        {!node || mode === "overview" ? (
          !node ? (
            pr ? (
              <PRSummaryPanel pr={pr} repo={repo} allNodes={allNodes} onSelectNode={onSelectNode} />
            ) : (
              <RepoSummaryPanel repo={repo} prs={prs ?? []} allNodes={allNodes} repoID={repoID} onSelectNode={onSelectNode} />
            )
        ) : (
        <NodeDetail
          node={node}
          allNodes={allNodes}
          edges={edges}
          onSelectNode={onSelectNode}
          onBackToOverview={() => {
            onSelectNode("");
            onModeChange("overview");
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
          prID={pr?.id}
          token={token}
          cachedCode={nodeCodeCache[node.id]}
          onCacheNodeCode={onCacheNodeCode}
          onBackToOverview={() => onModeChange("overview")}
          onBackToPR={() => {
            onSelectNode("");
            onModeChange("overview");
          }}
        />
        )}
      </div>
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize side panel"
        title="Resize side panel"
        onPointerDown={startResize}
        style={{
          position: "absolute",
          top: 0,
          right: -4,
          bottom: 0,
          width: 8,
          cursor: "col-resize",
          zIndex: 20,
        }}
      />
    </div>
  );
}

function RepoSummaryPanel({
  repo,
  prs,
  allNodes,
  repoID,
  onSelectNode,
}: {
  repo: Repository;
  prs: QueuePR[];
  allNodes: GraphNode[];
  repoID: string;
  onSelectNode: (id: string) => void;
}) {
  const entryNodes = allNodes.filter((n) => n.node_type === "entrypoint");
  const visibleNodes = entryNodes.length > 0 ? entryNodes : allNodes.slice(0, 12);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <p style={{ color: "#888888", fontSize: 12, margin: "0 0 6px 0", wordBreak: "break-all" }}>
        {repo.full_name}
      </p>
      <h1 style={{ color: "#111111", fontSize: 18, fontWeight: 600, margin: "0 0 8px 0" }}>
        Repository graph
      </h1>
      <p style={{ color: "#666666", fontSize: 13, lineHeight: 1.5, margin: "0 0 18px 0" }}>
        Browse indexed nodes, calls, and tests from {repo.default_branch}.
      </p>

      {prs.length > 0 && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
            Pull requests
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 8, marginBottom: 20 }}>
            {prs.map((pr) => (
              <a
                key={pr.id}
                href={`/${repo.full_name}/pull/${pr.number}`}
                style={{
                  background: "#FFFFFF",
                  border: "1px solid #D4D4D4",
                  borderRadius: 6,
                  color: "#111111",
                  display: "block",
                  padding: 12,
                  textDecoration: "none",
                }}
              >
                <span style={{ color: "#AAAAAA", fontSize: 12, marginRight: 6 }}>#{pr.number}</span>
                <span style={{ fontSize: 13, fontWeight: 600 }}>{pr.title}</span>
                {pr.summary && (
                  <span style={{ color: "#666666", display: "block", fontSize: 12, lineHeight: 1.45, marginTop: 6 }}>
                    {pr.summary}
                  </span>
                )}
              </a>
            ))}
          </div>
        </>
      )}

      {visibleNodes.length > 0 && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
            Nodes
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            {visibleNodes.map((n) => (
              <button
                key={n.id}
                onClick={() => onSelectNode(n.id)}
                style={{
                  background: "#F0F0F0", border: "none", borderRadius: 4,
                  padding: "4px 8px", cursor: "pointer", textAlign: "left",
                }}
              >
                <span style={{ fontSize: 13, color: "#222222" }}>{n.name}</span>
              </button>
            ))}
          </div>
        </>
      )}

      {prs.length === 0 && allNodes.length === 0 && (
        <div style={{ color: "#888888", fontSize: 13, textAlign: "center", padding: "48px 0" }}>
          No graph data yet.
        </div>
      )}
    </div>
  );
}

function PRSummaryPanel({
  pr,
  repo,
  allNodes,
  onSelectNode,
}: {
  pr: GraphPR;
  repo: Repository;
  allNodes: GraphNode[];
  onSelectNode: (id: string) => void;
}) {
  const changedNodes = allNodes.filter((n) => n.node_type === "changed");
  const totalAdded = changedNodes.reduce((s, n) => s + (n.lines_added || 0), 0);
  const totalRemoved = changedNodes.reduce((s, n) => s + (n.lines_removed || 0), 0);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelToolbar backHref={`/${repo.full_name}`} />

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
  onViewCode: () => void;
}) {
  const pkgPrefix = pkgLabel(node);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelToolbar mode={mode} onModeChange={onModeChange} backOnClick={onBackToOverview} />

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
              onClick={onViewCode}
              style={{ background: "none", border: "none", color: "#166534", fontSize: 12, cursor: "pointer", padding: 0, marginTop: 8 }}
            >
              View code →
            </button>
          )}
        </div>
      )}

      <button
        onClick={onViewCode}
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
        Open code view →
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

      <TestSection tests={node.tests ?? []} />
    </div>
  );
}

function CodePanel({
  node,
  repoID,
  prID,
  token,
  cachedCode,
  onCacheNodeCode,
  onBackToOverview,
  onBackToPR,
}: {
  node: GraphNode;
  repoID: string;
  prID?: string;
  token: string;
  cachedCode?: NodeCodeResponse;
  onCacheNodeCode: (nodeID: string, code: NodeCodeResponse) => void;
  onBackToOverview: () => void;
  onBackToPR: () => void;
}) {
  const [error, setError] = useState<{ nodeID: string; message: string } | null>(null);
  const pkgPrefix = pkgLabel(node);

  useEffect(() => {
    if (cachedCode?.node_id === node.id) {
      return;
    }

    let cancelled = false;

    const path = prID
      ? `/api/v1/repos/${repoID}/prs/${prID}/nodes/${node.id}/code`
      : `/api/v1/repos/${repoID}/nodes/${node.id}/code`;

    apiFetch<NodeCodeResponse>(path, token)
      .then((response) => {
        if (!cancelled) onCacheNodeCode(node.id, response);
      })
      .catch((err: Error) => {
        if (!cancelled) setError({ nodeID: node.id, message: err.message });
      });

    return () => {
      cancelled = true;
    };
  }, [cachedCode?.node_id, node.id, onCacheNodeCode, prID, repoID, token]);

  const codeForNode = cachedCode?.node_id === node.id ? cachedCode : null;
  const errorForNode = error?.nodeID === node.id ? error.message : null;
  const loading = !codeForNode && !errorForNode;
  const canShowDiff = Boolean(node.diff_hunk || codeForNode?.diff_hunk);
  const plainSegment = codeForNode?.head ?? codeForNode?.base;
  const shouldShowDiff = Boolean(node.change_type && canShowDiff);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelToolbar
        mode="code"
        onModeChange={(nextMode) => {
          if (nextMode === "overview") onBackToOverview();
        }}
        backOnClick={onBackToPR}
      />

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

      {loading && (
        <p style={{ color: "#777777", fontSize: 13, marginTop: 16 }}>Loading code…</p>
      )}

      {errorForNode && (
        <div style={{ background: "#FEE2E2", borderRadius: 6, color: "#991B1B", fontSize: 12, lineHeight: 1.5, marginTop: 16, padding: 10 }}>
          {errorForNode}
        </div>
      )}

      {!loading && !errorForNode && shouldShowDiff && codeForNode && (codeForNode.base || codeForNode.head) && (
        <FullComponentDiffViewer base={codeForNode.base} head={codeForNode.head} />
      )}

      {!loading && !errorForNode && shouldShowDiff && (!codeForNode || (!codeForNode.base && !codeForNode.head)) && (
        <UnifiedDiffViewer patch={codeForNode?.diff_hunk ?? node.diff_hunk ?? ""} />
      )}

      {!loading && !errorForNode && !shouldShowDiff && plainSegment && (
        <SourceViewer segment={plainSegment} />
      )}

      {!loading && !errorForNode && !shouldShowDiff && !plainSegment && codeForNode?.diff_hunk && (
        <UnifiedDiffViewer patch={codeForNode.diff_hunk} />
      )}
    </div>
  );
}

function PanelToolbar({
  mode,
  onModeChange,
  backHref,
  backOnClick,
}: {
  mode?: PanelMode;
  onModeChange?: (mode: PanelMode) => void;
  backHref?: string;
  backOnClick?: () => void;
}) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, marginBottom: 16 }}>
      <BackControl href={backHref} onClick={backOnClick} />
      {mode && onModeChange ? (
        <ModeToggle mode={mode} onModeChange={onModeChange} />
      ) : (
        <span />
      )}
    </div>
  );
}

function ModeToggle({
  mode,
  onModeChange,
}: {
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
}) {
  const buttonStyle = (active: boolean): CSSProperties => ({
    width: 30,
    height: 30,
    alignItems: "center",
    background: active ? "#111111" : "#CFCFCF",
    border: "none",
    borderRadius: 4,
    color: active ? "#FFFFFF" : "#333333",
    cursor: "pointer",
    display: "inline-flex",
    justifyContent: "center",
    padding: 0,
  });

  return (
    <div style={{ display: "inline-flex", gap: 4 }}>
      <button
        type="button"
        aria-label="Show semantic overview"
        title="Overview"
        onClick={() => onModeChange("overview")}
        style={buttonStyle(mode === "overview")}
      >
        <BookOpenText size={16} strokeWidth={2} />
      </button>
      <button
        type="button"
        aria-label="Show code"
        title="Code"
        onClick={() => onModeChange("code")}
        style={buttonStyle(mode === "code")}
      >
        <Code2 size={16} strokeWidth={2} />
      </button>
    </div>
  );
}

function SourceViewer({ segment }: { segment: NodeCodeSegment }) {
  const lines = segment.source.split("\n");

  return (
    <pre style={{
      background: "#DCDCDC",
      border: "none",
      borderRadius: 0,
      color: "#222222",
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 11,
      lineHeight: 1.55,
      margin: 0,
      overflow: "visible",
      padding: 0,
      whiteSpace: "pre-wrap",
      overflowWrap: "anywhere",
    }}>
      {lines.map((line, index) => (
        <span key={index} style={{ display: "grid", gridTemplateColumns: "42px 1fr", minWidth: 0 }}>
          <span style={{ color: "#999999", paddingRight: 10, textAlign: "right", userSelect: "none" }}>
            {segment.start_line + index}
          </span>
          <span style={{ paddingRight: 10, whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{line || " "}</span>
        </span>
      ))}
    </pre>
  );
}

type ComponentDiffLine = {
  kind: "context" | "added" | "removed";
  oldLine?: number;
  newLine?: number;
  text: string;
};

function FullComponentDiffViewer({
  base,
  head,
}: {
  base?: NodeCodeSegment;
  head?: NodeCodeSegment;
}) {
  const lines = buildFullComponentDiff(
    base?.source ?? "",
    head?.source ?? "",
    base?.start_line,
    head?.start_line
  );

  return (
    <div style={{
      background: "#DCDCDC",
      color: "#222222",
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 11,
      lineHeight: 1.55,
      margin: 0,
      overflow: "visible",
      padding: 0,
    }}>
      {lines.map((line, index) => {
        const background = line.kind === "added"
          ? "#A7E7A4"
          : line.kind === "removed"
            ? "#E58A8A"
            : "transparent";
        const prefix = line.kind === "added" ? "+" : line.kind === "removed" ? "-" : " ";

        return (
          <div
            key={index}
            style={{
              background,
              display: "grid",
              gridTemplateColumns: "42px 16px minmax(0, 1fr)",
              padding: "0 2px",
            }}
          >
            <span style={{ color: "#777777", paddingRight: 8, textAlign: "right", userSelect: "none" }}>
              {displayLineNumber(line) ?? ""}
            </span>
            <span style={{ userSelect: "none" }}>{prefix}</span>
            <span style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
              {line.text || " "}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function displayLineNumber(line: ComponentDiffLine): number | undefined {
  return line.kind === "removed" ? line.oldLine : line.newLine;
}

function buildFullComponentDiff(
  baseSource: string,
  headSource: string,
  baseStartLine = 1,
  headStartLine = 1
): ComponentDiffLine[] {
  const baseLines = baseSource ? baseSource.split("\n") : [];
  const headLines = headSource ? headSource.split("\n") : [];

  if (baseLines.length === 0) {
    return headLines.map((text, index) => ({ kind: "added", newLine: headStartLine + index, text }));
  }
  if (headLines.length === 0) {
    return baseLines.map((text, index) => ({ kind: "removed", oldLine: baseStartLine + index, text }));
  }

  const dp: number[][] = Array.from({ length: baseLines.length + 1 }, () =>
    Array(headLines.length + 1).fill(0)
  );

  for (let i = baseLines.length - 1; i >= 0; i--) {
    for (let j = headLines.length - 1; j >= 0; j--) {
      dp[i][j] = baseLines[i] === headLines[j]
        ? dp[i + 1][j + 1] + 1
        : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  const out: ComponentDiffLine[] = [];
  let i = 0;
  let j = 0;

  while (i < baseLines.length && j < headLines.length) {
    if (baseLines[i] === headLines[j]) {
      out.push({
        kind: "context",
        oldLine: baseStartLine + i,
        newLine: headStartLine + j,
        text: headLines[j],
      });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ kind: "removed", oldLine: baseStartLine + i, text: baseLines[i] });
      i++;
    } else {
      out.push({ kind: "added", newLine: headStartLine + j, text: headLines[j] });
      j++;
    }
  }

  while (i < baseLines.length) {
    out.push({ kind: "removed", oldLine: baseStartLine + i, text: baseLines[i] });
    i++;
  }
  while (j < headLines.length) {
    out.push({ kind: "added", newLine: headStartLine + j, text: headLines[j] });
    j++;
  }

  return out;
}

function UnifiedDiffViewer({ patch }: { patch: string }) {
  const lines = patch.split("\n");

  return (
    <pre style={{
      background: "#DCDCDC",
      border: "none",
      borderRadius: 0,
      color: "#222222",
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 11,
      lineHeight: 1.55,
      margin: 0,
      overflow: "visible",
      padding: 0,
      whiteSpace: "pre-wrap",
      overflowWrap: "anywhere",
    }}>
      {lines.map((line, index) => {
        const added = line.startsWith("+") && !line.startsWith("+++");
        const removed = line.startsWith("-") && !line.startsWith("---");
        const hunk = line.startsWith("@@");
        const background = added ? "#A7E7A4" : removed ? "#E58A8A" : hunk ? "#E8E8FF" : "transparent";
        const color = hunk ? "#4F46E5" : "#222222";

        return (
          <span key={index} style={{ background, color, display: "block", padding: "0 2px", whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
            {line || " "}
          </span>
        );
      })}
    </pre>
  );
}

const backControlStyle: CSSProperties = {
  background: "none",
  border: "none",
  borderRadius: 4,
  color: "#555555",
  padding: 0,
  cursor: "pointer",
  height: 30,
  alignItems: "center",
  display: "inline-flex",
  gap: 6,
  justifyContent: "center",
  textDecoration: "none",
  fontFamily: "inherit",
  fontSize: 13,
  fontWeight: 400,
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
      <a href={href} style={backControlStyle} aria-label="Back" title="Back">
        <ArrowLeft size={17} strokeWidth={2} />
        <span>Back</span>
      </a>
    );
  }

  return (
    <button type="button" onClick={onClick} style={backControlStyle} aria-label="Back" title="Back">
      <ArrowLeft size={17} strokeWidth={2} />
      <span>Back</span>
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

function TestSection({ tests }: { tests: GraphNode["tests"] }) {
  if (tests.length === 0) return null;

  return (
    <div style={{ marginTop: 20 }}>
      <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
        Tests
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {tests.map((test) => (
          <div
            key={`${test.file_path}:${test.line_start}:${test.full_name}`}
            style={{
              padding: "4px 0",
              textAlign: "left",
            }}
          >
            <div style={{ fontSize: 13, color: "#222222" }}>{test.name}</div>
            <div style={{ fontSize: 11, color: "#888888", marginTop: 2, wordBreak: "break-all" }}>
              {test.file_path}:{test.line_start}
            </div>
          </div>
        ))}
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
