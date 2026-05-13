import { describe, expect, it } from "vitest";
import { expansionOptionsForVisibleSelection, isTypeNode } from "./graph-selection";

describe("graph selection expansion", () => {
  it("requests panel-only expansion for visible type nodes selected from relations", () => {
    expect(expansionOptionsForVisibleSelection({ kind: "struct" })).toEqual({ detailOnlyForTypes: true });
    expect(expansionOptionsForVisibleSelection({ kind: "interface" })).toEqual({ detailOnlyForTypes: true });
    expect(expansionOptionsForVisibleSelection({ kind: "type" })).toEqual({ detailOnlyForTypes: true });
    expect(expansionOptionsForVisibleSelection({ kind: "class" })).toEqual({ detailOnlyForTypes: true });
  });

  it("keeps function and method selections as graph expansions", () => {
    expect(expansionOptionsForVisibleSelection({ kind: "function" })).toEqual({ detailOnlyForTypes: false });
    expect(expansionOptionsForVisibleSelection({ kind: "method" })).toEqual({ detailOnlyForTypes: false });
  });

  it("does not classify missing nodes as type nodes", () => {
    expect(isTypeNode(null)).toBe(false);
    expect(isTypeNode(undefined)).toBe(false);
  });
});
