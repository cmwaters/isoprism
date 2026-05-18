import { describe, expect, it } from "vitest";
import type { GraphNode } from "@/lib/types";
import { symbolContextLabel } from "./symbol-format";

const baseNode: GraphNode = {
  id: "node-1",
  full_name: "main.store.save",
  file_path: "main.go",
  package_path: ".",
  line_start: 1,
  line_end: 2,
  inputs: [],
  outputs: [],
  language: "go",
  kind: "method",
  is_test: false,
  is_entrypoint: false,
  node_type: "changed",
  summary: null,
  change_type: "modified",
  lines_added: 0,
  lines_removed: 0,
  weight: 0,
  degree: 0,
  graph_depth: 0,
  boundary: false,
  tests: [],
};

describe("symbol formatting", () => {
  it("uses the root file package label instead of rendering dot package paths", () => {
    expect(symbolContextLabel(baseNode)).toBe("main.store");
  });
});
