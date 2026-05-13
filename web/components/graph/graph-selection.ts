type GraphSelectionNode = {
  kind?: string;
} | null | undefined;

export type GraphExpansionOptions = {
  detailOnlyForTypes?: boolean;
  forceGraphMerge?: boolean;
};

export function isTypeNode(node: GraphSelectionNode): boolean {
  if (!node) return false;
  return ["class", "interface", "struct", "type"].includes(node.kind ?? "");
}

export function expansionOptionsForVisibleSelection(node: GraphSelectionNode): GraphExpansionOptions {
  return { detailOnlyForTypes: isTypeNode(node) };
}
