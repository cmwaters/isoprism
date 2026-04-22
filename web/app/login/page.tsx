"use client";

import { createClient } from "@/lib/supabase/client";
import { useState } from "react";

export default function LoginPage() {
  const supabase = createClient();
  const [loading, setLoading] = useState(false);

  async function signInWithGitHub() {
    setLoading(true);
    const { error } = await supabase.auth.signInWithOAuth({
      provider: "github",
      options: {
        redirectTo: `${window.location.origin}/auth/callback`,
        scopes: "read:user user:email",
      },
    });
    if (error) {
      console.error("Sign in error:", error.message);
      setLoading(false);
    }
  }

  return (
    <div
      style={{ background: "#0A0A0A" }}
      className="min-h-screen flex items-center justify-center"
    >
      <div style={{ maxWidth: 360, width: "100%", padding: "0 24px" }}>
        {/* Logo + wordmark */}
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 48 }}>
          <GraphLogo />
          <span style={{ color: "#F0F0F0", fontSize: 20, fontWeight: 600 }}>Isoprism</span>
        </div>

        <h1 style={{ color: "#F0F0F0", fontSize: 28, fontWeight: 600, marginBottom: 12, lineHeight: 1.25 }}>
          Understand what your PRs actually change.
        </h1>
        <p style={{ color: "#888888", fontSize: 15, marginBottom: 40, lineHeight: 1.6 }}>
          A graph view of every function affected. Plain-language summaries. No diffs.
        </p>

        <button
          onClick={signInWithGitHub}
          disabled={loading}
          style={{
            width: "100%",
            height: 48,
            background: loading ? "#CCCCCC" : "#F0F0F0",
            color: "#0A0A0A",
            border: "none",
            borderRadius: 8,
            fontSize: 15,
            fontWeight: 500,
            cursor: loading ? "not-allowed" : "pointer",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            gap: 10,
            transition: "background 150ms ease-out",
          }}
          onMouseEnter={(e) => { if (!loading) (e.currentTarget as HTMLButtonElement).style.background = "#FFFFFF"; }}
          onMouseLeave={(e) => { if (!loading) (e.currentTarget as HTMLButtonElement).style.background = "#F0F0F0"; }}
        >
          {loading ? (
            <span style={{ opacity: 0.7 }}>Connecting…</span>
          ) : (
            <>
              <GitHubIcon />
              Continue with GitHub
            </>
          )}
        </button>

        <p style={{ color: "#555555", fontSize: 12, textAlign: "center", marginTop: 48 }}>
          By signing in you authorise read access to your repositories.
        </p>
      </div>
    </div>
  );
}

function GraphLogo() {
  return (
    <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
      <circle cx="8" cy="16" r="4" fill="white" />
      <circle cx="24" cy="8" r="4" fill="white" />
      <circle cx="24" cy="24" r="4" fill="white" />
      <line x1="12" y1="14" x2="20" y2="10" stroke="white" strokeWidth="1.5" />
      <line x1="12" y1="18" x2="20" y2="22" stroke="white" strokeWidth="1.5" />
    </svg>
  );
}

function GitHubIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
    </svg>
  );
}
