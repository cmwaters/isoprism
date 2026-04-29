"use client";

import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { RepoStatus } from "@/lib/types";

interface Props {
  repoID: string;
  repoName: string;
}

type IndexingStatus = RepoStatus & {
  index_phase?: string;
  index_message?: string;
  index_percent?: number;
  eta_seconds?: number;
  index_job?: {
    commit_sha: string;
    status: "pending" | "running" | "ready" | "failed";
    phase: string;
    message: string;
    percent: number;
    files_total: number;
    files_done: number;
    nodes_total: number;
    nodes_done: number;
    edges_total: number;
    edges_done: number;
    eta_seconds?: number;
    updated_at: string;
    error?: string;
  };
};

export default function IndexingProgress({ repoID, repoName }: Props) {
  const router = useRouter();
  const supabase = createClient();
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState<IndexingStatus | null>(null);
  const [failed, setFailed] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    // Gentle fallback movement until the API reports concrete progress.
    let p = 0;
    const step = () => {
      p += 1;
      setProgress((current) => Math.max(current, Math.min(p, 20)));
    };
    const progInterval = setInterval(step, 240);

    // Poll status every 2 seconds
    intervalRef.current = setInterval(async () => {
      const { data: sessionData } = await supabase.auth.getSession();
      const token = sessionData.session?.access_token;
      if (!token) return;

      try {
        const nextStatus = await apiFetch<IndexingStatus>(`/api/v1/repos/${repoID}/status`, token);
        setStatus(nextStatus);
        if (typeof nextStatus.index_percent === "number") {
          setProgress(nextStatus.index_percent);
        } else if (nextStatus.index_job?.percent) {
          setProgress(nextStatus.index_job.percent);
        }

        if (nextStatus.index_status === "ready") {
          setProgress(100);
          clearInterval(progInterval);
          if (intervalRef.current) clearInterval(intervalRef.current);
          setTimeout(() => router.push(`/${repoName}`), 400);
        } else if (nextStatus.index_status === "failed") {
          setFailed(true);
          clearInterval(progInterval);
          if (intervalRef.current) clearInterval(intervalRef.current);
        }
      } catch {
        // keep polling
      }
    }, 2000);

    return () => {
      clearInterval(progInterval);
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [repoID, repoName, router]);

  const message = status?.index_message || status?.index_job?.message || "Preparing repository index";
  const counter = stageCounter(status);
  const eta = formatETA(status?.eta_seconds ?? status?.index_job?.eta_seconds);

  return (
    <div style={{ background: "#EBE9E9", minHeight: "100vh", display: "flex" }}>
      {/* Sidebar */}
      <div style={{ width: 240, background: "#E1E1E1", borderRight: "1px solid #D4D4D4", padding: 20 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
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
              <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 16 }}>
                <p style={{ color: "#111111", fontSize: 15, fontWeight: 500, margin: 0 }}>{message}</p>
                <span style={{ color: "#666666", fontSize: 13 }}>{Math.round(progress)}%</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16, marginTop: 8 }}>
                <p style={{ color: "#666666", fontSize: 13, margin: 0 }}>{counter}</p>
                <p style={{ color: "#666666", fontSize: 13, margin: 0 }}>{eta}</p>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function stageCounter(status: IndexingStatus | null) {
  const job = status?.index_job;
  if (!job) return "Starting";

  if (job.phase === "fetching_files" && job.files_total > 0) {
    return `${job.files_done.toLocaleString()} / ${job.files_total.toLocaleString()} files`;
  }
  if (job.phase === "writing_nodes" && job.nodes_total > 0) {
    return `${job.nodes_done.toLocaleString()} / ${job.nodes_total.toLocaleString()} nodes`;
  }
  if (job.phase === "building_edges" && job.edges_total > 0) {
    return `${job.edges_done.toLocaleString()} / ${job.edges_total.toLocaleString()} files analysed`;
  }
  if (job.phase === "extracting_tests") {
    return "Linking tests";
  }
  return "Resolving default branch";
}

function formatETA(seconds?: number) {
  if (!seconds || seconds < 20) return "Estimating time remaining";
  const minutes = Math.max(1, Math.round(seconds / 60));
  if (minutes === 1) return "About 1 minute remaining";
  return `About ${minutes} minutes remaining`;
}

function GitHubIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="#888888">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
    </svg>
  );
}
