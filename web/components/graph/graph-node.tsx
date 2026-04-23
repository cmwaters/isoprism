"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { GraphNode } from "@/lib/types";
import { cardColorByKind } from "./graph-canvas";

interface Props {
  data: { node: GraphNode };
  selected?: boolean;
}

function inferPackageLabel(node: GraphNode): string {
  const parts = node.file_path.split("/");
  const pkg = parts.length >= 2 ? parts[parts.length - 2] : "";
  if (node.kind === "method" || node.full_name.includes(".")) {
    const prefix = node.full_name.split(".").slice(0, -1).join(".");
    return pkg ? `${pkg}.${prefix}` : prefix;
  }
  return pkg;
}

function parseGoSignature(sig: string): { params: { name: string; type: string }[]; returns: string } {
  if (!sig || sig.startsWith("type ")) return { params: [], returns: "" };
  if (!sig.startsWith("func ")) return { params: [], returns: sig };

  let rest = sig.slice(5).trim();

  // Skip receiver `(recv Type)`
  if (rest.startsWith("(")) {
    let depth = 0;
    let i = 0;
    for (; i < rest.length; i++) {
      if (rest[i] === "(") depth++;
      else if (rest[i] === ")") { depth--; if (depth === 0) break; }
    }
    rest = rest.slice(i + 1).trim();
  }

  // Find params block after function name
  const parenIdx = rest.indexOf("(");
  if (parenIdx === -1) return { params: [], returns: "" };
  rest = rest.slice(parenIdx);

  let depth = 0, paramsEnd = 0;
  for (let i = 0; i < rest.length; i++) {
    if (rest[i] === "(") depth++;
    else if (rest[i] === ")") { depth--; if (depth === 0) { paramsEnd = i; break; } }
  }

  const paramsStr = rest.slice(1, paramsEnd).trim();
  const returns = rest.slice(paramsEnd + 1).trim();

  return { params: parseGoParams(paramsStr), returns };
}

function parseGoParams(s: string): { name: string; type: string }[] {
  if (!s) return [];

  const parts: string[] = [];
  let depth = 0, cur = "";
  for (const ch of s) {
    if ("([{".includes(ch)) depth++;
    else if (")]}".includes(ch)) depth--;
    else if (ch === "," && depth === 0) { parts.push(cur.trim()); cur = ""; continue; }
    cur += ch;
  }
  if (cur.trim()) parts.push(cur.trim());

  const result: { name: string; type: string }[] = [];
  let pending: string[] = [];
  for (const part of parts) {
    const sp = part.search(/\s/);
    if (sp === -1) {
      pending.push(part);
    } else {
      const name = part.slice(0, sp);
      const type = part.slice(sp + 1).trim();
      for (const n of pending) result.push({ name: n, type });
      pending = [];
      result.push({ name, type });
    }
  }
  for (const n of pending) result.push({ name: n, type: "" });
  return result;
}

function GraphNodeComponent({ data, selected }: Props) {
  const { node } = data;
  const bg = selected ? "#F5F5F5" : cardColorByKind(node.kind);
  const pkgLabel = inferPackageLabel(node);
  const { params, returns } = parseGoSignature(node.signature);
  const hasDiff = node.node_type === "changed" && (node.lines_added > 0 || node.lines_removed > 0);

  const handleStyle = { opacity: 0, width: 8, height: 8 };

  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
      <Handle type="target" position={Position.Top} style={handleStyle} />
      <Handle type="target" position={Position.Left} style={handleStyle} />
      <Handle type="source" position={Position.Right} style={handleStyle} />
      <Handle type="source" position={Position.Bottom} style={handleStyle} />

      {/* Card */}
      <div
        style={{
          background: bg,
          borderRadius: 8,
          boxShadow: selected ? "0 4px 16px rgba(0,0,0,0.18)" : "0 1px 4px rgba(0,0,0,0.12)",
          padding: 10,
          minWidth: 190,
          maxWidth: 250,
          cursor: "pointer",
          fontFamily: "'JetBrains Mono', 'Fira Mono', monospace",
          transition: "box-shadow 150ms ease-out",
        }}
      >
        {/* Package label */}
        {pkgLabel && (
          <div style={{ fontSize: 11, color: "#EF5DA8", marginBottom: 3, fontWeight: 500 }}>
            {pkgLabel}
          </div>
        )}

        {/* Function name */}
        <div style={{ fontSize: 13, fontWeight: 600, color: "#111111", marginBottom: params.length > 0 || returns ? 6 : 0 }}>
          {node.name}
        </div>

        {/* Parameters */}
        {params.length > 0 && (
          <div style={{ marginBottom: returns ? 4 : 0 }}>
            {params.slice(0, 4).map((p, i) => (
              <div key={i} style={{ fontSize: 11, lineHeight: 1.5 }}>
                {p.name && <span style={{ color: "#444444" }}>{p.name} </span>}
                <span style={{ color: "#0088FF" }}>{p.type}</span>
              </div>
            ))}
            {params.length > 4 && (
              <div style={{ fontSize: 10, color: "#999" }}>+{params.length - 4} more</div>
            )}
          </div>
        )}

        {/* Return types */}
        {returns && (
          <>
            <div style={{ borderTop: "1px solid rgba(0,0,0,0.1)", margin: "4px 0" }} />
            <div style={{ fontSize: 11, color: "#FF383C" }}>{returns}</div>
          </>
        )}

        {/* Added / deleted status */}
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
      </div>

      {/* Diff pills below card */}
      {hasDiff && (
        <div style={{ display: "flex", gap: 6, marginTop: 8 }}>
          {node.lines_added > 0 && (
            <span style={{
              background: "#EF5DA8",
              color: "#FFFFFF",
              borderRadius: 12,
              padding: "2px 10px",
              fontSize: 11,
              fontWeight: 600,
              fontFamily: "'JetBrains Mono', monospace",
            }}>
              +{node.lines_added}
            </span>
          )}
          {node.lines_removed > 0 && (
            <span style={{
              background: "#E08D8D",
              color: "#FFFFFF",
              borderRadius: 12,
              padding: "2px 10px",
              fontSize: 11,
              fontWeight: 600,
              fontFamily: "'JetBrains Mono', monospace",
            }}>
              -{node.lines_removed}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export default memo(GraphNodeComponent);
