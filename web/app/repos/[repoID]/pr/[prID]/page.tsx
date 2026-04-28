import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { GraphResponse, Repository } from "@/lib/types";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ repoID: string; prID: string }>;
}

export default async function PRGraphPage({ params }: Props) {
  const { repoID, prID } = await params;
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  let repo: Repository;
  let graph: GraphResponse;
  try {
    repo = await apiFetch<Repository>(`/api/v1/repos/${repoID}`, token);
    graph = await apiFetch<GraphResponse>(`/api/v1/repos/${repoID}/prs/${prID}/graph`, token);
  } catch {
    redirect(`/repos/${repoID}`);
  }

  redirect(`/${repo!.full_name}/pull/${graph!.pr.number}`);
}
