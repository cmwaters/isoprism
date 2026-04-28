import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";

export const dynamic = "force-dynamic";

export default async function RootPage() {
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();

  if (!user) redirect("/login");

  const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
  let redirectPath = "/login";

  // Direct site visits are login-first. The auth callback is the place where
  // a signed-in user with no connected repos is sent to GitHub App install.
  try {
    const res = await fetch(`${apiUrl}/api/v1/auth/status?user_id=${user.id}`, {
      cache: "no-store",
    });

    if (res.ok) {
      const { redirect } = await res.json();
      if (typeof redirect === "string" && redirect.length > 0) {
        redirectPath = redirect === "/onboarding" ? "/login" : redirect;
      }
    }
  } catch {
    // Fall through to the login screen below.
  }

  redirect(redirectPath);
}
