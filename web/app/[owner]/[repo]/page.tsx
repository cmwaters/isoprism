import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { QueueResponse, RepoGraphResponse, RepoProgramsResponse, Repository } from "@/lib/types";
import GraphCanvas from "@/components/graph/graph-canvas";

export const dynamic = "force-dynamic";

// Props describes the props consumed by this component.
interface Props {
  params: Promise<{ owner: string; repo: string }>;
}

// CanonicalRepoPage renders the canonical repo page for canonical repository routing.
export default async function CanonicalRepoPage({ params }: Props) {
  const { owner, repo: repoName } = await params;
  if (repoName.toLowerCase() === "settings") redirect("/settings");

  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  const repo = await findRepoByFullName(`${owner}/${repoName}`, token);
  if (!repo) redirect("/");

  let queue: QueueResponse = { prs: [] };
  let programs: RepoProgramsResponse = { repo, programs: [] };

  try {
    queue = await apiFetch<QueueResponse>(`/api/v1/repos/${repo.id}/queue`, token);
  } catch {
    // queue may be empty while indexing or if no ready PRs exist
  }

  try {
    programs = await apiFetch<RepoProgramsResponse>(`/api/v1/repos/${repo.id}/programs`, token);
  } catch {
    // program list may be empty while indexing
  }

  const graph: RepoGraphResponse = { repo, programs: programs.programs, nodes: [], edges: [] };

  return <GraphCanvas graph={graph} prs={queue.prs} repoID={repo.id} repo={repo} token={token} />;
}

// findRepoByFullName finds repo by full name for canonical repository routing.
async function findRepoByFullName(fullName: string, token: string): Promise<Repository | null> {
  const { repos } = await apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token);
  return repos.find((repo) => repo.full_name.toLowerCase() === fullName.toLowerCase()) ?? null;
}
