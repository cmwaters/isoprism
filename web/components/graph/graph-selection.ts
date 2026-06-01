// GraphSelectionNode describes a graph node used by the graph review UI.
type GraphSelectionNode = {
  kind?: string;
} | null | undefined;

// GraphExpansionOptions configures the graph review UI.
export type GraphExpansionOptions = {
  detailOnlyForTypes?: boolean;
  forceGraphMerge?: boolean;
};

// isTypeNode reports whether type node matches the expected condition.
export function isTypeNode(node: GraphSelectionNode): boolean {
  if (!node) return false;
  return ["class", "interface", "struct", "type"].includes(node.kind ?? "");
}

// expansionOptionsForVisibleSelection returns graph expansion behavior for the selected node kind.
export function expansionOptionsForVisibleSelection(node: GraphSelectionNode): GraphExpansionOptions {
  return { detailOnlyForTypes: isTypeNode(node) };
}
