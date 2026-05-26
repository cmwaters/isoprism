"use client";

import GraphCanvas from "@/components/graph/graph-canvas";
import type { QueueResponse, RepoGraphResponse, Repository } from "@/lib/types";

export default function LocalRepoGraph({
  graph,
  prs,
  repo,
}: {
  graph: RepoGraphResponse;
  prs: QueueResponse["prs"];
  repo: Repository;
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
    />
  );
}
