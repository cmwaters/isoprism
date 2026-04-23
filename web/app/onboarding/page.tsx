"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";

export default function OnboardingPage() {
  const router = useRouter();
  const supabase = createClient();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleConnectGitHub() {
    setLoading(true);
    setError("");

    const { data: session } = await supabase.auth.getSession();
    const userID = session.session?.user.id;
    if (!userID) {
      router.push("/login");
      return;
    }

    const appName = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;
    if (!appName) {
      setError("GitHub App not configured.");
      setLoading(false);
      return;
    }

    window.location.href = `https://github.com/apps/${appName}/installations/new?state=${userID}`;
  }

  return (
    <div style={{ minHeight: "100vh", background: "#EBE9E9", display: "flex", alignItems: "center", justifyContent: "center" }}>
      <div style={{ width: "100%", maxWidth: 360, padding: "0 24px" }}>
        <div style={{ marginBottom: 48, display: "flex", alignItems: "center", gap: 10 }}>
          <GraphLogo />
          <span style={{ color: "#111111", fontSize: 18, fontWeight: 600, letterSpacing: "-0.01em" }}>Isoprism</span>
        </div>

        <h1 style={{ color: "#111111", fontSize: 24, fontWeight: 600, marginBottom: 8 }}>Connect GitHub</h1>
        <p style={{ color: "#666666", fontSize: 14, marginBottom: 32, lineHeight: 1.6 }}>
          Install the Isoprism GitHub App to start indexing your repository.
        </p>

        {error && <p style={{ color: "#EF4444", fontSize: 13, marginBottom: 16 }}>{error}</p>}

        <button
          onClick={handleConnectGitHub}
          disabled={loading}
          style={{
            width: "100%",
            height: 44,
            background: "#111111",
            border: "none",
            borderRadius: 8,
            color: "#FFFFFF",
            fontSize: 14,
            fontWeight: 600,
            cursor: loading ? "not-allowed" : "pointer",
            opacity: loading ? 0.6 : 1,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            gap: 8,
          }}
        >
          {loading ? "Redirecting…" : (
            <>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
              </svg>
              Install GitHub App
            </>
          )}
        </button>
      </div>
    </div>
  );
}

function GraphLogo() {
  return (
    <svg width="28" height="28" viewBox="0 0 32 32" fill="none">
      <circle cx="8" cy="16" r="4" fill="#111111" />
      <circle cx="24" cy="8" r="4" fill="#111111" />
      <circle cx="24" cy="24" r="4" fill="#111111" />
      <line x1="12" y1="14" x2="20" y2="10" stroke="#111111" strokeWidth="1.5" />
      <line x1="12" y1="18" x2="20" y2="22" stroke="#111111" strokeWidth="1.5" />
    </svg>
  );
}
