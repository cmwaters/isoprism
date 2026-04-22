"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { GraphNode } from "@/lib/types";
import { packageColor } from "./graph-canvas";

interface Props {
  data: { node: GraphNode };
  selected?: boolean;
}

function GraphNodeComponent({ data, selected }: Props) {
  const { node } = data;
  const pkgColor = packageColor(node.full_name);

  // Parse signature to extract parameters and return type
  const { params, returns } = parseSignature(node.signature);

  const shadow = selected
    ? "0 4px 16px rgba(0,0,0,0.18)"
    : "0 1px 4px rgba(0,0,0,0.12)";

  return (
    <div
      style={{
        background: "white",
        borderRadius: 8,
        boxShadow: shadow,
        padding: 10,
        minWidth: 200,
        maxWidth: 260,
        cursor: "pointer",
        fontFamily: "'JetBrains Mono', 'Fira Mono', monospace",
        transition: "box-shadow 150ms ease-out",
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLDivElement).style.boxShadow = "0 2px 8px rgba(0,0,0,0.15)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLDivElement).style.boxShadow = shadow;
      }}
    >
      <Handle type="target" position={Position.Top} style={{ opacity: 0 }} />

      {/* Package label */}
      <div style={{ fontSize: 11, color: pkgColor, marginBottom: 3, fontWeight: 500 }}>
        {packagePrefix(node.full_name)}
      </div>

      {/* Function name */}
      <div style={{ fontSize: 13, fontWeight: 600, color: "#111111", marginBottom: 6 }}>
        {node.name}
      </div>

      {/* Parameters */}
      {params.length > 0 && (
        <div style={{ marginBottom: 4 }}>
          {params.slice(0, 4).map((p, i) => (
            <div key={i} style={{ fontSize: 11, color: "#444444", lineHeight: 1.5 }}>
              {p}
            </div>
          ))}
          {params.length > 4 && <div style={{ fontSize: 10, color: "#999" }}>+{params.length - 4} more</div>}
        </div>
      )}

      {/* Divider + return types */}
      {returns && (
        <>
          <div style={{ borderTop: "1px solid #EEEEEE", margin: "4px 0" }} />
          <div style={{ fontSize: 11, color: "#666666" }}>{returns}</div>
        </>
      )}

      {/* Diff stat badges (changed nodes only) */}
      {node.node_type === "changed" && (node.lines_added > 0 || node.lines_removed > 0) && (
        <div style={{ display: "flex", gap: 4, marginTop: 8 }}>
          {node.lines_added > 0 && (
            <span style={{ background: "#DCFCE7", color: "#16A34A", borderRadius: 4, padding: "1px 5px", fontSize: 10 }}>
              +{node.lines_added}
            </span>
          )}
          {node.lines_removed > 0 && (
            <span style={{ background: "#FEE2E2", color: "#EF4444", borderRadius: 4, padding: "1px 5px", fontSize: 10 }}>
              -{node.lines_removed}
            </span>
          )}
        </div>
      )}

      {/* Status badge for added/deleted */}
      {node.change_type === "added" && (
        <span style={{ background: "#DCFCE7", color: "#16A34A", borderRadius: 4, padding: "1px 6px", fontSize: 10, marginTop: 6, display: "inline-block" }}>
          Added
        </span>
      )}
      {node.change_type === "deleted" && (
        <span style={{ background: "#FEE2E2", color: "#EF4444", borderRadius: 4, padding: "1px 6px", fontSize: 10, marginTop: 6, display: "inline-block" }}>
          Deleted
        </span>
      )}

      <Handle type="source" position={Position.Bottom} style={{ opacity: 0 }} />
    </div>
  );
}

function packagePrefix(fullName: string): string {
  const dot = fullName.indexOf(".");
  if (dot === -1) return "";
  return fullName.slice(0, dot);
}

function parseSignature(sig: string): { params: string[]; returns: string } {
  // Very basic signature parser: extract between outer parens
  try {
    const parenOpen = sig.indexOf("(");
    const parenClose = sig.lastIndexOf(")");
    if (parenOpen === -1 || parenClose === -1) return { params: [], returns: "" };

    const paramsStr = sig.slice(parenOpen + 1, parenClose).trim();
    const afterParen = sig.slice(parenClose + 1).trim();

    const params = paramsStr ? paramsStr.split(",").map((p) => p.trim()).filter(Boolean) : [];
    return { params, returns: afterParen };
  } catch {
    return { params: [], returns: "" };
  }
}

export default memo(GraphNodeComponent);
