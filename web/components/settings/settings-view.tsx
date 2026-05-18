"use client";

import type React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft, ArrowUpRight, Check, GitBranch, Github, Loader2, RefreshCw, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import IndexingProgress from "@/components/onboarding/indexing-progress";
import { apiFetch } from "@/lib/api";
import { createClient } from "@/lib/supabase/client";
import { Repository } from "@/lib/types";

type GitHubUser = {
  login: string;
  name: string;
  avatarURL?: string;
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

      const metadata = session.user.user_metadata ?? {};
      const login =
        metadata.user_name ??
        metadata.preferred_username ??
        metadata.name ??
        session.user.email?.split("@")[0] ??
        account;

      try {
        const { repos: repoList } = await apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token);
        if (!active) return;
        setCurrentUser({
          login,
          name: metadata.full_name ?? metadata.name ?? login,
          avatarURL: metadata.avatar_url ?? metadata.picture,
        });
        setRepos(repoList ?? []);
        setSelectedRepoID((repoList ?? []).find((repo) => repo.is_selected)?.id ?? null);
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
  const backRepo = selectedReadyRepo ?? repos.find((repo) => repo.index_status === "ready" && !repo.unused_at) ?? null;
  const indexingRepo = repos.find((repo) => repo.id === indexingRepoID) ?? null;
  const filteredRepos = repos.filter((repo) => repo.full_name.toLowerCase().includes(search.toLowerCase()));

  async function indexSelectedRepo(repoID = selectedRepoID) {
    if (!repoID) return;
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    const repo = repos.find((candidate) => candidate.id === repoID);
    setIndexingRepoID(repoID);
    setError("");

    try {
      const result = await apiFetch<{ status: string }>(`/api/v1/repos/${repoID}/index`, token, { method: "POST" });
      if (result.status === "already_indexed" && repo) {
        router.push(`/${repo.full_name}`);
      }
    } catch {
      setError("Repository indexing could not be started.");
      setIndexingRepoID(null);
    }
  }

  async function selectRepo(repo: Repository) {
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    if (repo.index_status !== "ready") {
      setSelectedRepoID(repo.id);
      await indexSelectedRepo(repo.id);
      return;
    }

    try {
      await apiFetch<{ status: string }>(`/api/v1/repos/${repo.id}/select`, token, { method: "POST" });
      setRepos((current) => current.map((candidate) => ({
        ...candidate,
        is_selected: candidate.id === repo.id,
        unused_at: candidate.id === repo.id ? undefined : candidate.unused_at,
        purge_after: candidate.id === repo.id ? undefined : candidate.purge_after,
      })));
      setSelectedRepoID(repo.id);
    } catch {
      setError("Repository could not be selected.");
    }
  }

  async function uninstallRepo(repo: Repository) {
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    try {
      await apiFetch<{ status: string }>(`/api/v1/repos/${repo.id}/index`, token, { method: "DELETE" });
      setRepos((current) => current.map((candidate) => candidate.id === repo.id
        ? {
            ...candidate,
            is_selected: false,
            unused_at: new Date().toISOString(),
            purge_after: candidate.purge_after ?? new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
          }
        : candidate));
      if (selectedRepoID === repo.id) setSelectedRepoID(null);
    } catch {
      setError("Repository index could not be uninstalled.");
    }
  }

  if (indexingRepoID && indexingRepo) {
    return <IndexingProgress repoID={indexingRepoID} repoName={indexingRepo.full_name} />;
  }

  if (loading) {
    return (
      <SettingsShell>
        <div style={loadingStyle}>
          <Loader2 size={22} className="animate-spin" />
          <span>Loading settings</span>
        </div>
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
        <Link href={backRepo ? `/${backRepo.full_name}` : "/"} style={backLinkStyle}>
          <ArrowLeft size={16} />
          Back to repo
        </Link>
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
            {selectedReadyRepo && (
              <Link href={`/${selectedReadyRepo.full_name}`} style={secondaryActionStyle}>
                Open selected repo
              </Link>
            )}
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
                const canIndex = (repo.index_status !== "ready" || Boolean(repo.unused_at)) && !repo.revoked_at;
                return (
                  <div
                    key={repo.id}
                    style={repoButtonStyle(selected)}
                  >
                    <GitBranch size={18} color={selected ? "#111111" : "#666666"} />
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <div style={rowTitleStyle}>{repo.full_name}</div>
                      <div style={rowMetaStyle}>{repo.default_branch} · {status}</div>
                    </div>
                    <div style={repoActionsStyle}>
                      {canOpen && (
                        <Link href={`/${repo.full_name}`} style={secondaryActionStyle}>
                          Open
                        </Link>
                      )}
                      {!selected && repo.index_status === "ready" && !repo.unused_at && (
                        <button onClick={() => selectRepo(repo)} style={iconActionStyle} aria-label={`Select ${repo.full_name}`}>
                          <Check size={15} />
                        </button>
                      )}
                      {canIndex && (
                        <button onClick={() => indexSelectedRepo(repo.id)} style={iconActionStyle} aria-label={`Index ${repo.full_name}`}>
                          <RefreshCw size={15} />
                        </button>
                      )}
                      {repo.index_status === "ready" && !repo.unused_at && (
                        <button onClick={() => uninstallRepo(repo)} style={iconActionStyle} aria-label={`Uninstall index for ${repo.full_name}`}>
                          <Trash2 size={15} />
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

const backLinkStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 8,
  color: "#111111",
  textDecoration: "none",
  fontSize: 13,
  fontWeight: 650,
};

const mainStyle: React.CSSProperties = {
  width: "min(860px, 100%)",
  margin: "0 auto",
  padding: "48px 24px",
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
  alignItems: "center",
  gap: 12,
  cursor: "pointer",
  textAlign: "left",
});

const repoActionsStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 6,
  flexWrap: "wrap",
  justifyContent: "flex-end",
};

const iconActionStyle: React.CSSProperties = {
  width: 34,
  height: 34,
  borderRadius: 6,
  border: "1px solid #D4D4D4",
  background: "#FFFFFF",
  color: "#333333",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  cursor: "pointer",
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
  minHeight: "100vh",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 10,
  color: "#555555",
  fontSize: 14,
};
