import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { Repository } from "@/lib/types";

export const dynamic = "force-dynamic";

export default async function RootPage() {
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();

  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  // Check for an active repo
  try {
    const { repos } = await apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token);
    if (repos && repos.length > 0) {
      // Prefer a ready repo
      const ready = repos.find((r) => r.index_status === "ready");
      if (ready) redirect(`/repos/${ready.id}`);
      // Otherwise go to onboarding/repos to trigger indexing
      redirect("/onboarding/repos");
    }
  } catch {
    // No repos yet
  }

  redirect("/onboarding");
}
