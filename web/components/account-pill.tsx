"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { createClient } from "@/lib/supabase/client";

type AccountUser = {
  name: string;
  login: string;
  avatarURL?: string;
};

const hiddenPrefixes = ["/login", "/auth/callback"];

export default function AccountPill() {
  const pathname = usePathname();
  const supabase = useMemo(() => createClient(), []);
  const [user, setUser] = useState<AccountUser | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;

    async function loadUser() {
      const { data } = await supabase.auth.getUser();
      if (!active) return;

      const authUser = data.user;
      if (!authUser) {
        setUser(null);
        setLoading(false);
        return;
      }

      const metadata = authUser.user_metadata ?? {};
      const login =
        metadata.user_name ??
        metadata.preferred_username ??
        metadata.name ??
        authUser.email?.split("@")[0] ??
        "settings";

      setUser({
        login,
        name: metadata.full_name ?? metadata.name ?? login,
        avatarURL: metadata.avatar_url ?? metadata.picture,
      });
      setLoading(false);
    }

    loadUser();

    const { data: listener } = supabase.auth.onAuthStateChange(() => {
      loadUser();
    });

    return () => {
      active = false;
      listener.subscription.unsubscribe();
    };
  }, [supabase]);

  if (hiddenPrefixes.some((prefix) => pathname?.startsWith(prefix))) {
    return null;
  }

  if (loading) {
    return (
      <div
        aria-hidden="true"
        style={{
          position: "fixed",
          top: 18,
          right: 24,
          zIndex: 60,
          width: 144,
          height: 38,
          borderRadius: 999,
          background: "#DCDCDC",
          border: "1px solid #CFCFCF",
        }}
      />
    );
  }

  if (!user) return null;

  return (
    <Link
      href={`/${encodeURIComponent(user.login)}/settings`}
      aria-label="Open settings"
      style={{
        position: "fixed",
        top: 18,
        right: 24,
        zIndex: 60,
        display: "flex",
        alignItems: "center",
        gap: 8,
        height: 38,
        maxWidth: "min(260px, calc(100vw - 48px))",
        padding: "4px 10px 4px 5px",
        border: "1px solid #E4E4E4",
        borderRadius: 999,
        background: "rgba(255,255,255,0.92)",
        color: "#111111",
        textDecoration: "none",
        backdropFilter: "blur(10px)",
      }}
    >
      <span
        aria-hidden="true"
        style={{
          width: 28,
          height: 28,
          flex: "0 0 auto",
          borderRadius: "50%",
          background: user.avatarURL ? `url(${user.avatarURL}) center / cover` : "#111111",
          color: "#FFFFFF",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          fontSize: 12,
          fontWeight: 700,
        }}
      >
        {!user.avatarURL ? user.name.slice(0, 1).toUpperCase() : null}
      </span>
      <span
        style={{
          minWidth: 0,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
          fontSize: 13,
          fontWeight: 600,
        }}
      >
        {user.name}
      </span>
    </Link>
  );
}
