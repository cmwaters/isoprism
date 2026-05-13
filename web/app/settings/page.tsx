"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";

export default function SettingsRedirectPage() {
  const router = useRouter();

  useEffect(() => {
    async function redirectToSettings() {
      const supabase = createClient();
      const { data } = await supabase.auth.getSession();
      const user = data.session?.user;
      if (!user) {
        router.replace("/login");
        return;
      }

      const metadata = user.user_metadata ?? {};
      const login =
        metadata.user_name ??
        metadata.preferred_username ??
        metadata.name ??
        user.email?.split("@")[0] ??
        "account";

      router.replace(`/${encodeURIComponent(login)}/settings`);
    }

    redirectToSettings();
  }, [router]);

  return (
    <div style={{ minHeight: "100vh", background: "#EBE9E9", color: "#555555", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 14 }}>
      Loading settings
    </div>
  );
}
