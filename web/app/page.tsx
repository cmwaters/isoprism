import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";

export const dynamic = "force-dynamic";

export default async function RootPage() {
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();

  if (!user) redirect("/login");

  const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

  // Use the same redirect helper as the auth callback so returning users
  // land on the right screen even if the client-side repo fetch would fail.
  try {
    const res = await fetch(`${apiUrl}/api/v1/auth/status?user_id=${user.id}`, {
      cache: "no-store",
    });

    if (res.ok) {
      const { redirect: redirectPath } = await res.json();
      if (typeof redirectPath === "string" && redirectPath.length > 0) {
        redirect(redirectPath);
      }
    }
  } catch {
    // Fall through to the onboarding screen below.
  }

  redirect("/onboarding");
}
