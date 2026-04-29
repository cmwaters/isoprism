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
        scopes: "read:user user:email read:org",
      },
    });
    if (error) {
      console.error("Sign in error:", error.message);
      setLoading(false);
    }
  }

  return (
    <div
      style={{ background: "#EBE9E9" }}
      className="min-h-screen flex items-center justify-center"
    >
      <main style={{ maxWidth: 640, width: "100%", padding: "48px 24px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 32 }}>
          <span style={{ color: "#111111", fontSize: 20, fontWeight: 600 }}>Isoprism</span>
        </div>

        <section style={{ display: "grid", gap: 20, marginBottom: 36 }}>
          <p style={{ color: "#333333", fontSize: 18, lineHeight: 1.65, margin: 0 }}>
            This is a prototype, not a fully fledged product. It serves to answer one simple
            question: is there a better way of understanding and reviewing code changes?
          </p>

          <p style={{ color: "#555555", fontSize: 15, lineHeight: 1.7, margin: 0 }}>
            This is for beta testers. The expectation is to use this prototype where possible
            for reviewing PRs. You will connect this to your GitHub and select a single
            repository.
          </p>

          <p style={{ color: "#555555", fontSize: 15, lineHeight: 1.7, margin: 0 }}>
            There will be a footer for submitting feature requests and bug reports. You are
            expected to trial this for a week and fill out a short questionnaire at the end.
          </p>
        </section>

        <button
          onClick={signInWithGitHub}
          disabled={loading}
          style={{
            width: "100%",
            height: 48,
            background: loading ? "#CCCCCC" : "#111111",
            color: "#FFFFFF",
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
          onMouseEnter={(e) => { if (!loading) (e.currentTarget as HTMLButtonElement).style.background = "#333333"; }}
          onMouseLeave={(e) => { if (!loading) (e.currentTarget as HTMLButtonElement).style.background = "#111111"; }}
        >
          {loading ? (
            <span style={{ opacity: 0.7 }}>Connecting…</span>
          ) : (
            <>
              <GitHubIcon />
              Connect GitHub
            </>
          )}
        </button>

        <p style={{ color: "#888888", fontSize: 12, textAlign: "center", marginTop: 48 }}>
          By signing in you authorise read access to your repositories.
        </p>
      </main>
    </div>
  );
}

function GitHubIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
    </svg>
  );
}
