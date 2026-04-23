"use client";

import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { RepoStatus } from "@/lib/types";

const STATUS_MESSAGES = [
  "Fetching pull requests…",
  "Analysing changed functions…",
  "Building call graphs…",
  "Generating AI summaries…",
];

interface Props {
  repoID: string;
  repoName: string;
}

export default function IndexingProgress({ repoID, repoName }: Props) {
  const router = useRouter();
  const supabase = createClient();
  const [progress, setProgress] = useState(0);
  const [msgIndex, setMsgIndex] = useState(0);
  const [failed, setFailed] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const msgIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    // Animate progress bar: 0 → 70% over 3s, then pulse
    let p = 0;
    const step = () => {
      p += 2;
      if (p <= 70) setProgress(p);
    };
    const progInterval = setInterval(step, 85);

    // Cycle status messages
    msgIntervalRef.current = setInterval(() => {
      setMsgIndex((i) => (i + 1) % STATUS_MESSAGES.length);
    }, 1800);

    // Poll status every 2 seconds
    intervalRef.current = setInterval(async () => {
      const { data: sessionData } = await supabase.auth.getSession();
      const token = sessionData.session?.access_token;
      if (!token) return;

      try {
        const status = await apiFetch<RepoStatus>(`/api/v1/repos/${repoID}/status`, token);
        if (status.index_status === "ready") {
          setProgress(100);
          clearInterval(progInterval);
          if (intervalRef.current) clearInterval(intervalRef.current);
          if (msgIntervalRef.current) clearInterval(msgIntervalRef.current);
          setTimeout(() => router.push(`/repos/${repoID}`), 400);
        } else if (status.index_status === "failed") {
          setFailed(true);
          clearInterval(progInterval);
          if (intervalRef.current) clearInterval(intervalRef.current);
          if (msgIntervalRef.current) clearInterval(msgIntervalRef.current);
        }
      } catch {
        // keep polling
      }
    }, 2000);

    return () => {
      clearInterval(progInterval);
      if (intervalRef.current) clearInterval(intervalRef.current);
      if (msgIntervalRef.current) clearInterval(msgIntervalRef.current);
    };
  }, [repoID]);

  return (
    <div style={{ background: "#EBE9E9", minHeight: "100vh", display: "flex" }}>
      {/* Sidebar */}
      <div style={{ width: 240, background: "#E1E1E1", borderRight: "1px solid #D4D4D4", padding: 20 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <GraphLogo />
          <span style={{ color: "#111111", fontSize: 15, fontWeight: 600 }}>Isoprism</span>
        </div>
      </div>

      <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", padding: 48 }}>
        <div style={{ maxWidth: 480, width: "100%" }}>
          {/* Repo badge */}
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 32 }}>
            <GitHubIcon />
            <span style={{ color: "#666666", fontSize: 14 }}>{repoName}</span>
          </div>

          {failed ? (
            <div style={{ color: "#EF4444", fontSize: 15 }}>
              Indexing failed. Please try again or check the API logs.
            </div>
          ) : (
            <>
              {/* Progress bar */}
              <div style={{ width: "100%", height: 3, background: "#D4D4D4", borderRadius: 2, overflow: "hidden", marginBottom: 16 }}>
                <div
                  style={{
                    height: "100%",
                    background: "#6366F1",
                    width: `${progress}%`,
                    transition: "width 80ms linear",
                    borderRadius: 2,
                  }}
                />
              </div>
              <p style={{ color: "#666666", fontSize: 14 }}>{STATUS_MESSAGES[msgIndex]}</p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function GraphLogo() {
  return (
    <svg width="24" height="24" viewBox="0 0 32 32" fill="none">
      <circle cx="8" cy="16" r="4" fill="#111111" />
      <circle cx="24" cy="8" r="4" fill="#111111" />
      <circle cx="24" cy="24" r="4" fill="#111111" />
      <line x1="12" y1="14" x2="20" y2="10" stroke="#111111" strokeWidth="1.5" />
      <line x1="12" y1="18" x2="20" y2="22" stroke="#111111" strokeWidth="1.5" />
    </svg>
  );
}

function GitHubIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="#888888">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
    </svg>
  );
}
