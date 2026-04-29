"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { GraphNode, GraphNodeTypeRef } from "@/lib/types";
import { cardColorByKind } from "./graph-canvas";

const CARD_PADDING = 10;

interface Props {
  data: { node: GraphNode; onSelectType?: (nodeID: string) => void };
  selected?: boolean;
}

function darken(hex: string, amount = 0.13): string {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  const d = (v: number) => Math.max(0, Math.round(v * (1 - amount))).toString(16).padStart(2, "0");
  return `#${d(r)}${d(g)}${d(b)}`;
}

function Divider({ color }: { color: string }) {
  return (
    <div style={{
      height: 1,
      background: color,
      margin: `6px -${CARD_PADDING}px`,
    }} />
  );
}

function inferPackageLabel(node: GraphNode): string {
  if (node.granularity === "package") return "package";
  if (node.granularity === "object") return node.package_path || "object";
  const parts = node.file_path.split("/");
  // Use directory name as package; fall back to filename stem for root-level files
  const pkg = parts.length >= 2
    ? parts[parts.length - 2]
    : parts[0].replace(/\.[^.]+$/, "");
  if (node.kind === "method" || node.full_name.includes(".")) {
    const prefix = node.full_name.split(".").slice(0, -1).join(".");
    return pkg ? `${pkg}.${prefix}` : prefix;
  }
  return pkg;
}

function DiffPills({ node }: { node: GraphNode }) {
  if (!node.change_type && !node.changed_member_count) return null;
  const pillBase: React.CSSProperties = {
    display: "inline-flex", alignItems: "center",
    borderRadius: 12, padding: "2px 8px", fontSize: 11, fontWeight: 500,
  };
  return (
    <div style={{ display: "flex", gap: 6, marginTop: 6, paddingLeft: 2 }}>
      {!node.change_type && Boolean(node.changed_member_count) && (
        <span style={{ ...pillBase, background: "#F0FFF4", color: "#166534" }}>
          {node.changed_member_count} changed
        </span>
      )}
      {node.change_type === "added" && (
        <span style={{ ...pillBase, background: "#DCFCE7", color: "#16A34A" }}>
          Added {node.lines_added > 0 ? `+${node.lines_added}` : ""}
        </span>
      )}
      {node.change_type === "deleted" && (
        <span style={{ ...pillBase, background: "#FEE2E2", color: "#EF4444" }}>Deleted</span>
      )}
      {node.change_type === "modified" && (
        <>
          {node.lines_added > 0 && (
            <span style={{ ...pillBase, background: "#DCFCE7", color: "#16A34A" }}>+{node.lines_added}</span>
          )}
          {node.lines_removed > 0 && (
            <span style={{ ...pillBase, background: "#FEE2E2", color: "#EF4444" }}>-{node.lines_removed}</span>
          )}
        </>
      )}
    </div>
  );
}

function GraphNodeComponent({ data, selected }: Props) {
  const { node, onSelectType } = data;
  const bg = selected ? "#F5F5F5" : cardColorByKind(node.kind);
  const dividerColor = darken(bg);
  const pkgLabel = inferPackageLabel(node);
  const inputs = node.inputs ?? [];
  const outputs = node.outputs ?? [];
  const hasAggregateMeta = node.granularity !== "function" && Boolean(node.member_count);
  const hasIO = inputs.length > 0 || outputs.length > 0;

  const handleStyle = { opacity: 0, width: 8, height: 8 };

  return (
    <div style={{ display: "inline-flex", flexDirection: "column" }}>
      <Handle type="target" position={Position.Top} style={handleStyle} />
      <Handle type="target" position={Position.Left} style={handleStyle} />
      <Handle type="source" position={Position.Right} style={handleStyle} />
      <Handle type="source" position={Position.Bottom} style={handleStyle} />

      <div style={{
        background: bg,
        borderRadius: 8,
        padding: CARD_PADDING,
        maxWidth: 300,
        cursor: "pointer",
        fontFamily: "'JetBrains Mono', 'Fira Mono', monospace",
      }}>
        {/* Package label */}
        {pkgLabel && (
          <div style={{ fontSize: 11, color: "#EF5DA8", marginBottom: 3, fontWeight: 500, whiteSpace: "nowrap" }}>
            {pkgLabel}
          </div>
        )}

        {/* Function / aggregate name */}
        <div style={{
          fontSize: 13,
          fontWeight: 600,
          color: "#111111",
          marginBottom: hasIO ? 6 : 0,
          wordBreak: "break-word",
        }}>
          {node.full_name}
        </div>

        {hasAggregateMeta && (
          <div style={{ fontSize: 11, color: "#666666", lineHeight: 1.45 }}>
            {node.member_count} {node.member_count === 1 ? "member" : "members"}
            {node.changed_member_count ? ` · ${node.changed_member_count} changed` : ""}
          </div>
        )}

        {hasIO && <Divider color={dividerColor} />}

        {/* Parameters — one per line */}
        {inputs.length > 0 && (
          <div>
            {inputs.map((p, i) => (
              <div key={i} style={{ fontSize: 11, lineHeight: 1.5 }}>
                {p.name && <span style={{ color: "#444444" }}>{p.name} </span>}
                <TypeRef refInfo={p} color="#0088FF" onSelectType={onSelectType} />
              </div>
            ))}
          </div>
        )}

        {/* Return types — one per line */}
        {outputs.length > 0 && (
          <div style={{ marginTop: inputs.length > 0 ? 4 : 0 }}>
            {outputs.map((r, i) => (
              <div key={i} style={{ fontSize: 11, lineHeight: 1.5 }}>
                {r.name && <span style={{ color: "#444444" }}>{r.name} </span>}
                <TypeRef refInfo={r} color="#FF383C" onSelectType={onSelectType} />
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Diff pills — below the card, not inside */}
      <DiffPills node={node} />
    </div>
  );
}

function TypeRef({
  refInfo,
  color,
  onSelectType,
}: {
  refInfo: GraphNodeTypeRef;
  color: string;
  onSelectType?: (nodeID: string) => void;
}) {
  if (!refInfo.node_id || !onSelectType) {
    return <span style={{ color }}>{refInfo.type}</span>;
  }
  return (
    <button
      type="button"
      onClick={(event) => {
        event.stopPropagation();
        onSelectType(refInfo.node_id!);
      }}
      style={{
        appearance: "none",
        border: 0,
        padding: 0,
        background: "transparent",
        color,
        cursor: "pointer",
        font: "inherit",
        textDecoration: "underline",
        textUnderlineOffset: 2,
      }}
    >
      {refInfo.type}
    </button>
  );
}

export default memo(GraphNodeComponent);
