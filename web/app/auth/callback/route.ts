import { createClient } from "@/lib/supabase/server";
import { API_URL } from "@/lib/api";
import { NextRequest, NextResponse } from "next/server";

export async function GET(request: NextRequest) {
  const { searchParams, origin } = new URL(request.url);
  const code = searchParams.get("code");
  const next = searchParams.get("next");
  const pilot = searchParams.get("pilot");

  if (code) {
    const supabase = await createClient();
    const { data, error } = await supabase.auth.exchangeCodeForSession(code);
    if (!error && data.session) {
      // If a specific next URL was provided, use it
      if (next) {
        return NextResponse.redirect(`${origin}${next}`);
      }

      // Otherwise, ask the shared auth-status helper where to send the user.
      const userId = data.session.user.id;
      if (pilot) {
        try {
          await fetch(`${API_URL}/api/v1/pilot/invites/${encodeURIComponent(pilot)}/accept`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ user_id: userId }),
          });
        } catch {
          // Continue through normal auth routing; invalid pilot links are handled operationally.
        }
      }

      try {
        const statusRes = await fetch(
          `${API_URL}/api/v1/auth/status?user_id=${userId}`
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

      return NextResponse.redirect(`${origin}/onboarding`);
    }
  }

  return NextResponse.redirect(`${origin}/login?error=auth_callback_failed`);
}
