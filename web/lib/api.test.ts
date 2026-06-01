import { describe, expect, it } from "vitest";

import { apiBaseURL } from "./api";

describe("apiBaseURL", () => {
  it("uses the current origin for embedded local viewer requests", () => {
    const originalWindow = globalThis.window;
    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: {
        location: {
          origin: "http://127.0.0.1:3718",
          pathname: "/",
        },
      },
    });

    try {
      expect(apiBaseURL("local")).toBe("http://127.0.0.1:3718");
    } finally {
      Object.defineProperty(globalThis, "window", {
        configurable: true,
        value: originalWindow,
      });
    }
  });
});
