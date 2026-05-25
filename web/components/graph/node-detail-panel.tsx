"use client";

import Link from "next/link";
import { GitHubIssueDescription, GitHubIssueReference, GraphEdge, GraphNode, GraphPR, GraphProgram, NodeCodeResponse, NodeCodeSegment, PRFileDiff, QueuePR, Repository } from "@/lib/types";
import { apiFetch } from "@/lib/api";
import type { PanelMode } from "./graph-canvas";
import { ArrowLeft, BookOpenText, Code2, ListTree, Settings, X } from "lucide-react";
import { useCallback, useEffect, useState, type CSSProperties, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { renamedFromTitle, symbolContextLabel, symbolTitle } from "./symbol-format";

export type SelectedPRChange =
  | { type: "node"; nodeID: string }
  | { type: "file"; fileKey: string }
  | { type: "pr-description"; body: string; htmlURL: string }
  | { type: "issue"; issue: GitHubIssueReference };

interface Props {
  node: GraphNode | null;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  onSelectPRChange?: (change: SelectedPRChange) => void;
  repoID: string;
  repo: Repository;
  pr?: GraphPR;
  prFiles?: PRFileDiff[];
  testChanges?: GraphNode[];
  prs?: QueuePR[];
  programs?: GraphProgram[];
  loadingPRNumber?: number | null;
  loadingProgramID?: string | null;
  onSelectPR: (prNumber: number) => void;
  onSelectProgram: (programID: string) => void;
  onBackToRepo: () => void;
  token: string;
  nodeCodeCache: Record<string, NodeCodeResponse>;
  onCacheNodeCode: (nodeID: string, code: NodeCodeResponse) => void;
  issueDescriptionCache?: Record<string, GitHubIssueDescription>;
  onCacheIssueDescription?: (key: string, issue: GitHubIssueDescription) => void;
  width: number;
  minWidth: number;
  maxWidth: number;
  onResize: (width: number) => void;
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
  onViewCode: () => void;
  settingsHref?: string | null;
}

const settingsButtonStyle: CSSProperties = {
  width: "100%",
  height: 38,
  borderRadius: 6,
  border: "none",
  background: "transparent",
  color: "#111111",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "flex-start",
  gap: 8,
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 650,
  padding: "0 8px",
  textDecoration: "none",
};

export default function NodeDetailPanel({
  node,
  allNodes,
  edges,
  onSelectNode,
  onSelectPRChange,
  repoID,
  repo,
  pr,
  prFiles,
  testChanges,
  prs,
  programs,
  loadingPRNumber,
  loadingProgramID,
  onSelectPR,
  onSelectProgram,
  onBackToRepo,
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
  settingsHref,
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
      <div style={{ flex: 1, overflowY: "auto", paddingBottom: !pr ? 68 : 0 }}>
        {!node ? (
          pr ? (
            <PRSummaryPanel
              pr={pr}
              repo={repo}
              files={prFiles ?? []}
              allNodes={allNodes}
              testChanges={testChanges ?? []}
              onSelectNode={onSelectNode}
              onSelectPRChange={onSelectPRChange}
              onBackToRepo={onBackToRepo}
            />
          ) : (
            <RepoSummaryPanel
              repo={repo}
              prs={prs ?? []}
              programs={programs ?? []}
              allNodes={allNodes}
              loadingPRNumber={loadingPRNumber}
              loadingProgramID={loadingProgramID}
              onSelectPR={onSelectPR}
              onSelectProgram={onSelectProgram}
            />
          )
        ) : mode === "code" ? (
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
            repoID={repoID}
            prID={pr?.id}
            token={token}
            nodeCodeCache={nodeCodeCache}
            onCacheNodeCode={onCacheNodeCode}
          />
        )}
      </div>
      {!pr && settingsHref && (
        <div
          style={{
            position: "absolute",
            left: 20,
            right: 20,
            bottom: 52,
            zIndex: 30,
          }}
        >
          <Link href={settingsHref} prefetch style={settingsButtonStyle}>
            <Settings size={16} />
            <span>Settings</span>
          </Link>
        </div>
      )}
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
  programs,
  allNodes,
  loadingPRNumber,
  loadingProgramID,
  onSelectPR,
  onSelectProgram,
}: {
  repo: Repository;
  prs: QueuePR[];
  programs: GraphProgram[];
  allNodes: GraphNode[];
  loadingPRNumber?: number | null;
  loadingProgramID?: string | null;
  onSelectPR: (prNumber: number) => void;
  onSelectProgram: (programID: string) => void;
}) {
  const { owner, name } = splitRepoFullName(repo.full_name);
  const openTimeLabels = useOpenTimeLabels(prs.map((pr) => [pr.id, pr.opened_at]));
  const hasVisibleGraph = allNodes.length > 0;

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <p style={{ color: "#888888", fontSize: 12, margin: "0 0 6px 0", wordBreak: "break-all" }}>
        {owner}
      </p>
      <h1 style={{ color: "#111111", fontSize: 18, fontWeight: 600, margin: "0 0 8px 0" }}>
        {name}
      </h1>

      {prs.length > 0 && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8, marginTop: 20 }}>
            Pull requests
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 8, marginBottom: 20 }}>
            {prs.map((pr) => (
              <button
                key={pr.id}
                type="button"
                onClick={() => onSelectPR(pr.number)}
                disabled={loadingPRNumber === pr.number}
                style={{
                  background: "#FFFFFF",
                  border: "1px solid #D4D4D4",
                  borderRadius: 6,
                  color: "#111111",
                  display: "block",
                  cursor: loadingPRNumber === pr.number ? "wait" : "pointer",
                  fontFamily: "inherit",
                  padding: 12,
                  textAlign: "left",
                  width: "100%",
                }}
              >
                <span style={{ color: "#EF5DA8", fontSize: 12, fontWeight: 600, marginRight: 6 }}>#{pr.number}</span>
                <span style={{ fontSize: 13, fontWeight: 600 }}>{pr.title}</span>
                {loadingPRNumber === pr.number && (
                  <span style={{ color: "#888888", display: "block", fontSize: 12, marginTop: 6 }}>
                    Loading graph...
                  </span>
                )}
                <span style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: 6, marginTop: 8 }}>
                  {pr.author_login && (
                    <span style={prAuthorPillStyle}>
                      {pr.author_login}
                    </span>
                  )}
                  {pr.additions > 0 && (
                    <span style={additionPillStyle}>
                      +{pr.additions}
                    </span>
                  )}
                  {pr.deletions > 0 && (
                    <span style={deletionPillStyle}>
                      -{pr.deletions}
                    </span>
                  )}
                  <span style={repoPRBadgeStyle}>
                    {openTimeLabels.get(pr.id) || "open"}
                  </span>
                </span>
              </button>
            ))}
          </div>
        </>
      )}

      {programs.length > 0 && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8, marginTop: prs.length > 0 ? 0 : 20 }}>
            Programs
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 8, marginBottom: 20 }}>
            {programs.map((program) => (
              <button
                key={program.id}
                type="button"
                onClick={() => onSelectProgram(program.id)}
                disabled={loadingProgramID === program.id}
                style={{
                  background: "#FFFFFF",
                  border: "1px solid #D4D4D4",
                  borderRadius: 6,
                  color: "#111111",
                  cursor: loadingProgramID === program.id ? "wait" : "pointer",
                  display: "block",
                  fontFamily: "inherit",
                  padding: 12,
                  textAlign: "left",
                  width: "100%",
                }}
              >
                <span style={{ color: "#111111", display: "block", fontSize: 13, fontWeight: 600, overflowWrap: "anywhere" }}>
                  {programTitle(program)}
                </span>
                <span style={{ color: "#888888", display: "block", fontSize: 12, lineHeight: 1.4, marginTop: 4, overflowWrap: "anywhere" }}>
                  {programContextLabel(program)}
                </span>
                {loadingProgramID === program.id && (
                  <span style={{ color: "#888888", display: "block", fontSize: 12, marginTop: 6 }}>
                    Loading graph...
                  </span>
                )}
                <span style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: 6, marginTop: 8 }}>
                  <span style={repoPRBadgeStyle}>
                    {program.kind || "program"}
                  </span>
                  <span style={repoPRBadgeStyle}>
                    {program.file_path}:{program.line_start}
                  </span>
                </span>
              </button>
            ))}
          </div>
        </>
      )}

      {prs.length === 0 && programs.length === 0 && !hasVisibleGraph && (
        <div style={{ color: "#888888", fontSize: 13, textAlign: "center", padding: "48px 0" }}>
          There are no open pull requests or indexed programs yet.
        </div>
      )}
    </div>
  );
}

function splitRepoFullName(fullName: string): { owner: string; name: string } {
  const [owner, name] = fullName.split("/");
  return { owner: owner || fullName, name: name || fullName };
}

function programTitle(program: GraphProgram): string {
  const name = program.full_name.includes(":")
    ? program.full_name.slice(program.full_name.lastIndexOf(":") + 1)
    : program.full_name;
  const parts = name.split(".").filter(Boolean);
  return parts[parts.length - 1] || program.full_name;
}

function programContextLabel(program: GraphProgram): string {
  const packagePath = program.package_path || program.file_path.split("/").slice(0, -1).join("/");
  const packageParts = packagePath.split("/").filter(Boolean);
  return packageParts[packageParts.length - 1] || program.file_path;
}

function useOpenTimeLabels(prs: Array<[string, string]>): Map<string, string> {
  const [nowMs, setNowMs] = useState<number | null>(null);

  useEffect(() => {
    const updateNow = () => setNowMs(Date.now());
    const initialTimer = window.setTimeout(updateNow, 0);
    const intervalTimer = window.setInterval(updateNow, 60_000);
    return () => {
      window.clearTimeout(initialTimer);
      window.clearInterval(intervalTimer);
    };
  }, []);

  if (nowMs === null) return new Map();

  return new Map(prs.map(([id, openedAt]) => [id, formatOpenTime(openedAt, nowMs)]));
}

function formatOpenTime(openedAt: string, nowMs = Date.now()): string {
  const hoursOpen = Math.max(0, Math.floor((nowMs - new Date(openedAt).getTime()) / 3_600_000));
  if (hoursOpen < 24) return `${hoursOpen}h`;
  const daysOpen = Math.floor(hoursOpen / 24);
  if (daysOpen < 7) return `${daysOpen}d`;
  return `${Math.floor(daysOpen / 7)}w`;
}

const repoPRBadgeStyle: CSSProperties = {
  background: "#F0F0F0",
  border: "1px solid #D4D4D4",
  borderRadius: 4,
  color: "#666666",
  display: "inline-flex",
  fontSize: 11,
  lineHeight: 1.3,
  padding: "2px 6px",
};

const prAuthorPillStyle: CSSProperties = {
  background: "#F0F0F0",
  border: "1px solid #D4D4D4",
  borderRadius: 12,
  color: "#555555",
  fontSize: 11,
  padding: "2px 10px",
};

const additionPillStyle: CSSProperties = {
  background: "#DCFCE7",
  borderRadius: 12,
  color: "#16A34A",
  fontSize: 11,
  fontWeight: 500,
  padding: "2px 8px",
};

const deletionPillStyle: CSSProperties = {
  background: "#FEE2E2",
  borderRadius: 12,
  color: "#EF4444",
  fontSize: 11,
  fontWeight: 500,
  padding: "2px 8px",
};

const middlePanelTitleStyle: CSSProperties = {
  color: "#111111",
  fontSize: 15,
  fontWeight: 600,
  lineHeight: 1.4,
  margin: "0 0 12px 0",
};

const issuePillStyle: CSSProperties = {
  border: "1px solid #D4D4D4",
  borderRadius: 999,
  color: "#555555",
  display: "inline-flex",
  fontSize: 11,
  lineHeight: 1.3,
  padding: "2px 10px",
};

function issueStatePillStyle(state: string): CSSProperties {
  const closed = state.toLowerCase() === "closed";
  return {
    ...issuePillStyle,
    background: closed ? "#FEE2E2" : "#DCFCE7",
    borderColor: closed ? "#FECACA" : "#BBF7D0",
    color: closed ? "#EF4444" : "#16A34A",
    textTransform: "capitalize",
  };
}

const inlinePanelLinkStyle: CSSProperties = {
  background: "none",
  border: "none",
  color: "#6366F1",
  cursor: "pointer",
  fontFamily: "inherit",
  fontSize: 13,
  fontWeight: 600,
  padding: 0,
  textAlign: "left",
};

function findIssueReference(body: string, repoFullName: string): GitHubIssueReference | null {
  const trimmed = body.trim();
  if (!trimmed) return null;

  const [defaultOwner, defaultRepo] = repoFullName.split("/");
  const githubURL = trimmed.match(/https:\/\/github\.com\/([^/\s)]+)\/([^/\s)]+)\/issues\/(\d+)/i);
  if (githubURL) {
    return {
      owner: githubURL[1],
      repo: githubURL[2],
      number: Number(githubURL[3]),
    };
  }

  const qualified = trimmed.match(/(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)#(\d+)/i);
  if (qualified) {
    return {
      owner: qualified[1],
      repo: qualified[2],
      number: Number(qualified[3]),
    };
  }

  const sameRepo = trimmed.match(/(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?|issue)\s+#(\d+)/i);
  if (sameRepo && defaultOwner && defaultRepo) {
    return {
      owner: defaultOwner,
      repo: defaultRepo,
      number: Number(sameRepo[1]),
    };
  }

  return null;
}

export function issueReferenceKey(issue: GitHubIssueReference): string {
  return `${issue.owner}/${issue.repo}#${issue.number}`;
}

function PRSummaryPanel({
  pr,
  repo,
  files,
  allNodes,
  testChanges,
  onSelectNode,
  onSelectPRChange,
  onBackToRepo,
}: {
  pr: GraphPR;
  repo: Repository;
  files: PRFileDiff[];
  allNodes: GraphNode[];
  testChanges: GraphNode[];
  onSelectNode: (id: string) => void;
  onSelectPRChange?: (change: SelectedPRChange) => void;
  onBackToRepo: () => void;
}) {
  const changedNodes = allNodes.filter((n) => n.node_type === "changed" && !n.is_test);
  const descriptionText = (pr.summary || pr.body || "").trim();
  const hasAISummary = Boolean(pr.summary?.trim());
  const issue = findIssueReference(pr.body, repo.full_name);
  const testEntrypointChanges = testChanges.filter((node) => node.is_test && node.is_entrypoint);
  const totalAdded = files.reduce((s, file) => s + (file.additions || 0), 0);
  const totalRemoved = files.reduce((s, file) => s + (file.deletions || 0), 0);
  const graphFilePaths = new Set(changedNodes.map((node) => node.file_path));
  const testFilePaths = new Set(testChanges.map((node) => node.file_path));
  const documentationFiles = files.filter((file) => isMarkdownFile(file.filename) || Boolean(file.previous_filename && isMarkdownFile(file.previous_filename)));
  const documentationFilePaths = new Set(documentationFiles.map((file) => file.filename));
  const otherFiles = files.filter((file) => (
    !graphFilePaths.has(file.filename)
    && !testFilePaths.has(file.filename)
    && !documentationFilePaths.has(file.filename)
  ));

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelToolbar backOnClick={onBackToRepo} />

      {/* PR number + title */}
      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 4 }}>#{pr.number}</p>
      <h2 style={{ fontSize: 15, fontWeight: 600, color: "#111111", margin: "0 0 12px 0", lineHeight: 1.4 }}>
        {pr.title}
      </h2>

      {/* Author + diff stats */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
        {pr.author_login && (
          <span style={prAuthorPillStyle}>
            {pr.author_login}
          </span>
        )}
        {totalAdded > 0 && (
          <span style={additionPillStyle}>
            +{totalAdded}
          </span>
        )}
        {totalRemoved > 0 && (
          <span style={deletionPillStyle}>
            -{totalRemoved}
          </span>
        )}
      </div>

      {/* PR summary/description */}
      {descriptionText && (
        <>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
            Description
          </p>
          <div className="pr-description-markdown">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
              {descriptionText}
            </ReactMarkdown>
          </div>
          {hasAISummary && (
            <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginTop: 10, marginBottom: 18, width: "100%" }}>
              <button
                type="button"
                onClick={() => onSelectPRChange?.({ type: "pr-description", body: pr.body, htmlURL: pr.html_url })}
                style={inlinePanelLinkStyle}
              >
                View PR Description
              </button>
              {issue && (
                <button
                  type="button"
                  onClick={() => onSelectPRChange?.({ type: "issue", issue })}
                  style={{ ...inlinePanelLinkStyle, marginLeft: "auto" }}
                >
                  View Issue
                </button>
              )}
            </div>
          )}
        </>
      )}

      <ChangeSection
        title="Graph changes"
        emptyText="No graph function changes."
      >
        {changedNodes.map((node) => (
          <NodeChangeRow
            key={node.id}
            node={node}
            onClick={() => {
              onSelectNode(node.id);
              onSelectPRChange?.({ type: "node", nodeID: node.id });
            }}
          />
        ))}
      </ChangeSection>

      <ChangeSection
        title="Test changes"
        emptyText="No test function changes."
      >
        {testEntrypointChanges.map((node) => (
          <NodeChangeRow
            key={node.id}
            node={node}
            onClick={() => onSelectPRChange?.({ type: "node", nodeID: node.id })}
          />
        ))}
      </ChangeSection>

      <ChangeSection
        title="Documentation changes"
        emptyText="No documentation changes."
      >
        {documentationFiles.map((file) => (
          <FileChangeRow
            key={fileKey(file)}
            file={file}
            onClick={() => onSelectPRChange?.({ type: "file", fileKey: fileKey(file) })}
          />
        ))}
      </ChangeSection>

      {otherFiles.length > 0 && (
        <ChangeSection
          title="Other changes"
          emptyText="No other changes."
        >
          {otherFiles.map((file) => (
            <FileChangeRow
              key={fileKey(file)}
              file={file}
              onClick={() => onSelectPRChange?.({ type: "file", fileKey: fileKey(file) })}
            />
          ))}
        </ChangeSection>
      )}

      {/* Open on Github */}
      <a
        href={pr.html_url}
        target="_blank"
        rel="noopener noreferrer"
        style={{ fontSize: 13, color: "#6366F1", textDecoration: "none" }}
      >
        Open on Github →
      </a>
    </div>
  );
}

function ChangeSection({
  title,
  emptyText,
  children,
}: {
  title: string;
  emptyText: string;
  children: ReactNode;
}) {
  const childCount = Array.isArray(children) ? children.length : children ? 1 : 0;

  return (
    <section style={{ marginBottom: 20 }}>
      <div style={{ marginBottom: 8 }}>
        <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", margin: 0 }}>
          {title}
        </p>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {childCount > 0 ? children : (
          <div style={{ color: "#888888", fontSize: 12, padding: "6px 0" }}>
            {emptyText}
          </div>
        )}
      </div>
    </section>
  );
}

function NodeChangeRow({ node, onClick }: { node: GraphNode; onClick: () => void }) {
  const contextLabel = symbolContextLabel(node);

  return (
    <button type="button" onClick={onClick} style={changeRowButtonStyle}>
      <span style={{ minWidth: 0, flex: 1 }}>
        {contextLabel && (
          <span style={{ color: "#EF5DA8", display: "block", fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 500, lineHeight: 1.35, overflowWrap: "anywhere" }}>
            {contextLabel}
          </span>
        )}
        <span style={{ color: "#222222", display: "block", fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 600, lineHeight: 1.35, overflowWrap: "anywhere" }}>
          {symbolTitle(node)}
        </span>
        <span style={{ color: "#888888", display: "block", fontSize: 11, marginTop: 2, overflowWrap: "anywhere" }}>
          {node.file_path}:{node.line_start}
        </span>
      </span>
      <DiffPills node={node} compact alignRight />
    </button>
  );
}

function FileChangeRow({ file, onClick }: { file: PRFileDiff; onClick: () => void }) {
  const fileName = file.previous_filename
    ? `${fileTitle(file.previous_filename)} -> ${fileTitle(file.filename)}`
    : fileTitle(file.filename);
  const filePath = file.previous_filename
    ? `${file.previous_filename} -> ${file.filename}`
    : file.filename;

  return (
    <button type="button" onClick={onClick} style={changeRowButtonStyle}>
      <span style={{ minWidth: 0, flex: 1 }}>
        <span style={{ color: "#222222", display: "block", fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 600, lineHeight: 1.35, overflowWrap: "anywhere" }}>
          {fileName}
        </span>
        <span style={{ color: "#888888", display: "block", fontSize: 11, marginTop: 2, overflowWrap: "anywhere" }}>
          {filePath}
        </span>
      </span>
      <FileDiffPills file={file} />
    </button>
  );
}

function basename(path: string): string {
  return path.split("/").pop() || path;
}

function fileTitle(path: string): string {
  const name = basename(path).replace(/\.[^.]+$/, "");
  return name
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1).toLowerCase())
    .join(" ");
}

export function ComponentChangePanel({
  selectedChange,
  allNodes,
  edges,
  files,
  repoID,
  prID,
  token,
  nodeCodeCache,
  onCacheNodeCode,
  issueDescriptionCache,
  onCacheIssueDescription,
  onSelectNode,
  onClose,
  width,
  minWidth,
  maxWidth,
  onResize,
}: {
  selectedChange: SelectedPRChange;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  files: PRFileDiff[];
  repoID: string;
  prID?: string;
  token: string;
  nodeCodeCache: Record<string, NodeCodeResponse>;
  onCacheNodeCode: (nodeID: string, code: NodeCodeResponse) => void;
  issueDescriptionCache?: Record<string, GitHubIssueDescription>;
  onCacheIssueDescription?: (key: string, issue: GitHubIssueDescription) => void;
  onSelectNode: (id: string) => void;
  onClose: () => void;
  width: number;
  minWidth: number;
  maxWidth: number;
  onResize: (width: number) => void;
}) {
  const startResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = width;

    const onMove = (moveEvent: PointerEvent) => {
      onResize(Math.max(minWidth, Math.min(maxWidth, startWidth + moveEvent.clientX - startX)));
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };

    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }, [maxWidth, minWidth, onResize, width]);

  const node = selectedChange.type === "node"
    ? allNodes.find((candidate) => candidate.id === selectedChange.nodeID) ?? null
    : null;
  const file = selectedChange.type === "file"
    ? files.find((candidate) => fileKey(candidate) === selectedChange.fileKey) ?? null
    : null;

  return (
    <div
      style={{
        background: "#E5E5E5",
        borderRight: "1px solid #D4D4D4",
        height: "100vh",
        overflowY: "auto",
        position: "relative",
        width,
        flex: `0 0 ${width}px`,
      }}
    >
      {node ? (
        <NodeChangeDetailPanel
          node={node}
          allNodes={allNodes}
          edges={edges}
          onSelectNode={onSelectNode}
          repoID={repoID}
          prID={prID}
          token={token}
          cachedCode={nodeCodeCache[node.id]}
          onCacheNodeCode={onCacheNodeCode}
          onClose={onClose}
        />
      ) : file ? (
        <FileDiffPanel file={file} onClose={onClose} />
      ) : selectedChange.type === "pr-description" ? (
        <MarkdownDocumentPanel
          body={selectedChange.body}
          htmlURL={selectedChange.htmlURL}
          emptyText="No PR description."
          onClose={onClose}
        />
      ) : selectedChange.type === "issue" ? (
        <IssueDescriptionPanel
          repoID={repoID}
          prID={prID}
          token={token}
          issue={selectedChange.issue}
          cachedIssue={issueDescriptionCache?.[issueReferenceKey(selectedChange.issue)]}
          onCacheIssue={onCacheIssueDescription}
          onClose={onClose}
        />
      ) : (
        <div style={{ padding: 20 }}>
          <PanelCloseButton onClose={onClose} />
          <p style={{ color: "#777777", fontSize: 13 }}>Change not found.</p>
        </div>
      )}
      <div
        aria-hidden="true"
        onPointerDown={startResize}
        style={{
          cursor: "col-resize",
          position: "absolute",
          top: 0,
          bottom: 0,
          right: -4,
          width: 8,
          zIndex: 30,
        }}
      />
    </div>
  );
}

function NodeChangeDetailPanel({
  node,
  allNodes,
  edges,
  onSelectNode,
  repoID,
  prID,
  token,
  cachedCode,
  onCacheNodeCode,
  onClose,
}: {
  node: GraphNode;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  repoID: string;
  prID?: string;
  token: string;
  cachedCode?: NodeCodeResponse;
  onCacheNodeCode: (nodeID: string, code: NodeCodeResponse) => void;
  onClose: () => void;
}) {
  const [error, setError] = useState<{ nodeID: string; message: string } | null>(null);
  const pkgPrefix = pkgLabel(node);
  const typeNode = isTypeNode(node);

  useEffect(() => {
    if (typeNode) return;
    if (cachedCode?.node_id === node.id) return;

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
  }, [cachedCode?.node_id, node.id, onCacheNodeCode, prID, repoID, token, typeNode]);

  const codeForNode = cachedCode?.node_id === node.id ? cachedCode : null;
  const errorForNode = error?.nodeID === node.id ? error.message : null;
  const loading = !codeForNode && !errorForNode;
  const sourceSegment = codeForNode?.head ?? codeForNode?.base;
  const destinationEdges = outgoingCallEdges(node.id, edges);
  const destinationIDs = destinationEdges.map((edge) => edge.destination_id);
  const callSiteLines = buildCallSiteLines(sourceSegment, destinationIDs, allNodes);
  const methodEdges = outgoingOwnershipEdges(node.id, edges);
  const ownerEdges = incomingOwnershipEdges(node.id, edges);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelCloseButton onClose={onClose} />

      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 8, wordBreak: "break-all" }}>
        {node.file_path}
      </p>

      {pkgPrefix && (
        <p style={{ fontSize: 11, color: "#EF5DA8", marginBottom: 4, fontWeight: 500 }}>
          {pkgPrefix}
        </p>
      )}

      <h2 style={middlePanelTitleStyle}>
        {symbolTitle(node)}
      </h2>

      {node.change_type === "renamed" && (node.old_full_name || node.old_file_path) && (
        <div style={{ background: "#F5F5F5", border: "1px solid #D4D4D4", borderRadius: 6, color: "#555555", fontSize: 12, lineHeight: 1.5, marginBottom: 14, padding: 10 }}>
          {node.old_full_name && <div>Renamed from {renamedFromTitle(node)}</div>}
          {node.old_file_path && node.old_file_path !== node.file_path && <div>{`${node.old_file_path} -> ${node.file_path}`}</div>}
        </div>
      )}

      {node.summary && (
        <p style={{ fontSize: 14, color: "#555555", lineHeight: 1.6, marginBottom: node.change_summary ? 16 : 20 }}>
          {node.summary}
        </p>
      )}

      {node.change_summary && (
        <div style={{
          background: "#F0FFF4",
          borderLeft: "3px solid #BBF7D0",
          borderRadius: 8,
          padding: 12,
          marginBottom: 16,
        }}>
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
        </div>
      )}

      <RelationSection
        label="Calls"
        nodeIDs={destinationIDs}
        relationEdges={destinationEdges}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
        lineNumbers={callSiteLines}
      />

      <RelationSection
        label="Is Called By"
        nodeIDs={incomingCallEdges(node.id, edges).map((edge) => edge.source_id)}
        relationEdges={incomingCallEdges(node.id, edges)}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
      />

      {!typeNode && (
        <RelationSection
          label="Methods"
          nodeIDs={methodEdges.map((edge) => edge.destination_id)}
          relationEdges={methodEdges}
          allNodes={allNodes}
          onSelectNode={onSelectNode}
        />
      )}

      <RelationSection
        label="Receiver"
        nodeIDs={ownerEdges.map((edge) => edge.source_id)}
        relationEdges={ownerEdges}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
      />

      <TestSection tests={node.tests ?? []} />

      {typeNode ? (
        <TypeContentsSection node={node} allNodes={allNodes} edges={edges} onSelectNode={onSelectNode} />
      ) : (
        <div style={{ marginTop: 22 }}>
          <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", margin: "0 0 12px" }}>
            Code
          </p>
          <ComponentCodeBlock node={node} codeForNode={codeForNode} error={errorForNode} loading={loading} />
        </div>
      )}
    </div>
  );
}

function ComponentCodeBlock({
  node,
  codeForNode,
  error,
  loading,
}: {
  node: GraphNode;
  codeForNode: NodeCodeResponse | null;
  error: string | null;
  loading: boolean;
}) {
  const canShowDiff = Boolean(node.diff_hunk || codeForNode?.diff_hunk);
  const plainSegment = codeForNode?.head ?? codeForNode?.base;
  const shouldShowDiff = Boolean(node.change_type && canShowDiff);

  if (loading) {
    return <p style={{ color: "#777777", fontSize: 13, margin: 0 }}>Loading diff…</p>;
  }

  if (error) {
    return (
      <div style={{ background: "#FEE2E2", borderRadius: 6, color: "#991B1B", fontSize: 12, lineHeight: 1.5, padding: 10 }}>
        {error}
      </div>
    );
  }

  if (shouldShowDiff && codeForNode && shouldRenderFullComponentDiff(node.change_type, codeForNode)) {
    return <FullComponentDiffViewer base={codeForNode.base} head={codeForNode.head} />;
  }

  if (shouldShowDiff) {
    return <UnifiedDiffViewer patch={codeForNode?.diff_hunk ?? node.diff_hunk ?? ""} />;
  }

  if (!shouldShowDiff && plainSegment) {
    return <SourceViewer segment={plainSegment} />;
  }

  if (!shouldShowDiff && codeForNode?.diff_hunk) {
    return <UnifiedDiffViewer patch={codeForNode.diff_hunk} />;
  }

  return <div style={{ color: "#666666", fontSize: 12 }}>Diff unavailable</div>;
}

function FileDiffPanel({ file, onClose }: { file: PRFileDiff; onClose: () => void }) {
  const fileLabel = file.previous_filename
    ? `${file.previous_filename} -> ${file.filename}`
    : file.filename;

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelCloseButton onClose={onClose} />
      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 8, wordBreak: "break-all" }}>
        {file.filename}
      </p>
      <h2 style={{ color: "#111111", fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 600, lineHeight: 1.45, margin: "0 0 12px" }}>
        {fileLabel}
      </h2>
      <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 14 }}>
        <span style={{ ...repoPRBadgeStyle, textTransform: "capitalize" }}>{normalizedFileStatus(file.status)}</span>
        <FileDiffPills file={file} />
      </div>
      {file.patch ? (
        <UnifiedDiffViewer patch={file.patch} />
      ) : (
        <div style={{ color: "#666666", fontSize: 12 }}>
          Diff unavailable
        </div>
      )}
    </div>
  );
}

function MarkdownDocumentPanel({
  body,
  htmlURL,
  emptyText,
  onClose,
}: {
  body: string;
  htmlURL: string;
  emptyText: string;
  onClose: () => void;
}) {
  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelCloseButton onClose={onClose} />
      <h2 style={middlePanelTitleStyle}>
        PR Description
      </h2>
      {body.trim() ? (
        <div className="pr-description-markdown">
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
            {body}
          </ReactMarkdown>
        </div>
      ) : (
        <div style={{ color: "#666666", fontSize: 12 }}>
          {emptyText}
        </div>
      )}
      {htmlURL && (
        <a
          href={htmlURL}
          target="_blank"
          rel="noopener noreferrer"
          style={{ fontSize: 13, color: "#6366F1", marginTop: 16, textDecoration: "none" }}
        >
          Open on Github →
        </a>
      )}
    </div>
  );
}

function IssueDescriptionPanel({
  repoID,
  prID,
  token,
  issue,
  cachedIssue,
  onCacheIssue,
  onClose,
}: {
  repoID: string;
  prID?: string;
  token: string;
  issue: GitHubIssueReference;
  cachedIssue?: GitHubIssueDescription;
  onCacheIssue?: (key: string, issue: GitHubIssueDescription) => void;
  onClose: () => void;
}) {
  const issueKey = issueReferenceKey(issue);
  const [data, setData] = useState<GitHubIssueDescription | null>(cachedIssue ?? null);
  const [error, setError] = useState<{ key: string; message: string } | null>(() => (
    prID ? null : { key: issueKey, message: "Issue unavailable without a pull request." }
  ));

  useEffect(() => {
    if (!prID || cachedIssue) {
      return;
    }

    let cancelled = false;

    const params = new URLSearchParams({
      owner: issue.owner,
      repo: issue.repo,
      number: String(issue.number),
    });

    apiFetch<GitHubIssueDescription>(`/api/v1/repos/${repoID}/prs/${prID}/issue?${params.toString()}`, token)
      .then((response) => {
        if (!cancelled) {
          setData(response);
          onCacheIssue?.(issueKey, response);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) setError({ key: issueKey, message: err.message });
      });

    return () => {
      cancelled = true;
    };
  }, [cachedIssue, issue.number, issue.owner, issue.repo, issueKey, onCacheIssue, prID, repoID, token]);

  const matchingData = cachedIssue
    ?? (data?.owner === issue.owner && data.repo === issue.repo && data.number === issue.number ? data : null);
  const errorMessage = error?.key === issueKey ? error.message : null;

  if (errorMessage) {
    return (
      <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
        <PanelCloseButton onClose={onClose} />
        <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 8 }}>
          Issue #{issue.number}
        </p>
        <div style={{ background: "#FEE2E2", borderRadius: 6, color: "#991B1B", fontSize: 12, lineHeight: 1.5, padding: 10 }}>
          {errorMessage}
        </div>
      </div>
    );
  }

  if (!matchingData) {
    return (
      <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
        <PanelCloseButton onClose={onClose} />
        <p style={{ color: "#777777", fontSize: 13, margin: 0 }}>Loading issue...</p>
      </div>
    );
  }

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelCloseButton onClose={onClose} />
      <p style={{ fontSize: 11, color: "#AAAAAA", marginBottom: 8, wordBreak: "break-all" }}>
        {matchingData.owner}/{matchingData.repo} #{matchingData.number}
      </p>
      <h2 style={middlePanelTitleStyle}>
        {matchingData.title}
      </h2>
      <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 14 }}>
        <span style={issueStatePillStyle(matchingData.state)}>{matchingData.state}</span>
        {matchingData.author_login && <span style={issuePillStyle}>{matchingData.author_login}</span>}
      </div>
      {matchingData.body.trim() ? (
        <div className="pr-description-markdown">
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
            {matchingData.body}
          </ReactMarkdown>
        </div>
      ) : (
        <div style={{ color: "#666666", fontSize: 12 }}>No issue description.</div>
      )}
      <a
        href={matchingData.html_url}
        target="_blank"
        rel="noopener noreferrer"
        style={{ fontSize: 13, color: "#6366F1", marginTop: 16, textDecoration: "none" }}
      >
        Open on Github →
      </a>
    </div>
  );
}

function PanelCloseButton({ onClose }: { onClose: () => void }) {
  return (
    <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 16 }}>
      <button
        type="button"
        className="panel-close-button"
        aria-label="Close change panel"
        title="Close"
        onClick={onClose}
        style={{
          alignItems: "center",
          background: "transparent",
          border: "none",
          borderRadius: 4,
          color: "#333333",
          cursor: "pointer",
          display: "inline-flex",
          height: 28,
          justifyContent: "center",
          padding: 0,
          width: 28,
        }}
      >
        <X size={16} strokeWidth={2} />
      </button>
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
  repoID,
  prID,
  token,
  nodeCodeCache,
  onCacheNodeCode,
}: {
  node: GraphNode;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
  onBackToOverview: () => void;
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
  onViewCode: () => void;
  repoID: string;
  prID?: string;
  token: string;
  nodeCodeCache: Record<string, NodeCodeResponse>;
  onCacheNodeCode: (nodeID: string, code: NodeCodeResponse) => void;
}) {
  const pkgPrefix = pkgLabel(node);
  const cachedCode = nodeCodeCache[node.id];
  const typeNode = isTypeNode(node);
  const canViewCode = !typeNode;

  useEffect(() => {
    if (!canViewCode) return;
    if (cachedCode?.node_id === node.id) return;

    let cancelled = false;
    const path = prID
      ? `/api/v1/repos/${repoID}/prs/${prID}/nodes/${node.id}/code`
      : `/api/v1/repos/${repoID}/nodes/${node.id}/code`;

    apiFetch<NodeCodeResponse>(path, token)
      .then((response) => {
        if (!cancelled) onCacheNodeCode(node.id, response);
      })
      .catch(() => {
        // Call-site line numbers are helpful but not required for the overview panel.
      });

    return () => {
      cancelled = true;
    };
  }, [cachedCode?.node_id, canViewCode, node.id, onCacheNodeCode, prID, repoID, token]);

  const sourceSegment = cachedCode?.node_id === node.id
    ? cachedCode.head ?? cachedCode.base
    : undefined;
  const destinationEdges = outgoingCallEdges(node.id, edges);
  const destinationIDs = destinationEdges.map((edge) => edge.destination_id);
  const callSiteLines = buildCallSiteLines(sourceSegment, destinationIDs, allNodes);
  const methodEdges = outgoingOwnershipEdges(node.id, edges);
  const ownerEdges = incomingOwnershipEdges(node.id, edges);

  return (
    <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 0 }}>
      <PanelToolbar mode={mode} onModeChange={onModeChange} canViewCode={canViewCode} canViewContents={typeNode} backOnClick={onBackToOverview} />

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
      <h2 style={middlePanelTitleStyle}>
        {symbolTitle(node)}
      </h2>

      {node.change_type === "renamed" && (node.old_full_name || node.old_file_path) && (
        <div style={{ background: "#F5F5F5", border: "1px solid #D4D4D4", borderRadius: 6, color: "#555555", fontSize: 12, lineHeight: 1.5, marginBottom: 14, padding: 10 }}>
          {node.old_full_name && <div>Renamed from {renamedFromTitle(node)}</div>}
          {node.old_file_path && node.old_file_path !== node.file_path && <div>{`${node.old_file_path} -> ${node.file_path}`}</div>}
        </div>
      )}

      {mode === "contents" ? (
        <TypeContentsSection node={node} allNodes={allNodes} edges={edges} onSelectNode={onSelectNode} />
      ) : (
        <>
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

          {canViewCode && node.diff_hunk && (
            <button
              onClick={onViewCode}
              style={{ background: "none", border: "none", color: "#166534", fontSize: 12, cursor: "pointer", padding: 0, marginTop: 8 }}
            >
              View code →
            </button>
          )}
        </div>
      )}

      {canViewCode && (
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
      )}

      {/* Calls section */}
      <RelationSection
        label="Calls"
        nodeIDs={destinationIDs}
        relationEdges={destinationEdges}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
        lineNumbers={callSiteLines}
      />

      {/* Is Called By section */}
      <RelationSection
        label="Is Called By"
        nodeIDs={incomingCallEdges(node.id, edges).map((edge) => edge.source_id)}
        relationEdges={incomingCallEdges(node.id, edges)}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
      />

      {!typeNode && (
        <RelationSection
          label="Methods"
          nodeIDs={methodEdges.map((edge) => edge.destination_id)}
          relationEdges={methodEdges}
          allNodes={allNodes}
          onSelectNode={onSelectNode}
        />
      )}

      <RelationSection
        label="Receiver"
        nodeIDs={ownerEdges.map((edge) => edge.source_id)}
        relationEdges={ownerEdges}
        allNodes={allNodes}
        onSelectNode={onSelectNode}
      />

      <TestSection tests={node.tests ?? []} />
        </>
      )}
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
      <h2 style={middlePanelTitleStyle}>
        {symbolTitle(node)}
      </h2>

      {loading && (
        <p style={{ color: "#777777", fontSize: 13, marginTop: 16 }}>Loading code…</p>
      )}

      {errorForNode && (
        <div style={{ background: "#FEE2E2", borderRadius: 6, color: "#991B1B", fontSize: 12, lineHeight: 1.5, marginTop: 16, padding: 10 }}>
          {errorForNode}
        </div>
      )}

      {!loading && !errorForNode && shouldShowDiff && codeForNode && shouldRenderFullComponentDiff(node.change_type, codeForNode) && (
        <FullComponentDiffViewer base={codeForNode.base} head={codeForNode.head} />
      )}

      {!loading && !errorForNode && shouldShowDiff && (!codeForNode || !shouldRenderFullComponentDiff(node.change_type, codeForNode)) && (
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
  canViewCode = true,
  canViewContents = false,
  backHref,
  backOnClick,
}: {
  mode?: PanelMode;
  onModeChange?: (mode: PanelMode) => void;
  canViewCode?: boolean;
  canViewContents?: boolean;
  backHref?: string;
  backOnClick?: () => void;
}) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, marginBottom: 16 }}>
      <BackControl href={backHref} onClick={backOnClick} />
      {mode && onModeChange ? (
        <ModeToggle mode={mode} onModeChange={onModeChange} canViewCode={canViewCode} canViewContents={canViewContents} />
      ) : (
        <span />
      )}
    </div>
  );
}

function ModeToggle({
  mode,
  onModeChange,
  canViewCode = true,
  canViewContents = false,
}: {
  mode: PanelMode;
  onModeChange: (mode: PanelMode) => void;
  canViewCode?: boolean;
  canViewContents?: boolean;
}) {
  const buttonStyle = (active: boolean, disabled = false): CSSProperties => ({
    width: 30,
    height: 30,
    alignItems: "center",
    background: active ? "#111111" : "#CFCFCF",
    border: "none",
    borderRadius: 4,
    color: disabled ? "#999999" : active ? "#FFFFFF" : "#333333",
    cursor: disabled ? "not-allowed" : "pointer",
    display: "inline-flex",
    justifyContent: "center",
    opacity: disabled ? 0.5 : 1,
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
      {canViewCode && (
        <button
          type="button"
          aria-label="Show code"
          title="Code"
          onClick={() => onModeChange("code")}
          style={buttonStyle(mode === "code")}
        >
          <Code2 size={16} strokeWidth={2} />
        </button>
      )}
      {canViewContents && (
        <button
          type="button"
          aria-label="Show contents"
          title="Contents"
          onClick={() => onModeChange("contents")}
          style={buttonStyle(mode === "contents")}
        >
          <ListTree size={16} strokeWidth={2} />
        </button>
      )}
    </div>
  );
}

function SourceViewer({ segment }: { segment: NodeCodeSegment }) {
  const lines = segment.source.split("\n");

  return (
    <pre style={{
      background: "transparent",
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
          <span style={{ color: "#999999", paddingRight: 10, textAlign: "left", userSelect: "none" }}>
            {segment.start_line + index}
          </span>
          <span style={{ paddingRight: 10, whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{line || " "}</span>
        </span>
      ))}
    </pre>
  );
}

function TypeContentsSection({
  node,
  allNodes,
  edges,
  onSelectNode,
}: {
  node: GraphNode;
  allNodes: GraphNode[];
  edges: GraphEdge[];
  onSelectNode: (id: string) => void;
}) {
  const typeRefs = [...(node.inputs ?? []), ...(node.outputs ?? [])];
  const seen = new Set<string>();
  const methodRows = outgoingOwnershipEdges(node.id, edges).flatMap((edge) => {
    const target = allNodes.find((candidate) => candidate.id === edge.destination_id);
    if (!target || target.id === node.id || seen.has(target.id)) return [];
    seen.add(target.id);
    return [target];
  });
  const refRows = typeRefs.flatMap((ref) => {
    const key = ref.node_id ?? `${ref.type}:${ref.name ?? ""}`;
    if (!key || key === node.id || seen.has(key)) return [];
    seen.add(key);
    const target = ref.node_id ? allNodes.find((candidate) => candidate.id === ref.node_id) : undefined;
    if (target?.id === node.id) return [];
    return [{ ref, target }];
  });
  const typeUsageRows = outgoingTypeUsageEdges(node.id, edges).flatMap((edge) => {
    const target = allNodes.find((candidate) => candidate.id === edge.destination_id);
    if (!target || target.id === node.id || seen.has(target.id)) return [];
    seen.add(target.id);
    return [target];
  });
  const hasContents = methodRows.length > 0 || typeUsageRows.length > 0 || refRows.length > 0;

  return (
    <div style={{ marginTop: 20 }}>
      <p style={{ fontSize: 11, color: "#AAAAAA", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
        Contents
      </p>
      {!hasContents ? (
        <p style={{ color: "#777777", fontSize: 13, lineHeight: 1.5, margin: 0 }}>No methods or referenced types found.</p>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          {methodRows.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <p style={{ color: "#AAAAAA", fontSize: 11, letterSpacing: "0.08em", margin: "0 0 2px", textTransform: "uppercase" }}>Methods</p>
              {methodRows.map((target) => {
                const pkg = pkgLabel(target);
                return (
                  <button
                    key={target.id}
                    type="button"
                    onClick={() => onSelectNode(target.id)}
                    style={{
                      background: "none",
                      border: "none",
                      color: "#222222",
                      cursor: "pointer",
                      display: "grid",
                      gap: 8,
                      gridTemplateColumns: "34px 1fr",
                      padding: "4px 0",
                      textAlign: "left",
                      width: "100%",
                    }}
                  >
                    <span style={{ color: "#888888", fontSize: 11, userSelect: "none" }}>L{target.line_start}</span>
                    <span style={{ minWidth: 0 }}>
                      {pkg && <span style={{ fontSize: 11, color: "#EF5DA8", display: "block" }}>{pkg}</span>}
                      <span style={{ display: "block", fontSize: 13 }}>{symbolTitle(target)}</span>
                    </span>
                  </button>
                );
              })}
            </div>
          )}
          {typeUsageRows.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <p style={{ color: "#AAAAAA", fontSize: 11, letterSpacing: "0.08em", margin: "0 0 2px", textTransform: "uppercase" }}>Used Types</p>
              {typeUsageRows.map((target) => {
                const pkg = target ? pkgLabel(target) : undefined;
                return (
                  <button
                    key={target.id}
                    type="button"
                    onClick={() => onSelectNode(target.id)}
                    style={{
                      background: "none",
                      border: "none",
                      color: "#222222",
                      cursor: "pointer",
                      padding: "4px 0",
                      textAlign: "left",
                      width: "100%",
                    }}
                  >
                    {pkg && <span style={{ fontSize: 11, color: "#EF5DA8", display: "block" }}>{pkg}</span>}
                    <span style={{ display: "block", fontSize: 13 }}>{symbolTitle(target)}</span>
                  </button>
                );
              })}
            </div>
          )}
          {refRows.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <p style={{ color: "#AAAAAA", fontSize: 11, letterSpacing: "0.08em", margin: "0 0 2px", textTransform: "uppercase" }}>Referenced Types</p>
              {refRows.map(({ ref, target }) => {
                const label = target ? symbolTitle(target) : ref.name ?? ref.type;
                const pkg = target ? pkgLabel(target) : undefined;
                const clickableID = target?.id ?? ref.node_id;
                const disabled = !clickableID;
                return (
                  <button
                    key={ref.node_id ?? `${ref.type}:${ref.name ?? ""}`}
                    type="button"
                    onClick={() => {
                      if (clickableID) onSelectNode(clickableID);
                    }}
                    disabled={disabled}
                    style={{
                      background: "none",
                      border: "none",
                      color: disabled ? "#777777" : "#222222",
                      cursor: disabled ? "default" : "pointer",
                      padding: "4px 0",
                      textAlign: "left",
                      width: "100%",
                    }}
                  >
                    {pkg && <span style={{ fontSize: 11, color: "#EF5DA8", display: "block" }}>{pkg}</span>}
                    <span style={{ display: "block", fontSize: 13 }}>{label}</span>
                    {!target && ref.type && ref.type !== label && (
                      <span style={{ color: "#888888", display: "block", fontSize: 11, marginTop: 2 }}>{ref.type}</span>
                    )}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
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
      background: "transparent",
      color: "#222222",
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 11,
      lineHeight: 1.55,
      margin: 0,
      overflow: "visible",
      padding: 0,
    }}>
      {lines.map((line, index) => {
        const color = line.kind === "added"
          ? "#16A34A"
          : line.kind === "removed"
            ? "#EF4444"
            : "#222222";

        return (
          <div
            key={index}
            style={{
              display: "grid",
              gridTemplateColumns: "42px minmax(0, 1fr)",
              padding: "0 2px",
            }}
          >
            <span style={{ color: "#777777", paddingRight: 8, textAlign: "left", userSelect: "none" }}>
              {displayLineNumber(line) ?? ""}
            </span>
            <span style={{ color, whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
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

function shouldRenderFullComponentDiff(
  changeType: GraphNode["change_type"],
  codeForNode: NodeCodeResponse
): boolean {
  if (changeType === "added") {
    return Boolean(codeForNode.head);
  }
  if (changeType === "deleted") {
    return Boolean(codeForNode.base);
  }
  if (changeType === "modified" || changeType === "renamed") {
    return Boolean(codeForNode.base && codeForNode.head);
  }
  return Boolean(codeForNode.base || codeForNode.head);
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
      background: "transparent",
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
        const color = added ? "#16A34A" : removed ? "#EF4444" : hunk ? "#4F46E5" : "#222222";
        const text = added || removed ? line.slice(1) : line;

        return (
          <span key={index} style={{ color, display: "block", padding: "0 2px", whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
            {text || " "}
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
  relationEdges,
  allNodes,
  onSelectNode,
  lineNumbers,
}: {
  label: string;
  nodeIDs: string[];
  relationEdges?: GraphEdge[];
  allNodes: GraphNode[];
  onSelectNode: (id: string) => void;
  lineNumbers?: Record<string, number>;
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
          const relationEdge = relationEdges?.find((edge) => edge.source_id === n.id || edge.destination_id === n.id);
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
                width: "100%",
              }}
            >
              <span style={{ color: "#888888", flex: "0 0 34px", fontSize: 11, textAlign: "left", userSelect: "none" }}>
                L{lineNumbers?.[n.id] ?? n.line_start}
              </span>
              <div style={{ minWidth: 0, flex: 1 }}>
                {pkg && <span style={{ fontSize: 11, color: "#EF5DA8", display: "block" }}>{pkg}</span>}
                <span style={{ alignItems: "center", display: "flex", gap: 6 }}>
                  <span style={{ fontSize: 13, color: "#222222" }}>{symbolTitle(n)}</span>
                  {relationEdge?.change_type && (
                    <span style={{
                      background: relationEdge.change_type === "added" ? "#DCFCE7" : "#FEE2E2",
                      borderRadius: 10,
                      color: relationEdge.change_type === "added" ? "#16A34A" : "#EF4444",
                      fontSize: 10,
                      fontWeight: 600,
                      padding: "1px 6px",
                    }}>
                      {relationEdge.change_type === "added" ? "Added" : "Removed"}
                    </span>
                  )}
                </span>
              </div>
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
  return symbolContextLabel(node);
}

function isTypeNode(node: GraphNode): boolean {
  return ["class", "interface", "struct", "type"].includes(node.kind);
}

function functionDisplayName(node: GraphNode): string {
  return symbolTitle(node);
}

function outgoingCallEdges(nodeID: string, edges: GraphEdge[]): GraphEdge[] {
  return edges.filter((e) => e.edge_kind === "calls" && e.source_id === nodeID);
}

function incomingCallEdges(nodeID: string, edges: GraphEdge[]): GraphEdge[] {
  return edges.filter((e) => e.edge_kind === "calls" && e.destination_id === nodeID);
}

function outgoingOwnershipEdges(nodeID: string, edges: GraphEdge[]): GraphEdge[] {
  return edges.filter((e) => e.edge_kind === "owns_method" && e.source_id === nodeID);
}

function outgoingTypeUsageEdges(nodeID: string, edges: GraphEdge[]): GraphEdge[] {
  return edges.filter((e) => e.edge_kind === "uses_type" && e.source_id === nodeID);
}

function incomingOwnershipEdges(nodeID: string, edges: GraphEdge[]): GraphEdge[] {
  return edges.filter((e) => e.edge_kind === "owns_method" && e.destination_id === nodeID);
}

function buildCallSiteLines(
  segment: NodeCodeSegment | undefined,
  destinationIDs: string[],
  allNodes: GraphNode[]
): Record<string, number> {
  if (!segment) return {};

  const lines = segment.source.split("\n");
  const byID: Record<string, number> = {};

  for (const id of destinationIDs) {
    const callee = allNodes.find((candidate) => candidate.id === id);
    if (!callee) continue;

    const index = lines.findIndex((line) => lineMatchesCall(line, functionDisplayName(callee)));
    if (index >= 0) {
      byID[id] = segment.start_line + index;
    }
  }

  return byID;
}

function lineMatchesCall(line: string, functionName: string): boolean {
  const escaped = functionName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`(?:\\b|\\.)${escaped}\\s*\\(`).test(line);
}

const changeRowButtonStyle: CSSProperties = {
  alignItems: "center",
  background: "#F5F5F5",
  border: "1px solid #D4D4D4",
  borderRadius: 6,
  color: "#222222",
  cursor: "pointer",
  display: "flex",
  fontFamily: "inherit",
  gap: 10,
  padding: "8px 10px",
  textAlign: "left",
  width: "100%",
};

function fileKey(file: PRFileDiff): string {
  return `${file.status}:${file.previous_filename ?? ""}:${file.filename}`;
}

function isMarkdownFile(path: string): boolean {
  const lower = path.toLowerCase();
  return lower.endsWith(".md") || lower.endsWith(".mdx") || lower.endsWith(".markdown");
}

function normalizedFileStatus(status: string): string {
  if (status === "removed") return "removed";
  if (status === "deleted") return "removed";
  return status;
}

function FileDiffPills({ file }: { file: PRFileDiff }) {
  const status = normalizedFileStatus(file.status);

  return (
    <span style={{ alignItems: "center", display: "inline-flex", gap: 4, marginLeft: "auto" }}>
      {status === "added" || status === "removed" || status === "renamed" ? (
        <span style={{
          borderRadius: 8,
          background: status === "added" ? "#DCFCE7" : status === "removed" ? "#FEE2E2" : "#E0E7FF",
          color: status === "added" ? "#16A34A" : status === "removed" ? "#EF4444" : "#4F46E5",
          display: "inline-flex",
          fontSize: 10,
          fontWeight: 500,
          lineHeight: 1.25,
          padding: "1px 6px",
          textTransform: "capitalize",
          whiteSpace: "nowrap",
        }}>
          {status}
        </span>
      ) : null}
      {status !== "added" && status !== "removed" && file.additions > 0 && (
        <span style={{ ...fileDiffPillBase, background: "#DCFCE7", color: "#16A34A" }}>+{file.additions}</span>
      )}
      {status !== "added" && status !== "removed" && file.deletions > 0 && (
        <span style={{ ...fileDiffPillBase, background: "#FEE2E2", color: "#EF4444" }}>-{file.deletions}</span>
      )}
      {status === "added" && file.additions > 0 && (
        <span style={{ ...fileDiffPillBase, background: "#DCFCE7", color: "#16A34A" }}>+{file.additions}</span>
      )}
      {status === "removed" && file.deletions > 0 && (
        <span style={{ ...fileDiffPillBase, background: "#FEE2E2", color: "#EF4444" }}>-{file.deletions}</span>
      )}
    </span>
  );
}

const fileDiffPillBase: CSSProperties = {
  borderRadius: 8,
  display: "inline-flex",
  fontSize: 10,
  fontWeight: 500,
  lineHeight: 1.25,
  padding: "1px 6px",
  whiteSpace: "nowrap",
};

function DiffPills({
  node,
  compact = false,
  alignRight = false,
}: {
  node: GraphNode;
  compact?: boolean;
  alignRight?: boolean;
}) {
  if (!node.change_type) return alignRight ? <span style={{ marginLeft: "auto" }} /> : null;

  const pillBase: CSSProperties = {
    borderRadius: compact ? 8 : 12,
    display: "inline-flex",
    fontSize: compact ? 10 : 11,
    fontWeight: 500,
    lineHeight: 1.25,
    padding: compact ? "1px 6px" : "1px 7px",
    whiteSpace: "nowrap",
  };

  return (
    <span
      style={{
        alignItems: "center",
        display: "inline-flex",
        gap: compact ? 4 : 6,
        marginLeft: alignRight ? "auto" : 0,
      }}
    >
      {node.change_type === "added" ? (
        <span style={{ ...pillBase, background: "#DCFCE7", color: "#16A34A" }}>
          Added
        </span>
      ) : node.change_type === "deleted" ? (
        <span style={{ ...pillBase, background: "#FEE2E2", color: "#EF4444" }}>
          Removed
        </span>
      ) : node.change_type === "renamed" ? (
        <span style={{ ...pillBase, background: "#E0E7FF", color: "#4F46E5" }}>
          Renamed
        </span>
      ) : null}
      {node.change_type !== "deleted" && node.lines_added > 0 && (
        <span style={{ ...pillBase, background: "#DCFCE7", color: "#16A34A" }}>+{node.lines_added}</span>
      )}
      {node.change_type !== "deleted" && node.lines_removed > 0 && (
        <span style={{ ...pillBase, background: "#FEE2E2", color: "#EF4444" }}>-{node.lines_removed}</span>
      )}
    </span>
  );
}

const markdownComponents: Components = {
  a: ({ children, href }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
};
