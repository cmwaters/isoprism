import { createClient } from "@/lib/supabase/server";
import { NextRequest, NextResponse } from "next/server";

export async function GET(request: NextRequest) {
  const { searchParams, origin } = new URL(request.url);
  const code = searchParams.get("code");
  const next = searchParams.get("next");

  if (code) {
    const supabase = await createClient();
    const { data, error } = await supabase.auth.exchangeCodeForSession(code);
    if (!error && data.session) {
      // If a specific next URL was provided, use it
      if (next) {
        return NextResponse.redirect(`${origin}${next}`);
      }

      // Otherwise, check org membership to determine where to send the user
      const providerToken = data.session.provider_token;
      const userId = data.session.user.id;
      const apiUrl = process.env.NEXT_PUBLIC_API_URL;

      if (providerToken && apiUrl) {
        try {
          const statusRes = await fetch(
            `${apiUrl}/api/v1/auth/status?github_token=${providerToken}&user_id=${userId}`
          );
          if (statusRes.ok) {
            const { redirect } = await statusRes.json();
            if (redirect) {
              return NextResponse.redirect(`${origin}${redirect}`);
            }
          }
        } catch {
          // Fall through to default
        }
      }

      return NextResponse.redirect(`${origin}/onboarding`);
    }
  }

  return NextResponse.redirect(`${origin}/login?error=auth_callback_failed`);
}
