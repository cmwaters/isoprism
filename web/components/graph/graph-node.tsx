"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { GraphNode } from "@/lib/types";
import { cardColorByKind } from "./graph-canvas";

const CARD_PADDING = 10;

interface Props {
  data: { node: GraphNode };
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

function parseGoSignature(sig: string): { params: { name: string; type: string }[]; returns: string[] } {
  if (!sig || sig.startsWith("type ")) return { params: [], returns: [] };
  if (!sig.startsWith("func ")) return { params: [], returns: [sig] };

  let rest = sig.slice(5).trim();
  if (rest.startsWith("(")) {
    let depth = 0, i = 0;
    for (; i < rest.length; i++) {
      if (rest[i] === "(") depth++;
      else if (rest[i] === ")") { depth--; if (depth === 0) break; }
    }
    rest = rest.slice(i + 1).trim();
  }

  const parenIdx = rest.indexOf("(");
  if (parenIdx === -1) return { params: [], returns: [] };
  rest = rest.slice(parenIdx);

  let depth = 0, paramsEnd = 0;
  for (let i = 0; i < rest.length; i++) {
    if (rest[i] === "(") depth++;
    else if (rest[i] === ")") { depth--; if (depth === 0) { paramsEnd = i; break; } }
  }

  const paramsStr = rest.slice(1, paramsEnd).trim();
  const rawReturns = rest.slice(paramsEnd + 1).trim();

  // Split tuple returns "(a, b)" → ["a", "b"], or single return as-is
  let returns: string[] = [];
  if (rawReturns) {
    if (rawReturns.startsWith("(") && rawReturns.endsWith(")")) {
      returns = rawReturns.slice(1, -1).split(",").map(s => s.trim()).filter(Boolean);
    } else {
      returns = [rawReturns];
    }
  }

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

function DiffPills({ node }: { node: GraphNode }) {
  if (!node.change_type) return null;
  const pillBase: React.CSSProperties = {
    display: "inline-flex", alignItems: "center",
    borderRadius: 12, padding: "2px 8px", fontSize: 11, fontWeight: 500,
  };
  return (
    <div style={{ display: "flex", gap: 6, marginTop: 6, paddingLeft: 2 }}>
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
  const { node } = data;
  const bg = selected ? "#F5F5F5" : cardColorByKind(node.kind);
  const dividerColor = darken(bg);
  const pkgLabel = inferPackageLabel(node);
  const { params, returns } = parseGoSignature(node.signature);
  const hasSignature = params.length > 0 || returns.length > 0;

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

        {/* Function / struct name */}
        <div style={{
          fontSize: 13,
          fontWeight: 600,
          color: "#111111",
          marginBottom: hasSignature ? 6 : 0,
          wordBreak: "break-word",
        }}>
          {node.name}
        </div>

        {/* Parameters — one per line */}
        {params.length > 0 && (
          <div>
            {params.map((p, i) => (
              <div key={i} style={{ fontSize: 11, lineHeight: 1.5 }}>
                {p.name && <span style={{ color: "#444444" }}>{p.name} </span>}
                <span style={{ color: "#0088FF" }}>{p.type}</span>
              </div>
            ))}
          </div>
        )}

        {/* Return types — full-width divider + one per line */}
        {returns.length > 0 && (
          <>
            <Divider color={dividerColor} />
            {returns.map((r, i) => (
              <div key={i} style={{ fontSize: 11, color: "#FF383C", lineHeight: 1.5 }}>{r}</div>
            ))}
          </>
        )}
      </div>

      {/* Diff pills — below the card, not inside */}
      <DiffPills node={node} />
    </div>
  );
}

export default memo(GraphNodeComponent);
