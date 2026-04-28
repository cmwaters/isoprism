import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { Repository } from "@/lib/types";

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

  try {
    repo = await apiFetch<Repository>(`/api/v1/repos/${repoID}`, token);
  } catch {
    redirect("/");
  }

  redirect(`/${repo.full_name}`);
}
