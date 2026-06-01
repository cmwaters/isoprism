"use client";

import GraphCanvas from "@/components/graph/graph-canvas";
import type { QueueResponse, RepoGraphResponse, Repository } from "@/lib/types";

// LocalRepoGraph renders the shared graph UI for local repo data.
export default function LocalRepoGraph({
  graph,
  prs,
  repo,
  loadingReviewItems = false,
  loadingPrograms = false,
}: {
  graph: RepoGraphResponse;
  prs: QueueResponse["prs"];
  repo: Repository;
  loadingReviewItems?: boolean;
  loadingPrograms?: boolean;
}) {
  return (
    <GraphCanvas
      graph={graph}
      prs={prs}
      repoID="local"
      repo={repo}
      token="local"
      settingsHref={null}
      showFeedbackBanner={false}
      enableLocalReview
      loadingReviewItems={loadingReviewItems}
      loadingPrograms={loadingPrograms}
    />
  );
}
