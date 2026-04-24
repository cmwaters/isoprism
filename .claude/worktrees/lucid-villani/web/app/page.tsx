import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { Organization } from "@/lib/types";

export const dynamic = "force-dynamic";

export default async function RootPage() {
  const supabase = await createClient();
  const {
    data: { user },
  } = await supabase.auth.getUser();

  if (!user) {
    redirect("/login");
  }

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;

  if (!token) {
    redirect("/login");
  }

  let firstOrgSlug: string | null = null;
  try {
    const { orgs } = await apiFetch<{ orgs: Organization[] }>(
      "/api/v1/me/orgs",
      token
    );

    if (orgs && orgs.length > 0) {
      firstOrgSlug = orgs[0].slug;
    }
  } catch {
    // Fall through to onboarding
  }

  if (firstOrgSlug) {
    redirect(`/orgs/${firstOrgSlug}`);
  }

  redirect("/onboarding");
}
