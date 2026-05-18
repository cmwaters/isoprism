import { redirect } from "next/navigation";
import Link from "next/link";
import type React from "react";
import { createClient } from "@/lib/supabase/server";
import { API_URL } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function RootPage() {
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();

  if (!user) return <WorkInProgressPage />;

  let redirectPath = "/";

  // Direct site visits are pilot-first. The auth callback is the place where
  // invited pilot users with no connected repos are sent to GitHub App install.
  try {
    const res = await fetch(`${API_URL}/api/v1/auth/status?user_id=${user.id}`, {
      cache: "no-store",
    });

    if (res.ok) {
      const { redirect } = await res.json();
      if (typeof redirect === "string" && redirect.length > 0) {
        redirectPath = redirect === "/onboarding" ? "/" : redirect;
      }
    }
  } catch {
    // Fall through to the work-in-progress screen below.
  }

  if (redirectPath === "/") return <WorkInProgressPage />;

  redirect(redirectPath);
}

function WorkInProgressPage() {
  return (
    <main style={pageStyle}>
      <section style={contentStyle}>
        <div style={brandStyle}>Isoprism</div>
        <p style={eyebrowStyle}>Work in progress</p>
        <h1 style={titleStyle}>We&apos;re working on the tools for the future</h1>
        <p style={copyStyle}>
          We are still shaping the prototype with a small group of engineers before opening it
          more broadly. If you are interested in trying the pilot, answer a few questions and
          register your interest and we will follow up with next steps.
        </p>
        <Link href="/pilot/register" style={buttonStyle}>
          Register for the pilot
        </Link>
      </section>
    </main>
  );
}

const pageStyle: React.CSSProperties = {
  minHeight: "100vh",
  background: "#EBE9E9",
  color: "#111111",
  display: "grid",
  placeItems: "center",
  padding: "48px 24px",
};

const contentStyle: React.CSSProperties = {
  width: "min(620px, 100%)",
  display: "grid",
  gap: 18,
};

const brandStyle: React.CSSProperties = {
  fontSize: 20,
  fontWeight: 650,
};

const eyebrowStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 12,
  fontWeight: 750,
  letterSpacing: 0,
  margin: "16px 0 0",
  textTransform: "uppercase",
};

const titleStyle: React.CSSProperties = {
  fontSize: 36,
  lineHeight: 1.12,
  fontWeight: 760,
  margin: 0,
  maxWidth: 560,
};

const copyStyle: React.CSSProperties = {
  color: "#444444",
  fontSize: 16,
  lineHeight: 1.65,
  margin: 0,
  maxWidth: 560,
};

const buttonStyle: React.CSSProperties = {
  width: "fit-content",
  minHeight: 44,
  borderRadius: 7,
  background: "#111111",
  color: "#FFFFFF",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  padding: "0 16px",
  fontSize: 14,
  fontWeight: 700,
  textDecoration: "none",
};
