import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { GraphResponse } from "@/lib/types";
import GraphCanvas from "@/components/graph/graph-canvas";

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

  let graph: GraphResponse;
  try {
    graph = await apiFetch<GraphResponse>(`/api/v1/repos/${repoID}/prs/${prID}/graph`, token);
  } catch {
    redirect(`/repos/${repoID}`);
  }

  return <GraphCanvas graph={graph!} repoID={repoID} />;
}
