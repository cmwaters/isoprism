import { API_URL } from "@/lib/api";
import LocalRepoGraph from "@/components/local/local-repo-graph";
import { QueueResponse, RepoGraphResponse, RepoProgramsResponse } from "@/lib/types";

export const dynamic = "force-dynamic";

export default async function LocalRepoPage() {
  const [programs, queue] = await Promise.all([
    localFetch<RepoProgramsResponse>("/api/v1/repos/local/programs"),
    localFetch<QueueResponse>("/api/v1/repos/local/queue").catch(() => ({ prs: [] })),
  ]);
  const repo = programs.repo;
  const graph: RepoGraphResponse = { repo, programs: programs.programs, nodes: [], edges: [] };

  return <LocalRepoGraph graph={graph} prs={queue.prs} repo={repo} />;
}

async function localFetch<T>(path: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Local Isoprism API error ${res.status}: ${await res.text()}`);
  }
  return res.json() as Promise<T>;
}
