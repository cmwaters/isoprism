import { API_URL } from "@/lib/api";
import GraphCanvas from "@/components/graph/graph-canvas";
import { QueueResponse, RepoGraphResponse, RepoProgramsResponse, Repository } from "@/lib/types";

export const dynamic = "force-dynamic";

export default async function LocalRepoPage() {
  const repo = await localFetch<Repository>("/api/v1/local/repo");
  const queue = await localFetch<QueueResponse>("/api/v1/repos/local/queue").catch(() => ({ prs: [] }));
  const programs = await localFetch<RepoProgramsResponse>("/api/v1/repos/local/programs").catch(() => ({ repo, programs: [] }));
  const graph: RepoGraphResponse = { repo, programs: programs.programs, nodes: [], edges: [] };

  return (
    <>
      <script
        dangerouslySetInnerHTML={{
          __html: `window.__ISOPRISM_API_URL__=${JSON.stringify(API_URL)};`,
        }}
      />
      <GraphCanvas
        graph={graph}
        prs={queue.prs}
        repoID="local"
        repo={repo}
        token="local"
        settingsHref={null}
        showFeedbackBanner={false}
      />
    </>
  );
}

async function localFetch<T>(path: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Local Isoprism API error ${res.status}: ${await res.text()}`);
  }
  return res.json() as Promise<T>;
}
