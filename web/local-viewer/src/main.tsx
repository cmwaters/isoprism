import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import LocalRepoGraph from "@/components/local/local-repo-graph";
import type { GraphProgram, QueueResponse, RepoGraphResponse, RepoProgramsResponse, Repository } from "@/lib/types";
import "@/app/globals.css";
import "./shell.css";

// LoadStatus represents one async loading state for the embedded local viewer.
type LoadStatus = "loading" | "ready" | "error";

// LoadState tracks independently loaded programs and review items for the embedded local viewer.
type LoadState = {
  repo: Repository;
  programs: GraphProgram[];
  prs: QueueResponse["prs"];
  programsStatus: LoadStatus;
  reviewStatus: LoadStatus;
  error?: string;
};

// ReviewItemsResponse describes an outbound response for the embedded local viewer.
type ReviewItemsResponse = {
  review_items: QueueResponse["prs"];
};

// localFetch loads JSON from the local viewer API.
async function localFetch<T>(path: string): Promise<T> {
  const res = await fetch(path, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Local Isoprism API error ${res.status}: ${await res.text()}`);
  }
  return res.json() as Promise<T>;
}

// placeholderRepo provides stable repo metadata while the local daemon responses are loading.
const placeholderRepo: Repository = {
  id: "local",
  user_id: "",
  installation_id: "",
  github_repo_id: 0,
  full_name: "",
  default_branch: "main",
  index_status: "ready",
  is_active: true,
  created_at: "",
};

// LocalViewer renders the local viewer for the embedded local viewer.
function LocalViewer() {
  const [state, setState] = useState<LoadState>({
    repo: placeholderRepo,
    programs: [],
    prs: [],
    programsStatus: "loading",
    reviewStatus: "loading",
  });

  useEffect(() => {
    let cancelled = false;

    // loadPrograms loads local repo metadata and program entrypoints.
    async function loadPrograms() {
      try {
        const programs = await localFetch<RepoProgramsResponse>("/api/programs");
        if (cancelled) return;
        setState((current) => ({
          ...current,
          repo: programs.repo,
          programs: programs.programs,
          programsStatus: "ready",
        }));
      } catch (error) {
        if (cancelled) return;
        setState((current) => ({
          ...current,
          programsStatus: "error",
          error: error instanceof Error ? error.message : String(error),
        }));
      }
    }

    // loadReviewItems loads local and GitHub review cards independently of program data.
    async function loadReviewItems() {
      try {
        const queue = await localFetch<ReviewItemsResponse>("/api/review-items");
        if (cancelled) return;
        setState((current) => ({
          ...current,
          prs: queue.review_items,
          reviewStatus: "ready",
        }));
      } catch {
        if (cancelled) return;
        setState((current) => ({
          ...current,
          prs: [],
          reviewStatus: "error",
        }));
      }
    }

    void loadPrograms();
    void loadReviewItems();
    return () => {
      cancelled = true;
    };
  }, []);

  const graph: RepoGraphResponse = { repo: state.repo, programs: state.programs, nodes: [], edges: [] };

  return (
    <>
      <LocalRepoGraph
        graph={graph}
        prs={state.prs}
        repo={state.repo}
        loadingReviewItems={state.reviewStatus === "loading"}
        loadingPrograms={state.programsStatus === "loading"}
      />
      {state.error && (
        <div style={{ bottom: 24, color: "#AA2222", fontSize: 12, left: 400, position: "fixed", zIndex: 40 }}>
          {state.error}
        </div>
      )}
    </>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <LocalViewer />
  </React.StrictMode>
);
