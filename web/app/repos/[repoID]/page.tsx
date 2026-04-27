import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { QueueResponse, RepoGraphResponse, Repository } from "@/lib/types";
import RepoGraphCanvas from "@/components/graph/repo-graph-canvas";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ repoID: string }>;
}

export default async function RepoQueuePage({ params }: Props) {
  const { repoID } = await params;
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  let repo: Repository | null = null;
  let queue: QueueResponse = { prs: [] };
  let graph: RepoGraphResponse | null = null;

  try {
    repo = await apiFetch<Repository>(`/api/v1/repos/${repoID}`, token);
  } catch {
    redirect("/");
  }

  try {
    queue = await apiFetch<QueueResponse>(`/api/v1/repos/${repoID}/queue`, token);
  } catch {
    // queue may be empty if indexing just finished
  }

  try {
    graph = await apiFetch<RepoGraphResponse>(`/api/v1/repos/${repoID}/graph`, token);
  } catch {
    graph = {
      repo,
      nodes: [],
      edges: [],
    };
  }

  return <RepoGraphCanvas graph={graph} prs={queue.prs} repoID={repoID} />;
}
