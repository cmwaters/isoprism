import type { GraphNode } from "@/lib/types";

// filePackageName returns the package-like path segment for a source file.
function filePackageName(filePath: string): string {
  const parts = filePath.split("/");
  return parts.length >= 2
    ? parts[parts.length - 2]
    : parts[0]?.replace(/\.[^.]+$/, "") ?? "";
}

// packageLabel returns the compact package label for a graph node.
function packageLabel(packagePath: string | undefined, filePath: string): string {
  if (!packagePath || packagePath === ".") return filePackageName(filePath);
  const parts = packagePath.split("/").filter(Boolean);
  return parts[parts.length - 1] || packagePath;
}

// symbolName returns the leaf symbol name from a full graph name.
function symbolName(fullName: string): string {
  const colonIndex = fullName.lastIndexOf(":");
  return colonIndex >= 0 ? fullName.slice(colonIndex + 1) : fullName;
}

// symbolParts splits a graph symbol into displayable name parts.
function symbolParts(node: GraphNode): string[] {
  return symbolName(node.full_name).split(".").filter(Boolean);
}

// symbolTitle returns the main title shown on a graph node.
export function symbolTitle(node: GraphNode): string {
  const parts = symbolParts(node);
  return parts[parts.length - 1] || node.full_name;
}

// symbolContextLabel returns the package or receiver label shown on a graph node.
export function symbolContextLabel(node: GraphNode): string {
  const pkg = packageLabel(node.package_path, node.file_path);
  const parts = symbolParts(node);
  const receiver = node.kind === "method" && parts.length >= 2
    ? parts[parts.length - 2]
    : "";

  if (pkg && receiver) return `${pkg}.${receiver}`;
  return receiver || pkg;
}

// renamedFromTitle returns the previous symbol title for renamed nodes.
export function renamedFromTitle(node: GraphNode): string {
  if (!node.old_full_name) return "";
  const oldNode = { ...node, full_name: node.old_full_name } satisfies GraphNode;
  return symbolTitle(oldNode);
}
