"use client";

import type React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowUpRight, GitBranch, Github, Loader2, Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { apiFetch } from "@/lib/api";
import { getCachedSettingsRepos, warmSettingsRepos } from "@/lib/settings-cache";
import { createClient } from "@/lib/supabase/client";
import { RepoStatus, Repository } from "@/lib/types";

type GitHubUser = {
  login: string;
  name: string;
  avatarURL?: string;
};

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

export function SettingsView({
  account,
}: {
  account: string;
}) {
  const router = useRouter();
  const supabase = useMemo(() => createClient(), []);

  const [currentUser, setCurrentUser] = useState<GitHubUser | null>(null);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [selectedRepoID, setSelectedRepoID] = useState<string | null>(null);
  const [indexingRepoID, setIndexingRepoID] = useState<string | null>(null);
  const [token, setToken] = useState("");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;

    async function loadSettings() {
      setLoading(true);
      setError("");

      const { data: sessionData } = await supabase.auth.getSession();
      const session = sessionData.session;
      const token = session?.access_token;
      if (!token) {
        router.push("/login");
        return;
      }
      setToken(token);

      const metadata = session.user.user_metadata ?? {};
      const login =
        metadata.user_name ??
        metadata.preferred_username ??
        metadata.name ??
        session.user.email?.split("@")[0] ??
        account;

      try {
        const cachedRepos = getCachedSettingsRepos();
        if (cachedRepos && active) {
          setCurrentUser({
            login,
            name: metadata.full_name ?? metadata.name ?? login,
            avatarURL: metadata.avatar_url ?? metadata.picture,
          });
          setRepos(cachedRepos);
          const selectedRepo = cachedRepos.find((repo) => repo.is_selected) ?? null;
          setSelectedRepoID(selectedRepo?.id ?? null);
          setIndexingRepoID(selectedRepo && (selectedRepo.index_status !== "ready" || selectedRepo.unused_at) ? selectedRepo.id : null);
          setLoading(false);
        }

        const repoList = await warmSettingsRepos(token);
        if (!active) return;
        if (!repoList) {
          if (cachedRepos) return;
          throw new Error("settings repos unavailable");
        }
        setCurrentUser({
          login,
          name: metadata.full_name ?? metadata.name ?? login,
          avatarURL: metadata.avatar_url ?? metadata.picture,
        });
        setRepos(repoList);
        const selectedRepo = repoList.find((repo) => repo.is_selected) ?? null;
        setSelectedRepoID(selectedRepo?.id ?? null);
        setIndexingRepoID(selectedRepo && (selectedRepo.index_status !== "ready" || selectedRepo.unused_at) ? selectedRepo.id : null);
      } catch {
        if (!active) return;
        setError("Settings could not be loaded.");
      } finally {
        if (active) setLoading(false);
      }
    }

    loadSettings();

    return () => {
      active = false;
    };
  }, [account, router, supabase]);

  const manageURL = "https://github.com/settings/installations";

  const selectedReadyRepo = repos.find((repo) => repo.is_selected && repo.index_status === "ready") ?? null;
  const accountType = repos.some((repo) => repo.user_class === "pilot") ? "Pilot" : "Premium";
  const indexingRepo = repos.find((repo) => repo.id === indexingRepoID) ?? null;
  const filteredRepos = repos.filter((repo) => repo.full_name.toLowerCase().includes(search.toLowerCase()));

  async function selectRepo(repo: Repository) {
    const { data: sessionData } = await supabase.auth.getSession();
    const nextToken = sessionData.session?.access_token;
    if (!nextToken) return;

    const previousSelectedRepoID = selectedRepoID;
    setToken(nextToken);
    setError("");
    setSelectedRepoID(repo.id);
    setRepos((current) => current.map((candidate) => ({
      ...candidate,
      is_selected: candidate.id === repo.id,
      unused_at: candidate.id === repo.id ? undefined : candidate.unused_at,
      purge_after: candidate.id === repo.id ? undefined : candidate.purge_after,
    })));

    try {
      const result = await apiFetch<{ status: string }>(`/api/v1/repos/${repo.id}/index`, nextToken, { method: "POST" });
      if (result.status === "already_indexed") {
        setRepos((current) => current.map((candidate) => candidate.id === repo.id
          ? { ...candidate, index_status: "ready", is_selected: true, unused_at: undefined, purge_after: undefined }
          : { ...candidate, is_selected: false }));
        setIndexingRepoID(null);
        return;
      }
      setRepos((current) => current.map((candidate) => candidate.id === repo.id
        ? { ...candidate, index_status: candidate.index_status === "ready" ? "ready" : "pending", is_selected: true, unused_at: undefined, purge_after: undefined }
        : { ...candidate, is_selected: false }));
      setIndexingRepoID(repo.id);
    } catch {
      setError("Repository could not be selected.");
      setIndexingRepoID(null);
      setSelectedRepoID(previousSelectedRepoID);
      setRepos((current) => current.map((candidate) => ({
        ...candidate,
        is_selected: candidate.id === previousSelectedRepoID,
      })));
    }
  }

  function markRepoReady(repoID: string) {
    setRepos((current) => current.map((candidate) => candidate.id === repoID
      ? { ...candidate, index_status: "ready", is_selected: true, unused_at: undefined, purge_after: undefined }
      : { ...candidate, is_selected: false }));
    setSelectedRepoID(repoID);
    setIndexingRepoID(null);
  }

  function markRepoFailed(repoID: string) {
    setRepos((current) => current.map((candidate) => candidate.id === repoID
      ? { ...candidate, index_status: "failed" }
        : candidate));
    setIndexingRepoID(null);
  }

  if (loading) {
    return (
      <SettingsShell>
        <aside style={sidebarStyle}>
          <div style={accountBlockStyle}>
            <div aria-hidden="true" style={{ ...avatarStyle, background: "#111111" }}>
              {(currentUser?.name ?? account).slice(0, 1).toUpperCase()}
            </div>
            <div style={{ minWidth: 0 }}>
              <div style={accountNameStyle}>{currentUser?.name ?? "Settings"}</div>
              <div style={accountLoginStyle}>{currentUser?.login ?? "Loading account"}</div>
            </div>
          </div>
        </aside>
        <main style={{ ...mainStyle, ...loadingMainStyle }}>
          <div style={loadingStyle}>
            <Loader2 size={22} className="animate-spin" />
            <span>Loading settings</span>
          </div>
        </main>
      </SettingsShell>
    );
  }

  return (
    <SettingsShell>
      <aside style={sidebarStyle}>
        <div style={accountBlockStyle}>
          <div
            aria-hidden="true"
            style={{
              ...avatarStyle,
              background: currentUser?.avatarURL ? `url(${currentUser.avatarURL}) center / cover` : "#111111",
            }}
          >
            {!currentUser?.avatarURL ? (currentUser?.name ?? account).slice(0, 1).toUpperCase() : null}
          </div>
          <div style={{ minWidth: 0 }}>
            <div style={accountNameStyle}>{currentUser?.name ?? account}</div>
            <div style={accountLoginStyle}>{currentUser?.login ?? account}</div>
          </div>
        </div>
        <div style={accountTypeStyle}>{accountType}</div>
      </aside>

      <main style={mainStyle}>
        <header style={headerStyle}>
          <div>
            <h1 style={titleStyle}>Manage Repositories</h1>
            <p style={copyStyle}>
              Manage GitHub access, indexing, and the repository selected for review.
            </p>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            {selectedReadyRepo && (
              <Link href={`/${selectedReadyRepo.full_name}`} style={secondaryActionStyle}>
                Exit settings
              </Link>
            )}
          </div>
        </header>

        {error && <Notice tone="error">{error}</Notice>}

        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <div>
              <h2 style={sectionTitleStyle}>GitHub connection</h2>
              <p style={copyStyle}>
                Signed in as {currentUser?.name ?? account}. Repository permissions are managed through the GitHub App.
              </p>
            </div>
            <Github size={24} color="#111111" />
          </div>

          <div style={actionRowStyle}>
            <a href={manageURL} style={primaryActionStyle}>
              <ArrowUpRight size={15} />
              Manage GitHub App
            </a>
          </div>
        </section>

        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <div>
              <h2 style={sectionTitleStyle}>Repositories</h2>
              <p style={copyStyle}>Authorized repositories appear here. Added repositories are visible before they are indexed.</p>
            </div>
          </div>

          <label style={searchBoxStyle}>
            <Search size={15} color="#888888" />
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search repositories"
              style={searchInputStyle}
            />
          </label>

          <div style={listStyle}>
            {filteredRepos.length === 0 ? (
              <div style={emptyStyle}>No repositories match. Update GitHub App access if a repository is missing.</div>
            ) : (
              filteredRepos.map((repo) => {
                const selected = Boolean(repo.is_selected);
                const status = repoStatusLabel(repo);
                const canOpen = selected && repo.index_status === "ready" && !repo.unused_at;
                const isIndexing = indexingRepo?.id === repo.id;
                return (
                  <div
                    key={repo.id}
                    style={repoButtonStyle(selected)}
                  >
                    <GitBranch size={18} color={selected ? "#111111" : "#666666"} style={{ marginTop: 2 }} />
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <div style={rowTitleStyle}>{repo.full_name}</div>
                      <div style={rowMetaStyle}>{repo.default_branch} · {status}</div>
                      {isIndexing && (
                        <InlineIndexingProgress
                          repoID={repo.id}
                          token={token}
                          onReady={() => markRepoReady(repo.id)}
                          onFailed={() => markRepoFailed(repo.id)}
                        />
                      )}
                    </div>
                    <div style={repoActionsStyle}>
                      {canOpen && (
                        <Link href={`/${repo.full_name}`} style={secondaryActionStyle}>
                          Open
                        </Link>
                      )}
                      {!canOpen && !isIndexing && (
                        <button onClick={() => selectRepo(repo)} style={selectButtonStyle} aria-label={`Select ${repo.full_name}`}>
                          Select
                        </button>
                      )}
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </section>
      </main>
    </SettingsShell>
  );
}

function SettingsShell({ children }: { children: React.ReactNode }) {
  return (
    <div style={shellStyle}>
      {children}
    </div>
  );
}

function Notice({ children, tone }: { children: React.ReactNode; tone: "error" | "neutral" }) {
  return (
    <div style={{
      border: `1px solid ${tone === "error" ? "#F3B4B4" : "#D6D6D6"}`,
      background: tone === "error" ? "#FFF1F1" : "#F6F6F6",
      borderRadius: 8,
      padding: "10px 12px",
      color: tone === "error" ? "#991B1B" : "#555555",
      fontSize: 13,
      lineHeight: 1.45,
      marginBottom: 14,
    }}>
      {children}
    </div>
  );
}

function repoStatusLabel(repo: Repository) {
  if (repo.index_status === "ready" && !repo.unused_at) return "Indexed";
  return "Not indexed";
}

function InlineIndexingProgress({
  repoID,
  token,
  onReady,
  onFailed,
}: {
  repoID: string;
  token: string;
  onReady: () => void;
  onFailed: () => void;
}) {
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState<IndexingStatus | null>(null);
  const [failed, setFailed] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!token) return;

    let active = true;
    let p = 0;
    const progInterval = setInterval(() => {
      p += 1;
      if (active) setProgress((current) => Math.max(current, Math.min(p, 20)));
    }, 240);

    intervalRef.current = setInterval(async () => {
      try {
        const nextStatus = await apiFetch<IndexingStatus>(`/api/v1/repos/${repoID}/status`, token);
        if (!active) return;
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
          window.setTimeout(onReady, 400);
        } else if (nextStatus.index_status === "failed") {
          setFailed(true);
          clearInterval(progInterval);
          if (intervalRef.current) clearInterval(intervalRef.current);
          onFailed();
        }
      } catch {
        // Keep polling. The indexer may have just started and status can lag briefly.
      }
    }, 2000);

    return () => {
      active = false;
      clearInterval(progInterval);
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [onFailed, onReady, repoID, token]);

  const message = status?.index_message || status?.index_job?.message || "Preparing repository index";
  const counter = stageCounter(status);
  const eta = formatETA(status?.eta_seconds ?? status?.index_job?.eta_seconds);

  if (failed) {
    return <div style={inlineErrorStyle}>Indexing failed. Try selecting this repository again.</div>;
  }

  return (
    <div style={inlineProgressStyle}>
      <div style={progressTrackStyle}>
        <div
          style={{
            ...progressFillStyle,
            width: `${progress}%`,
          }}
        />
      </div>
      <div style={progressTextRowStyle}>
        <p style={progressMessageStyle}>{message}</p>
        <span style={progressPercentStyle}>{Math.round(progress)}%</span>
      </div>
      <div style={progressMetaRowStyle}>
        <p style={progressMetaStyle}>{counter}</p>
        <p style={progressMetaStyle}>{eta}</p>
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

const shellStyle: React.CSSProperties = {
  minHeight: "100vh",
  background: "#EBE9E9",
  color: "#111111",
  display: "flex",
};

const sidebarStyle: React.CSSProperties = {
  width: 280,
  flex: "0 0 280px",
  minHeight: "100vh",
  padding: "44px 28px",
  background: "#DCDCDC",
  display: "flex",
  flexDirection: "column",
  gap: 24,
};

const accountBlockStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 12,
};

const avatarStyle: React.CSSProperties = {
  width: 44,
  height: 44,
  borderRadius: "50%",
  flex: "0 0 44px",
  color: "#FFFFFF",
  fontSize: 17,
  fontWeight: 700,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
};

const accountNameStyle: React.CSSProperties = {
  color: "#111111",
  fontSize: 15,
  fontWeight: 700,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const accountLoginStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 13,
  marginTop: 2,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const accountTypeStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  width: "fit-content",
  minHeight: 26,
  padding: "0 9px",
  borderRadius: 999,
  border: "1px solid #C7C7C7",
  background: "#E9E9E9",
  color: "#555555",
  fontSize: 13,
  fontWeight: 650,
};

const mainStyle: React.CSSProperties = {
  width: "min(860px, 100%)",
  margin: "0 auto",
  padding: "48px 24px",
};

const loadingMainStyle: React.CSSProperties = {
  minHeight: "100vh",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
};

const headerStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  gap: 24,
  marginBottom: 24,
};

const titleStyle: React.CSSProperties = {
  margin: 0,
  color: "#111111",
  fontSize: 28,
  lineHeight: 1.18,
  fontWeight: 700,
};

const copyStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 13,
  lineHeight: 1.5,
  margin: "6px 0 0",
};

const sectionStyle: React.CSSProperties = {
  border: "1px solid #D4D4D4",
  borderRadius: 8,
  background: "#FFFFFF",
  padding: 20,
  marginBottom: 16,
};

const sectionHeaderStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  gap: 20,
  marginBottom: 16,
};

const sectionTitleStyle: React.CSSProperties = {
  margin: 0,
  color: "#111111",
  fontSize: 17,
  fontWeight: 700,
};

const actionRowStyle: React.CSSProperties = {
  display: "flex",
  gap: 8,
  flexWrap: "wrap",
};

const primaryActionStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 7,
  minHeight: 38,
  padding: "0 13px",
  borderRadius: 6,
  border: "none",
  background: "#111111",
  color: "#FFFFFF",
  textDecoration: "none",
  fontSize: 13,
  fontWeight: 650,
};

const secondaryActionStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 6,
  minHeight: 34,
  padding: "0 10px",
  borderRadius: 6,
  border: "1px solid #D4D4D4",
  background: "#FFFFFF",
  color: "#333333",
  textDecoration: "none",
  fontSize: 12,
  fontWeight: 650,
  whiteSpace: "nowrap",
};

const searchBoxStyle: React.CSSProperties = {
  height: 40,
  display: "flex",
  alignItems: "center",
  gap: 8,
  border: "1px solid #D4D4D4",
  borderRadius: 6,
  background: "#FFFFFF",
  padding: "0 11px",
  marginBottom: 14,
};

const searchInputStyle: React.CSSProperties = {
  width: "100%",
  border: "none",
  outline: "none",
  color: "#111111",
  fontSize: 13,
  background: "transparent",
};

const listStyle: React.CSSProperties = {
  display: "grid",
  gap: 8,
};

const repoButtonStyle = (selected: boolean): React.CSSProperties => ({
  width: "100%",
  minHeight: 58,
  border: `1px solid ${selected ? "#111111" : "#E0E0E0"}`,
  borderRadius: 8,
  background: selected ? "#F1F1F1" : "#FAFAFA",
  padding: "10px 12px",
  display: "flex",
  alignItems: "flex-start",
  gap: 12,
  textAlign: "left",
});

const repoActionsStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 6,
  flexWrap: "wrap",
  justifyContent: "flex-end",
};

const selectButtonStyle: React.CSSProperties = {
  minHeight: 34,
  padding: "0 11px",
  borderRadius: 6,
  border: "1px solid #D4D4D4",
  background: "#FFFFFF",
  color: "#333333",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  cursor: "pointer",
  fontSize: 12,
  fontWeight: 650,
};

const rowTitleStyle: React.CSSProperties = {
  color: "#111111",
  fontSize: 14,
  fontWeight: 650,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const rowMetaStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  lineHeight: 1.45,
  marginTop: 2,
};

const inlineProgressStyle: React.CSSProperties = {
  marginTop: 12,
  maxWidth: 460,
};

const progressTrackStyle: React.CSSProperties = {
  width: "100%",
  height: 3,
  background: "#D4D4D4",
  borderRadius: 2,
  overflow: "hidden",
  marginBottom: 10,
};

const progressFillStyle: React.CSSProperties = {
  height: "100%",
  background: "#6366F1",
  transition: "width 80ms linear",
  borderRadius: 2,
};

const progressTextRowStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "baseline",
  justifyContent: "space-between",
  gap: 16,
};

const progressMessageStyle: React.CSSProperties = {
  color: "#111111",
  fontSize: 13,
  fontWeight: 500,
  margin: 0,
};

const progressPercentStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 12,
};

const progressMetaRowStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 16,
  marginTop: 6,
};

const progressMetaStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 12,
  margin: 0,
};

const inlineErrorStyle: React.CSSProperties = {
  color: "#EF4444",
  fontSize: 13,
  marginTop: 10,
};

const emptyStyle: React.CSSProperties = {
  border: "1px dashed #D4D4D4",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: "24px 16px",
  color: "#777777",
  fontSize: 13,
  textAlign: "center",
};

const loadingStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 10,
  color: "#555555",
  fontSize: 14,
};
