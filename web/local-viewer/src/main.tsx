import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import LocalRepoGraph from "@/components/local/local-repo-graph";
import type { QueueResponse, RepoGraphResponse, RepoProgramsResponse, Repository } from "@/lib/types";
import "@/app/globals.css";
import "./shell.css";

type LoadState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; graph: RepoGraphResponse; prs: QueueResponse["prs"]; repo: Repository };

type ReviewItemsResponse = {
  review_items: QueueResponse["prs"];
};

async function localFetch<T>(path: string): Promise<T> {
  const res = await fetch(path, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Local Isoprism API error ${res.status}: ${await res.text()}`);
  }
  return res.json() as Promise<T>;
}

function LocalViewer() {
  const [state, setState] = useState<LoadState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const [programs, queue] = await Promise.all([
          localFetch<RepoProgramsResponse>("/api/programs"),
          localFetch<ReviewItemsResponse>("/api/review-items").catch(() => ({ review_items: [] })),
        ]);
        if (cancelled) return;
        setState({
          status: "ready",
          repo: programs.repo,
          prs: queue.review_items,
          graph: { repo: programs.repo, programs: programs.programs, nodes: [], edges: [] },
        });
      } catch (error) {
        if (cancelled) return;
        setState({ status: "error", message: error instanceof Error ? error.message : String(error) });
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  if (state.status === "loading") {
    return <div style={{ padding: 32, color: "#555555" }}>Loading local graph...</div>;
  }

  if (state.status === "error") {
    return <div style={{ padding: 32, color: "#AA2222" }}>{state.message}</div>;
  }

  return <LocalRepoGraph graph={state.graph} prs={state.prs} repo={state.repo} />;
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <LocalViewer />
  </React.StrictMode>
);
