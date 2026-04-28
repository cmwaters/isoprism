import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { QueueResponse, RepoGraphResponse, Repository } from "@/lib/types";
import GraphCanvas from "@/components/graph/graph-canvas";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ owner: string; repo: string }>;
}

export default async function CanonicalRepoPage({ params }: Props) {
  const { owner, repo: repoName } = await params;
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  const repo = await findRepoByFullName(`${owner}/${repoName}`, token);
  if (!repo) redirect("/");

  let queue: QueueResponse = { prs: [] };
  let graph: RepoGraphResponse | null = null;

  try {
    queue = await apiFetch<QueueResponse>(`/api/v1/repos/${repo.id}/queue`, token);
  } catch {
    // queue may be empty while indexing or if no ready PRs exist
  }

  try {
    graph = await apiFetch<RepoGraphResponse>(`/api/v1/repos/${repo.id}/graph`, token);
  } catch {
    graph = { repo, nodes: [], edges: [] };
  }

  return <GraphCanvas graph={graph} prs={queue.prs} repoID={repo.id} repo={repo} token={token} />;
}

async function findRepoByFullName(fullName: string, token: string): Promise<Repository | null> {
  const { repos } = await apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token);
  return repos.find((repo) => repo.full_name.toLowerCase() === fullName.toLowerCase()) ?? null;
}
