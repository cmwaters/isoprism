"use client";

import { useCallback, useState, useEffect, useMemo, useRef } from "react";
import {
  ReactFlow,
  Node,
  Edge,
  Background,
  BaseEdge,
  EdgeProps,
  useNodesState,

  useReactFlow,
  ReactFlowProvider,
  NodeMouseHandler,
  EdgeMouseHandler,
  PanOnScrollMode,
  ConnectionMode,
  MarkerType,
  useStore,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { GraphExpansionResponse, GraphResponse, GraphNode as APIGraphNode, NodeCodeResponse, QueuePR, RepoGraphResponse, Repository } from "@/lib/types";
import { apiFetch } from "@/lib/api";
import BetaFeedbackBanner from "@/components/beta-feedback-banner";
import { SettingsView } from "@/components/settings/settings-view";
import GraphNodeComponent from "./graph-node";
import NodeDetailPanel, { ComponentChangePanel, type SelectedPRChange } from "./node-detail-panel";

export const nodeTypes = { graphNode: GraphNodeComponent };
export const edgeTypes = { smartBezier: SmartBezierEdge };

type Point = { x: number; y: number };
type Rect = Point & { width: number; height: number };
type AnchorSide = "top" | "right" | "bottom" | "left";
type Anchor = Point & { normal: Point; side: AnchorSide; offset: number };
type MeasuredNode = {
  internals: { positionAbsolute: Point };
  measured: { width?: number; height?: number };
  width?: number;
  height?: number;
  data?: { node?: APIGraphNode };
};
export type PanelMode = "overview" | "code";

const DIFF_PILLS_HEIGHT = 28;
const CARD_CORNER_MARGIN = 20;
const MIN_ANCHOR_SPACING = 20;

function cardRect(node: MeasuredNode): Rect {
  const measuredHeight = node.measured.height ?? node.height ?? 120;
  const hasDiffPills = Boolean(node.data?.node?.change_type);

  return {
    x: node.internals.positionAbsolute.x,
    y: node.internals.positionAbsolute.y,
    width: node.measured.width ?? node.width ?? NODE_W,
    height: Math.max(48, measuredHeight - (hasDiffPills ? DIFF_PILLS_HEIGHT : 0)),
  };
}

function rectCenter(rect: Rect): Point {
  return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
}

function anchorFromSide(rect: Rect, side: AnchorSide, offset: number): Anchor {
  const horizontal = side === "top" || side === "bottom";
  const length = horizontal ? rect.width : rect.height;
  const margin = Math.min(CARD_CORNER_MARGIN, length / 2);
  const clampedOffset = Math.max(margin, Math.min(length - margin, offset));

  switch (side) {
    case "top":
      return { x: rect.x + clampedOffset, y: rect.y, normal: { x: 0, y: -1 }, side, offset: clampedOffset };
    case "right":
      return { x: rect.x + rect.width, y: rect.y + clampedOffset, normal: { x: 1, y: 0 }, side, offset: clampedOffset };
    case "bottom":
      return { x: rect.x + clampedOffset, y: rect.y + rect.height, normal: { x: 0, y: 1 }, side, offset: clampedOffset };
    case "left":
      return { x: rect.x, y: rect.y + clampedOffset, normal: { x: -1, y: 0 }, side, offset: clampedOffset };
  }
}

function anchorAtAngle(rect: Rect, angle: number): Anchor {
  const center = rectCenter(rect);
  const dx = Math.cos(angle);
  const dy = Math.sin(angle);
  const scale = Math.min(
    dx === 0 ? Infinity : (rect.width / 2) / Math.abs(dx),
    dy === 0 ? Infinity : (rect.height / 2) / Math.abs(dy)
  );

  let x = center.x + dx * scale;
  let y = center.y + dy * scale;

  if (Math.abs(x - rect.x) < 0.01) {
    y = Math.max(rect.y, Math.min(rect.y + rect.height, y));
    return anchorFromSide(rect, "left", y - rect.y);
  } else if (Math.abs(x - (rect.x + rect.width)) < 0.01) {
    y = Math.max(rect.y, Math.min(rect.y + rect.height, y));
    return anchorFromSide(rect, "right", y - rect.y);
  } else if (Math.abs(y - rect.y) < 0.01) {
    x = Math.max(rect.x, Math.min(rect.x + rect.width, x));
    return anchorFromSide(rect, "top", x - rect.x);
  } else {
    x = Math.max(rect.x, Math.min(rect.x + rect.width, x));
    return anchorFromSide(rect, "bottom", x - rect.x);
  }
}

function faceCenterAnchor(rect: Rect, normal: Point): Anchor {
  if (Math.abs(normal.x) > Math.abs(normal.y)) {
    return anchorFromSide(rect, normal.x > 0 ? "right" : "left", rect.height / 2);
  }

  return anchorFromSide(rect, normal.y > 0 ? "bottom" : "top", rect.width / 2);
}

function edgeAngleFromNode(nodeID: string, nodeRect: Rect, edge: Edge, nodeRects: Map<string, Rect>): number {
  const otherID = edge.source === nodeID ? edge.target : edge.source;
  const otherRect = nodeRects.get(otherID);
  if (!otherRect) return 0;

  const from = rectCenter(nodeRect);
  const to = rectCenter(otherRect);
  return Math.atan2(to.y - from.y, to.x - from.x);
}

function lineAnchor(nodeID: string, rect: Rect, edge: Edge, nodeRects: Map<string, Rect>): Anchor {
  const otherID = edge.source === nodeID ? edge.target : edge.source;
  const otherRect = nodeRects.get(otherID);
  if (!otherRect) return faceCenterAnchor(rect, { x: 1, y: 0 });

  return anchorAtAngle(rect, edgeAngleFromNode(nodeID, rect, edge, nodeRects));
}

function spacedFaceAnchors(rect: Rect, anchors: Anchor[]): Anchor[] {
  if (anchors.length <= 1) return anchors;

  const side = anchors[0].side;
  const length = side === "top" || side === "bottom" ? rect.width : rect.height;
  const margin = Math.min(CARD_CORNER_MARGIN, length / 2);
  const minOffset = margin;
  const maxOffset = length - margin;
  const available = maxOffset - minOffset;
  const spacing = anchors.length > 1 ? Math.min(MIN_ANCHOR_SPACING, available / (anchors.length - 1)) : MIN_ANCHOR_SPACING;

  const byOffset = anchors
    .map((anchor, index) => ({ anchor, index }))
    .sort((a, b) => a.anchor.offset - b.anchor.offset || a.index - b.index);

  const offsets = byOffset.map(({ anchor }) => Math.max(minOffset, Math.min(maxOffset, anchor.offset)));

  for (let i = 1; i < offsets.length; i++) {
    offsets[i] = Math.max(offsets[i], offsets[i - 1] + spacing);
  }

  const overflow = offsets[offsets.length - 1] - maxOffset;
  if (overflow > 0) {
    for (let i = 0; i < offsets.length; i++) offsets[i] -= overflow;
  }

  for (let i = offsets.length - 2; i >= 0; i--) {
    offsets[i] = Math.min(offsets[i], offsets[i + 1] - spacing);
  }

  const underflow = minOffset - offsets[0];
  if (underflow > 0) {
    for (let i = 0; i < offsets.length; i++) offsets[i] += underflow;
  }

  const result = [...anchors];
  byOffset.forEach(({ index }, sortedIndex) => {
    result[index] = anchorFromSide(rect, side, offsets[sortedIndex]);
  });

  return result;
}

function endpointAnchor(nodeID: string, edgeID: string, rect: Rect, edges: Edge[], nodeRects: Map<string, Rect>): Anchor {
  const incidentEdges = edges.filter((edge) => edge.source === nodeID || edge.target === nodeID);

  if (incidentEdges.length <= 1) {
    const edge = incidentEdges[0];
    return edge ? lineAnchor(nodeID, rect, edge, nodeRects) : faceCenterAnchor(rect, { x: 1, y: 0 });
  }

  const anchorsByEdge = incidentEdges.map((edge) => ({ edge, anchor: lineAnchor(nodeID, rect, edge, nodeRects) }));
  const anchorsBySide = new Map<AnchorSide, { edge: Edge; anchor: Anchor }[]>();

  for (const item of anchorsByEdge) {
    const sideAnchors = anchorsBySide.get(item.anchor.side) ?? [];
    sideAnchors.push(item);
    anchorsBySide.set(item.anchor.side, sideAnchors);
  }

  for (const sideAnchors of anchorsBySide.values()) {
    const spacedAnchors = spacedFaceAnchors(rect, sideAnchors.map(({ anchor }) => anchor));
    sideAnchors.forEach((item, index) => {
      item.anchor = spacedAnchors[index];
    });
  }

  return anchorsByEdge.find(({ edge }) => edge.id === edgeID)?.anchor ?? lineAnchor(nodeID, rect, incidentEdges[0], nodeRects);
}

function smartBezierPath(sourceAnchor: Anchor, targetAnchor: Anchor): string {
  const distance = Math.hypot(targetAnchor.x - sourceAnchor.x, targetAnchor.y - sourceAnchor.y);
  const controlDistance = Math.max(36, Math.min(180, distance * 0.36));
  const sourceControl = {
    x: sourceAnchor.x + sourceAnchor.normal.x * controlDistance,
    y: sourceAnchor.y + sourceAnchor.normal.y * controlDistance,
  };
  const targetControl = {
    x: targetAnchor.x + targetAnchor.normal.x * controlDistance,
    y: targetAnchor.y + targetAnchor.normal.y * controlDistance,
  };

  return `M ${sourceAnchor.x},${sourceAnchor.y} C ${sourceControl.x},${sourceControl.y} ${targetControl.x},${targetControl.y} ${targetAnchor.x},${targetAnchor.y}`;
}

function SmartBezierEdge({
  id,
  source,
  target,
  sourceX,
  sourceY,
  targetX,
  targetY,
  markerEnd,
  style,
  interactionWidth,
}: EdgeProps) {
  const sourceNode = useStore((store) => store.nodeLookup.get(source));
  const targetNode = useStore((store) => store.nodeLookup.get(target));
  const flowEdges = useStore((store) => store.edges);
  const nodeLookup = useStore((store) => store.nodeLookup);

  const path = sourceNode && targetNode
    ? (() => {
        const nodeRects = new Map(
          Array.from(nodeLookup.entries()).map(([nodeID, node]) => [nodeID, cardRect(node)])
        );
        const sourceRect = cardRect(sourceNode);
        const targetRect = cardRect(targetNode);
        return smartBezierPath(
          endpointAnchor(source, id, sourceRect, flowEdges, nodeRects),
          endpointAnchor(target, id, targetRect, flowEdges, nodeRects)
        );
      })()
    : `M ${sourceX},${sourceY} C ${sourceX},${sourceY} ${targetX},${targetY} ${targetX},${targetY}`;

  return (
    <BaseEdge
      id={id}
      path={path}
      markerEnd={markerEnd}
      style={style}
      interactionWidth={interactionWidth}
    />
  );
}

// ── Card color by kind ────────────────────────────────────────────────────────
export function cardColorByKind(kind: string): string {
  switch (kind) {
    case "function":
    case "method":
      return "#D5E7EB";
    case "struct":
    case "type":
      return "#CBCCE5";
    case "interface":
      return "#E5C8DC";
    default:
      return "#D5E7EB";
  }
}

// ── Edge-length layout ────────────────────────────────────────────────────────
const NODE_W = 280;
const NODE_H = 136;
const PANEL_MIN_WIDTH = 260;
const PANEL_MAX_WIDTH = 620;
const PANEL_DEFAULT_WIDTH = 370;
const COMPONENT_PANEL_MIN_WIDTH = 320;
const COMPONENT_PANEL_MAX_WIDTH = 720;
const COMPONENT_PANEL_DEFAULT_WIDTH = 430;

type LayoutOptions = {
  previousPositions?: Record<string, Point>;
  anchor?: Point;
  pinnedIDs?: Set<string>;
  iterations?: number;
};

function edgeWeight(edge: Edge): number {
  const data = edge.data as { weight?: number; underlyingEdgeCount?: number } | undefined;
  return Math.min(5, Math.max(1, data?.underlyingEdgeCount ?? data?.weight ?? 1));
}

function undirectedEdgeKey(a: string, b: string): string {
  return a < b ? `${a}|${b}` : `${b}|${a}`;
}

function initialPosition(index: number, total: number, anchor?: Point): Point {
  const radius = Math.max(280, Math.sqrt(Math.max(total, 1)) * 135);
  const angle = (index / Math.max(total, 1)) * Math.PI * 2;
  const center = anchor ?? { x: 0, y: 0 };
  return {
    x: center.x + Math.cos(angle) * radius,
    y: center.y + Math.sin(angle) * radius,
  };
}

export function edgeLengthLayout(nodes: Node[], edges: Edge[], options: LayoutOptions = {}): Node[] {
  if (nodes.length === 0) return [];

  const nodeIDs = new Set(nodes.map((node) => node.id));
  const sizes = new Map(nodes.map((node) => [
    node.id,
    {
      width: node.measured?.width ?? node.width ?? NODE_W,
      height: node.measured?.height ?? node.height ?? NODE_H,
    },
  ]));
  const positions = new Map<string, Point>();
  const velocities = new Map<string, Point>();
  nodes.forEach((node, index) => {
    const previous = options.previousPositions?.[node.id];
    const initial = previous ?? initialPosition(index, nodes.length, options.anchor);
    positions.set(node.id, { ...initial });
    velocities.set(node.id, { x: 0, y: 0 });
  });

  const validEdges = edges.filter((edge) => nodeIDs.has(edge.source) && nodeIDs.has(edge.target));
  const connectedPairs = new Set(validEdges.map((edge) => undirectedEdgeKey(edge.source, edge.target)));
  const previous = options.previousPositions ?? {};
  const iterations = options.iterations ?? (Object.keys(previous).length > 0 ? 90 : 220);
  const pinnedIDs = options.pinnedIDs ?? new Set<string>();

  for (let tick = 0; tick < iterations; tick++) {
    const forces = new Map(nodes.map((node) => [node.id, { x: 0, y: 0 }]));

    validEdges.forEach((edge) => {
      const source = positions.get(edge.source);
      const target = positions.get(edge.target);
      const sourceSize = sizes.get(edge.source);
      const targetSize = sizes.get(edge.target);
      if (!source || !target || !sourceSize || !targetSize) return;

      const sx = source.x + sourceSize.width / 2;
      const sy = source.y + sourceSize.height / 2;
      const tx = target.x + targetSize.width / 2;
      const ty = target.y + targetSize.height / 2;
      const dx = tx - sx;
      const dy = ty - sy;
      const distance = Math.max(1, Math.hypot(dx, dy));
      const desired = 310;
      const strength = 0.012 * edgeWeight(edge);
      const force = (distance - desired) * strength;
      const fx = (dx / distance) * force;
      const fy = (dy / distance) * force;
      const sourceForce = forces.get(edge.source);
      const targetForce = forces.get(edge.target);
      if (sourceForce && targetForce) {
        sourceForce.x += fx;
        sourceForce.y += fy;
        targetForce.x -= fx;
        targetForce.y -= fy;
      }
    });

    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i];
        const b = nodes[j];
        const pa = positions.get(a.id);
        const pb = positions.get(b.id);
        const sa = sizes.get(a.id);
        const sb = sizes.get(b.id);
        if (!pa || !pb || !sa || !sb) continue;
        const ax = pa.x + sa.width / 2;
        const ay = pa.y + sa.height / 2;
        const bx = pb.x + sb.width / 2;
        const by = pb.y + sb.height / 2;
        const dx = bx - ax || 0.01;
        const dy = by - ay || 0.01;
        const distance = Math.max(1, Math.hypot(dx, dy));
        const radiusA = Math.hypot(sa.width, sa.height) / 2;
        const radiusB = Math.hypot(sb.width, sb.height) / 2;
        const minDistance = radiusA + radiusB + 150;
        const overlap = Math.max(0, minDistance - distance);
        const disconnectedRepulsion = connectedPairs.has(undirectedEdgeKey(a.id, b.id)) ? 0 : 280 / Math.max(distance, 120);
        const force = overlap * 0.055 + 280 / (distance * distance) + disconnectedRepulsion;
        const fx = (dx / distance) * force;
        const fy = (dy / distance) * force;
        const fa = forces.get(a.id);
        const fb = forces.get(b.id);
        if (fa && fb) {
          fa.x -= fx;
          fa.y -= fy;
          fb.x += fx;
          fb.y += fy;
        }
      }
    }

    nodes.forEach((node) => {
      const position = positions.get(node.id);
      const force = forces.get(node.id);
      const velocity = velocities.get(node.id);
      if (!position || !force || !velocity) return;

      force.x += -position.x * 0.002;
      force.y += -position.y * 0.002;
      const previousPosition = previous[node.id];
      if (previousPosition) {
        const preserveStrength = pinnedIDs.has(node.id) ? 0.12 : 0.025;
        force.x += (previousPosition.x - position.x) * preserveStrength;
        force.y += (previousPosition.y - position.y) * preserveStrength;
      }

      velocity.x = (velocity.x + force.x) * 0.72;
      velocity.y = (velocity.y + force.y) * 0.72;
      position.x += Math.max(-36, Math.min(36, velocity.x));
      position.y += Math.max(-36, Math.min(36, velocity.y));
    });
  }

  return nodes.map((node) => ({
    ...node,
    position: positions.get(node.id) ?? node.position,
  }));
}

export function hexGridLayout(nodes: Node[], edges: Edge[], ...rest: unknown[]): Node[] {
  void rest;
  return edgeLengthLayout(nodes, edges);
}

// ── Inner canvas ──────────────────────────────────────────────────────────────
type UnifiedGraph = GraphResponse | RepoGraphResponse;

function graphContextKey(graph: UnifiedGraph): string {
  return isPRGraph(graph) ? `pr:${graph.pr.id}` : "repo";
}

function edgeKey(edge: { source_id: string; destination_id: string; edge_kind?: string }): string {
  return `${edge.source_id}|${edge.destination_id}|${edge.edge_kind ?? "calls"}`;
}

function graphNodeSemanticKey(node: APIGraphNode): string {
  return `${node.file_path}|${node.full_name}`;
}

function mergeGraphNode(existing: APIGraphNode, incoming: APIGraphNode): APIGraphNode {
  if (incoming.change_type && !existing.change_type) {
    return { ...incoming, id: existing.id };
  }
  return {
    ...existing,
    boundary: incoming.boundary,
    degree: Math.max(existing.degree ?? 0, incoming.degree ?? 0),
    tests: existing.tests?.length ? existing.tests : incoming.tests,
  };
}

function mergeExpansionGraph(graph: UnifiedGraph, response: GraphExpansionResponse): UnifiedGraph {
  const incomingNodeIDs = new Set(response.nodes.map((node) => node.id));
  const nodeByID = new Map<string, APIGraphNode>();
  const idBySemanticKey = new Map<string, string>();
  const canonicalIDByIncomingID = new Map<string, string>();

  for (const node of graph.nodes) {
    if (node.id === response.expanded_node_id) {
      const updated = { ...node, boundary: response.has_more };
      nodeByID.set(node.id, updated);
      idBySemanticKey.set(graphNodeSemanticKey(updated), node.id);
    } else {
      const updated = incomingNodeIDs.has(node.id) ? { ...node, boundary: false } : node;
      nodeByID.set(node.id, updated);
      idBySemanticKey.set(graphNodeSemanticKey(updated), node.id);
    }
  }
  for (const node of response.nodes) {
    const semanticKey = graphNodeSemanticKey(node);
    const existingID = idBySemanticKey.get(semanticKey);
    if (existingID) {
      const existing = nodeByID.get(existingID);
      if (existing) nodeByID.set(existingID, mergeGraphNode(existing, node));
      canonicalIDByIncomingID.set(node.id, existingID);
      continue;
    }
    nodeByID.set(node.id, node);
    idBySemanticKey.set(semanticKey, node.id);
    canonicalIDByIncomingID.set(node.id, node.id);
  }

  const edgeByKey = new Map<string, UnifiedGraph["edges"][number]>();
  for (const edge of graph.edges) {
    edgeByKey.set(edgeKey(edge), edge);
  }
  for (const edge of response.edges) {
    const sourceID = canonicalIDByIncomingID.get(edge.source_id) ?? edge.source_id;
    const destinationID = canonicalIDByIncomingID.get(edge.destination_id) ?? edge.destination_id;
    if (sourceID === destinationID) continue;
    const mergedEdge = { ...edge, source_id: sourceID, destination_id: destinationID };
    edgeByKey.set(edgeKey(mergedEdge), mergedEdge);
  }

  return {
    ...graph,
    nodes: Array.from(nodeByID.values()),
    edges: Array.from(edgeByKey.values()),
  };
}

function isPRGraph(graph: UnifiedGraph): graph is GraphResponse {
  return "pr" in graph;
}

function positionsFromNodes(nodes: Node[]): Record<string, Point> {
  const positions: Record<string, Point> = {};
  nodes.forEach((node) => {
    positions[node.id] = { x: node.position.x, y: node.position.y };
  });
  return positions;
}

function nodeMatchesRenameSource(node: APIGraphNode, oldFullName: string, oldFilePath: string): boolean {
  if (node.file_path !== oldFilePath) return false;
  return node.full_name === oldFullName || node.full_name.endsWith(`.${oldFullName}`);
}

function collapseRenamedGraphNodes(graph: UnifiedGraph): UnifiedGraph {
  if (!isPRGraph(graph)) return graph;

  const oldIDToRenamedID = new Map<string, string>();
  const renamedNodes = graph.nodes.filter((node) => node.change_type === "renamed" && node.old_full_name && node.old_file_path);

  for (const renamedNode of renamedNodes) {
    const oldFullName = renamedNode.old_full_name;
    const oldFilePath = renamedNode.old_file_path;
    if (!oldFullName || !oldFilePath) continue;

    const oldNode = graph.nodes.find((node) => (
      node.id !== renamedNode.id
      && !node.change_type
      && nodeMatchesRenameSource(node, oldFullName, oldFilePath)
    ));
    if (oldNode) oldIDToRenamedID.set(oldNode.id, renamedNode.id);
  }

  if (oldIDToRenamedID.size === 0) return graph;

  const remapID = (id: string) => oldIDToRenamedID.get(id) ?? id;
  const seenEdges = new Set<string>();

  return {
    ...graph,
    nodes: graph.nodes.filter((node) => !oldIDToRenamedID.has(node.id)),
    edges: graph.edges.flatMap((edge) => {
      const sourceID = remapID(edge.source_id);
      const destinationID = remapID(edge.destination_id);
      if (sourceID === destinationID) return [];

      const key = `${sourceID}->${destinationID}`;
      if (seenEdges.has(key)) return [];
      seenEdges.add(key);

      return [{ ...edge, source_id: sourceID, destination_id: destinationID }];
    }),
  };
}

function sameTestEntry(test: APIGraphNode, ref: { full_name: string; file_path: string }): boolean {
  return test.full_name === ref.full_name && test.file_path === ref.file_path;
}

function buildTestFocusedGraph(graph: UnifiedGraph, testNode: APIGraphNode | null): UnifiedGraph {
  if (!isPRGraph(graph) || !testNode) return graph;

  const testChanges = graph.test_changes ?? [];
  const testChangeByID = new Map(testChanges.map((node) => [node.id, node]));
  const reachableTestIDs = new Set([testNode.id]);
  const queue = [testNode.id];

  for (let head = 0; head < queue.length; head++) {
    const currentID = queue[head];
    for (const edge of graph.edges) {
      if (edge.source_id !== currentID || !testChangeByID.has(edge.destination_id) || reachableTestIDs.has(edge.destination_id)) {
        continue;
      }
      reachableTestIDs.add(edge.destination_id);
      queue.push(edge.destination_id);
    }
  }

  const testNodes = Array.from(reachableTestIDs)
    .map((id) => testChangeByID.get(id))
    .filter((node): node is APIGraphNode => Boolean(node))
    .map((node, index) => ({
      ...node,
      graph_depth: index === 0 ? 0 : 1,
      boundary: false,
    }));
  const reachableTestNodeIDs = new Set(testNodes.map((node) => node.id));
  const testContext = graph.test_context ?? [];
  const directTargets = graph.nodes.filter((node) => (node.tests ?? []).some((test) => sameTestEntry(testNode, test)));
  const edgeTargetPool = [...graph.nodes, ...testContext];
  const edgeTargets = edgeTargetPool.filter((node) => graph.edges.some((edge) => reachableTestNodeIDs.has(edge.source_id) && edge.destination_id === node.id));
  const targetByID = new Map([...directTargets, ...edgeTargets].map((node) => [node.id, node]));
  const targets = Array.from(targetByID.values());
  const nodeIDs = new Set([...reachableTestNodeIDs, ...targets.map((node) => node.id)]);
  const edges = graph.edges
    .filter((edge) => nodeIDs.has(edge.source_id) && nodeIDs.has(edge.destination_id))
    .map((edge) => {
      const caller = testChangeByID.get(edge.source_id);
      return caller?.change_type === "added" ? { ...edge, change_type: "added" as const } : edge;
    });

  const nodes = [
    ...testNodes.map((node) => ({
      ...node,
      degree: edges.filter((edge) => edge.source_id === node.id || edge.destination_id === node.id).length,
      weight: Math.max(node.weight ?? 0, targets.length),
    })),
    ...targets.map((node) => ({ ...node, graph_depth: 2, boundary: false })),
  ];

  return {
    ...graph,
    nodes,
    edges,
  };
}

function InnerCanvas({
  graph,
  repoID,
  token,
  repo,
  prs = [],
}: {
  graph: UnifiedGraph;
  repoID: string;
  token: string;
  repo?: Repository;
  prs?: QueuePR[];
}) {
  const { fitView, getNode, getZoom, setCenter } = useReactFlow();
  const [activeGraph, setActiveGraph] = useState<UnifiedGraph>(graph);
  const [prGraphCache, setPRGraphCache] = useState<Record<number, GraphResponse>>({});
  const [loadingPRNumber, setLoadingPRNumber] = useState<number | null>(null);
  const [selectedNode, setSelectedNode] = useState<APIGraphNode | null>(null);
  const [selectedPRChange, setSelectedPRChange] = useState<SelectedPRChange | null>(null);
  const [panelMode, setPanelMode] = useState<PanelMode>("overview");
  const [panelWidth, setPanelWidth] = useState(PANEL_DEFAULT_WIDTH);
  const [componentPanelWidth, setComponentPanelWidth] = useState(COMPONENT_PANEL_DEFAULT_WIDTH);
  const [nodeCodeCache, setNodeCodeCache] = useState<Record<string, NodeCodeResponse>>({});
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [layoutPositions, setLayoutPositions] = useState<Record<string, Point>>({});
  const layoutPositionsRef = useRef<Record<string, Point>>({});
  const pendingLayoutAnchorRef = useRef<Point | undefined>(undefined);
  const pendingPinnedIDsRef = useRef<Set<string>>(new Set());
  const suppressNextFitViewRef = useRef(false);
  const selectionZoomRef = useRef<number | null>(null);
  const previousComponentPanelWidthRef = useRef(componentPanelWidth);
  const [expandedEdgeKeys, setExpandedEdgeKeys] = useState<Set<string>>(() => new Set());
  const [expandingEdgeKey, setExpandingEdgeKey] = useState<string | null>(null);
  const [expandingNodeIDs, setExpandingNodeIDs] = useState<Record<string, boolean>>({});
  const [expandedNodeIDs, setExpandedNodeIDs] = useState<Record<string, boolean>>({});
  const selectedNodeIDRef = useRef<string | null>(null);
  const baseVisibleGraph = useMemo(() => collapseRenamedGraphNodes(activeGraph), [activeGraph]);
  const activeGraphKey = useMemo(() => graphContextKey(activeGraph), [activeGraph]);
  const activePRTestChanges = useMemo(
    () => isPRGraph(activeGraph) ? activeGraph.test_changes ?? [] : [],
    [activeGraph]
  );
  const selectedTestNode = selectedPRChange?.type === "node"
    ? activePRTestChanges.find((node) => node.id === selectedPRChange.nodeID) ?? null
    : null;
  const visibleGraph = useMemo(
    () => buildTestFocusedGraph(baseVisibleGraph, selectedTestNode),
    [baseVisibleGraph, selectedTestNode]
  );

  const selectGraphNode = useCallback((id: string) => {
    selectionZoomRef.current = getZoom();
    const apiNode = visibleGraph.nodes.find((n) => n.id === id)
      ?? baseVisibleGraph.nodes.find((n) => n.id === id)
      ?? activePRTestChanges.find((n) => n.id === id)
      ?? null;
    setSelectedNode(apiNode);
    if (!apiNode) {
      setSelectedPRChange(null);
      return;
    }
    if (isPRGraph(baseVisibleGraph) && apiNode) {
      setSelectedPRChange({ type: "node", nodeID: apiNode.id });
    } else {
      setSelectedPRChange(null);
    }
  }, [activePRTestChanges, baseVisibleGraph, getZoom, visibleGraph]);

  const initialNodes: Node[] = useMemo(() => visibleGraph.nodes.map((n) => ({
    id: n.id,
    type: "graphNode",
    data: { node: n, onSelectType: selectGraphNode },
    position: { x: 0, y: 0 },
  })), [visibleGraph.nodes, selectGraphNode]);

  const baseEdges: Edge[] = useMemo(() => visibleGraph.edges.map((e, idx) => ({
    id: `e${idx}`,
    source: e.source_id,
    target: e.destination_id,
    type: "smartBezier",
    data: { weight: e.weight, underlyingEdgeCount: e.underlying_edge_count },
  })), [visibleGraph.edges]);

  const [nodes, setNodes, onNodesChange] = useNodesState(
    edgeLengthLayout(initialNodes, baseEdges)
  );

  // Recompute edge styles whenever selection changes
  const edges: Edge[] = useMemo(() => {
    const selID = selectedNode?.id ?? null;
    return baseEdges.map((e) => {
      const isConnected = selID && (e.source === selID || e.target === selID);
      const isDimmed = selID && !isConnected;
      const apiEdge = visibleGraph.edges.find((edge) => edge.source_id === e.source && edge.destination_id === e.target);
      const baseColor = apiEdge?.change_type === "added" ? "#16A34A" : apiEdge?.change_type === "deleted" ? "#EF4444" : "#888888";
      const dimmedColor = apiEdge?.change_type === "added" ? "#BBF7D0" : apiEdge?.change_type === "deleted" ? "#FECACA" : "#CCCCCC";
      const color = isConnected ? (apiEdge?.change_type === "added" || apiEdge?.change_type === "deleted" ? baseColor : "#333333") : isDimmed ? dimmedColor : baseColor;
      const weightedWidth = Math.min(5, 1 + Math.log2(1 + (apiEdge?.weight ?? 1)) * 0.6);
      const width = isConnected ? Math.max(2, weightedWidth) : weightedWidth;
      const dash = apiEdge?.change_type === "deleted" ? "6 5" : apiEdge?.edge_kind === "owns_method" ? "3 4" : undefined;
      return {
        ...e,
        style: { stroke: color, strokeWidth: width, strokeDasharray: dash },
        markerEnd: { type: MarkerType.ArrowClosed, width: 10, height: 10, color },
      };
    });
  }, [visibleGraph.edges, baseEdges, selectedNode]);

  const onEdgesChange = useCallback(() => {}, []);

  useEffect(() => {
    selectedNodeIDRef.current = selectedNode?.id ?? null;
  }, [selectedNode?.id]);

  useEffect(() => {
    setActiveGraph(graph);
    layoutPositionsRef.current = {};
    setLayoutPositions({});
    setExpandedEdgeKeys(new Set());
  }, [graph]);

  useEffect(() => {
    setNodes((current) => {
      const previousPositions = Object.keys(layoutPositionsRef.current).length > 0
        ? layoutPositionsRef.current
        : positionsFromNodes(current);
      const next = edgeLengthLayout(initialNodes, baseEdges, {
        previousPositions,
        anchor: pendingLayoutAnchorRef.current,
        pinnedIDs: pendingPinnedIDsRef.current,
        iterations: Object.keys(previousPositions).length > 0 ? 90 : 220,
      });
      pendingLayoutAnchorRef.current = undefined;
      pendingPinnedIDsRef.current = new Set();
      const nextPositions = positionsFromNodes(next);
      layoutPositionsRef.current = nextPositions;
      setLayoutPositions(nextPositions);
      const selectedID = selectedNodeIDRef.current;
      return selectedID ? next.map((node) => ({ ...node, selected: node.id === selectedID })) : next;
    });
    if (suppressNextFitViewRef.current) {
      suppressNextFitViewRef.current = false;
      return;
    }
    const timeout = window.setTimeout(() => fitView({ padding: 0.15 }), 50);
    return () => window.clearTimeout(timeout);
  }, [visibleGraph, baseEdges, fitView, initialNodes, setNodes]);

  useEffect(() => {
    const selectedID = selectedNode?.id ?? null;
    setNodes((current) => {
      let changed = false;
      const next = current.map((node) => {
        const selected = node.id === selectedID;
        if (node.selected === selected) return node;
        changed = true;
        return { ...node, selected };
      });
      return changed ? next : current;
    });
    if (!selectedID) return;

    const timeout = window.setTimeout(() => {
      const node = getNode(selectedID);
      if (!node) return;
      const width = node.measured?.width ?? node.width ?? 180;
      const height = node.measured?.height ?? node.height ?? 90;
      const zoom = selectionZoomRef.current ?? getZoom();
      selectionZoomRef.current = null;
      setCenter(node.position.x + width / 2, node.position.y + height / 2, { zoom, duration: 450 });
    }, 120);
    return () => window.clearTimeout(timeout);
  }, [getNode, getZoom, selectedNode?.id, setCenter, setNodes]);

  useEffect(() => {
    setSelectedNode(null);
    setSelectedPRChange(null);
    setPanelMode("overview");
    setExpandingNodeIDs({});
    setExpandedNodeIDs({});
  }, [activeGraphKey]);

  const expandGraphNode = useCallback(async (nodeID: string) => {
    const node = baseVisibleGraph.nodes.find((n) => n.id === nodeID);
    if (!node || expandingNodeIDs[nodeID] || (expandedNodeIDs[nodeID] && !node.boundary)) return;

    const contextKey = graphContextKey(activeGraph);
    const position = layoutPositionsRef.current[nodeID] ?? getNode(nodeID)?.position;
    if (position) pendingLayoutAnchorRef.current = position;
    pendingPinnedIDsRef.current = new Set([nodeID]);
    setExpandingNodeIDs((current) => ({ ...current, [nodeID]: true }));
    try {
      const response = await apiFetch<GraphExpansionResponse>(`/api/v1/repos/${repoID}/graph/expand`, token, {
        method: "POST",
        body: JSON.stringify({
          node_id: nodeID,
          visible_node_ids: baseVisibleGraph.nodes.map((n) => n.id),
          graph_context: isPRGraph(activeGraph)
            ? { mode: "pr", pr_id: activeGraph.pr.id }
            : { mode: "repo" },
        }),
      });
      setExpandedNodeIDs((current) => ({ ...current, [nodeID]: true }));
      suppressNextFitViewRef.current = true;
      setActiveGraph((current) => {
        if (graphContextKey(current) !== contextKey) return current;
        return mergeExpansionGraph(current, response);
      });
    } catch (error) {
      console.error("Graph expansion failed", error);
    } finally {
      setExpandingNodeIDs((current) => {
        const next = { ...current };
        delete next[nodeID];
        return next;
      });
    }
  }, [activeGraph, baseVisibleGraph.nodes, expandedNodeIDs, expandingNodeIDs, getNode, repoID, token]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_, node) => {
      selectGraphNode(node.id);
      void expandGraphNode(node.id);
    },
    [expandGraphNode, selectGraphNode]
  );

  const onEdgeClick: EdgeMouseHandler = useCallback(async (_, edge) => {
    if (!isPRGraph(activeGraph)) {
      return;
    }
    const apiEdge = visibleGraph.edges.find((candidate) => candidate.source_id === edge.source && candidate.destination_id === edge.target);
    if (!apiEdge) return;

    const key = edgeKey(apiEdge);
    if (expandedEdgeKeys.has(key) || expandingEdgeKey === key) return;

    const sourcePosition = layoutPositionsRef.current[edge.source] ?? getNode(edge.source)?.position;
    const targetPosition = layoutPositionsRef.current[edge.target] ?? getNode(edge.target)?.position;
    if (sourcePosition && targetPosition) {
      pendingLayoutAnchorRef.current = {
        x: (sourcePosition.x + targetPosition.x) / 2,
        y: (sourcePosition.y + targetPosition.y) / 2,
      };
    }
    pendingPinnedIDsRef.current = new Set([edge.source, edge.target]);
    setExpandingEdgeKey(key);
    try {
      const visibleNodeIDs = baseVisibleGraph.nodes.map((node) => node.id);
      const requestExpansion = (nodeID: string) => apiFetch<GraphExpansionResponse>(`/api/v1/repos/${repoID}/graph/expand`, token, {
        method: "POST",
        body: JSON.stringify({
          node_id: nodeID,
          visible_node_ids: visibleNodeIDs,
          graph_context: { mode: "pr", pr_id: activeGraph.pr.id },
        }),
      });
      const [sourceExpansion, targetExpansion] = await Promise.all([
        requestExpansion(edge.source),
        requestExpansion(edge.target),
      ]);
      setExpandedEdgeKeys((current) => new Set([...current, key]));
      setExpandedNodeIDs((current) => ({ ...current, [edge.source]: true, [edge.target]: true }));
      const contextKey = graphContextKey(activeGraph);
      suppressNextFitViewRef.current = true;
      setActiveGraph((current) => {
        if (graphContextKey(current) !== contextKey) return current;
        return mergeExpansionGraph(mergeExpansionGraph(current, sourceExpansion), targetExpansion);
      });
    } finally {
      setExpandingEdgeKey(null);
    }
  }, [activeGraph, baseVisibleGraph.nodes, expandedEdgeKeys, expandingEdgeKey, getNode, repoID, token, visibleGraph.edges]);

  const onPaneClick = useCallback(() => {
    setSelectedNode(null);
    setSelectedPRChange(null);
    setPanelMode("overview");
  }, []);

  const onResizePanel = useCallback((nextWidth: number) => {
    setPanelWidth(Math.max(PANEL_MIN_WIDTH, Math.min(PANEL_MAX_WIDTH, nextWidth)));
  }, []);

  const onResizeComponentPanel = useCallback((nextWidth: number) => {
    setComponentPanelWidth(Math.max(COMPONENT_PANEL_MIN_WIDTH, Math.min(COMPONENT_PANEL_MAX_WIDTH, nextWidth)));
  }, []);

  useEffect(() => {
    const previousWidth = previousComponentPanelWidthRef.current;
    previousComponentPanelWidthRef.current = componentPanelWidth;
    if (!selectedPRChange || settingsOpen || previousWidth === componentPanelWidth) return;
    const timeout = window.setTimeout(() => fitView({ padding: 0.15 }), 60);
    return () => window.clearTimeout(timeout);
  }, [componentPanelWidth, fitView, selectedPRChange, settingsOpen]);

  const onCacheNodeCode = useCallback((nodeID: string, code: NodeCodeResponse) => {
    setNodeCodeCache((current) => current[nodeID] ? current : { ...current, [nodeID]: code });
  }, []);

  const onSelectPR = useCallback(async (prNumber: number) => {
    setSettingsOpen(false);
    setSelectedPRChange(null);
    setExpandedEdgeKeys(new Set());
    layoutPositionsRef.current = {};
    setLayoutPositions({});
    const cached = prGraphCache[prNumber];
    if (cached) {
      setActiveGraph(cached);
      return;
    }

    setLoadingPRNumber(prNumber);
    try {
      const prGraph = await apiFetch<GraphResponse>(`/api/v1/repos/${repoID}/prs/number/${prNumber}/graph`, token);
      setPRGraphCache((current) => ({ ...current, [prNumber]: prGraph }));
      setActiveGraph(prGraph);
    } finally {
      setLoadingPRNumber(null);
    }
  }, [prGraphCache, repoID, token]);

  const onBackToRepo = useCallback(() => {
    setSettingsOpen(false);
    setSelectedPRChange(null);
    setExpandedEdgeKeys(new Set());
    layoutPositionsRef.current = {};
    setLayoutPositions({});
    setActiveGraph(graph);
  }, [graph]);

  const totalNodes = visibleGraph.nodes.length;
  const maxNodes = 20;
  const activeRepo = repo ?? (isPRGraph(activeGraph) ? fallbackRepo(repoID) : activeGraph.repo);
  const activePR = isPRGraph(activeGraph) ? activeGraph.pr : undefined;
  const activePRFiles = isPRGraph(activeGraph) ? activeGraph.files ?? [] : [];
  const detailNodes = isPRGraph(baseVisibleGraph)
    ? uniqueGraphNodes([...baseVisibleGraph.nodes, ...visibleGraph.nodes, ...activePRTestChanges, ...(baseVisibleGraph.test_context ?? [])])
    : visibleGraph.nodes;

  return (
    <div data-layout-positions={Object.keys(layoutPositions).length} style={{ display: "flex", height: "100vh", width: "100vw", position: "relative" }}>
      <NodeDetailPanel
        node={activePR ? null : selectedNode}
        allNodes={baseVisibleGraph.nodes}
        edges={visibleGraph.edges}
        onSelectNode={(id) => {
          selectGraphNode(id);
        }}
        onSelectPRChange={(change) => {
          setSelectedPRChange(change);
          if (change.type === "node") {
            setSelectedNode(detailNodes.find((node) => node.id === change.nodeID) ?? null);
          } else {
            setSelectedNode(null);
          }
          setPanelMode("overview");
        }}
        repoID={repoID}
        repo={activeRepo}
        pr={activePR}
        prFiles={activePRFiles}
        testChanges={activePRTestChanges}
        prs={prs}
        loadingPRNumber={loadingPRNumber}
        onSelectPR={onSelectPR}
        onBackToRepo={onBackToRepo}
        token={token}
        nodeCodeCache={nodeCodeCache}
        onCacheNodeCode={onCacheNodeCode}
        width={panelWidth}
        minWidth={PANEL_MIN_WIDTH}
        maxWidth={PANEL_MAX_WIDTH}
        onResize={onResizePanel}
        mode={panelMode}
        onModeChange={setPanelMode}
        onViewCode={() => {
          setPanelMode("code");
        }}
        onOpenSettings={() => {
          setSettingsOpen(true);
          setSelectedNode(null);
          setSelectedPRChange(null);
          setPanelMode("overview");
        }}
      />

      {activePR && selectedPRChange && !settingsOpen && (
        <ComponentChangePanel
          selectedChange={selectedPRChange}
          allNodes={detailNodes}
          edges={visibleGraph.edges}
          files={activePRFiles}
          repoID={repoID}
          prID={activePR.id}
          token={token}
          nodeCodeCache={nodeCodeCache}
          onCacheNodeCode={onCacheNodeCode}
          onSelectNode={(id) => {
            selectGraphNode(id);
            setSelectedPRChange({ type: "node", nodeID: id });
          }}
          onClose={() => {
            setSelectedPRChange(null);
            setSelectedNode(null);
            setPanelMode("overview");
          }}
          width={componentPanelWidth}
          minWidth={COMPONENT_PANEL_MIN_WIDTH}
          maxWidth={COMPONENT_PANEL_MAX_WIDTH}
          onResize={onResizeComponentPanel}
        />
      )}

      <div style={{ flex: 1, background: "#EBE9E9", position: "relative" }}>
        {settingsOpen ? (
          <div style={{ position: "absolute", inset: 0, overflowY: "auto" }}>
            <SettingsView
              account={activeRepo.full_name.split("/")[0] || "settings"}
              embedded
              onClose={() => setSettingsOpen(false)}
            />
          </div>
        ) : (
        <div style={{ position: "absolute", inset: 0 }}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeClick={onNodeClick}
            onEdgeClick={onEdgeClick}
            onPaneClick={onPaneClick}
            connectionMode={ConnectionMode.Loose}
            fitView
            panOnScroll
            panOnScrollMode={PanOnScrollMode.Free}
            minZoom={0.15}
            maxZoom={1}
            proOptions={{ hideAttribution: true }}
          >
            <Background color="#D8D6D6" gap={20} size={1} />
          </ReactFlow>

          <div style={{
            position: "absolute",
            bottom: 64,
            right: 24,
            display: "flex",
            flexDirection: "column",
            gap: 4,
            zIndex: 10,
          }}>
            <ZoomControls onFit={() => fitView({ padding: 0.15 })} />
          </div>

          {totalNodes >= maxNodes && (
            <div style={{
              position: "absolute",
              bottom: 64,
              left: 24,
              color: "#888888",
              fontSize: 12,
              zIndex: 10,
            }}>
              Showing {totalNodes} affected functions
            </div>
          )}

        </div>
        )}
      </div>

      <BetaFeedbackBanner
        token={token}
        repo={activeRepo}
        pr={activePR}
        selectedNode={selectedNode}
      />
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

export default function GraphCanvas({
  graph,
  repoID,
  token,
  repo,
  prs,
}: {
  graph: UnifiedGraph;
  repoID: string;
  token: string;
  repo?: Repository;
  prs?: QueuePR[];
}) {
  return (
    <ReactFlowProvider>
      <InnerCanvas graph={graph} repoID={repoID} token={token} repo={repo} prs={prs} />
    </ReactFlowProvider>
  );
}

function fallbackRepo(repoID: string): Repository {
  return {
    id: repoID,
    user_id: "",
    installation_id: "",
    github_repo_id: 0,
    full_name: "",
    default_branch: "main",
    index_status: "ready",
    is_active: true,
    created_at: "",
  };
}

function uniqueGraphNodes(nodes: APIGraphNode[]): APIGraphNode[] {
  const seen = new Set<string>();
  const seenSemantic = new Set<string>();
  const result: APIGraphNode[] = [];
  for (const node of nodes) {
    const semanticKey = graphNodeSemanticKey(node);
    if (seen.has(node.id) || seenSemantic.has(semanticKey)) continue;
    seen.add(node.id);
    seenSemantic.add(semanticKey);
    result.push(node);
  }
  return result;
}
