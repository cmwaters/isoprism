import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// read reads a local file as UTF-8 text for the parity test.
function read(path: string) {
  return readFileSync(new URL(path, import.meta.url), "utf8");
}

describe("local viewer parity", () => {
  it("uses the same repo graph surface as the Next local route", () => {
    expect(read("../app/local/page.tsx")).toContain('from "@/components/local/local-repo-graph"');
    expect(read("./src/main.tsx")).toContain('from "@/components/local/local-repo-graph"');
  });

  it("loads the app shell styles that Next normally applies around the shared component", () => {
    const entry = read("./src/main.tsx");
    const shell = read("./src/shell.css");

    expect(entry).toContain('import "@/app/globals.css"');
    expect(entry).toContain('import "./shell.css"');
    expect(shell).toContain('font-family: "Geist"');
    expect(shell).toContain("--font-sans");
    expect(shell).toContain("--font-geist-mono");
  });
});
