"use client";

import type React from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowUpRight, GitBranch, Github, Loader2, RefreshCw, Search, X } from "lucide-react";
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

export default function SettingsPage() {
  const params = useParams<{ owner: string }>();
  const account = decodeURIComponent(params.owner);

  return <SettingsView account={account} />;
}

export function SettingsView({
  account,
  embedded = false,
  onClose,
}: {
  account: string;
  embedded?: boolean;
  onClose?: () => void;
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
        setSelectedRepoID((repoList ?? []).find((repo) => repo.index_status === "ready")?.id ?? null);
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

  const readyRepo = repos.find((repo) => repo.index_status === "ready") ?? null;
  const indexingRepo = repos.find((repo) => repo.id === indexingRepoID) ?? null;
  const selectedRepo = repos.find((repo) => repo.id === selectedRepoID) ?? null;
  const filteredRepos = repos.filter((repo) => repo.full_name.toLowerCase().includes(search.toLowerCase()));

  async function indexSelectedRepo() {
    if (!selectedRepoID) return;
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    const repo = repos.find((candidate) => candidate.id === selectedRepoID);
    setIndexingRepoID(selectedRepoID);
    setError("");

    try {
      const result = await apiFetch<{ status: string }>(`/api/v1/repos/${selectedRepoID}/index`, token, { method: "POST" });
      if (result.status === "already_indexed" && repo) {
        router.push(`/${repo.full_name}`);
      }
    } catch {
      setError("Repository indexing could not be started.");
      setIndexingRepoID(null);
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
    <SettingsShell embedded={embedded}>
      <main style={embedded ? embeddedMainStyle : mainStyle}>
        {embedded && onClose && (
          <button aria-label="Close settings" onClick={onClose} style={closeButtonStyle}>
            <X size={18} />
          </button>
        )}

        {!embedded && (
          <Link href="/" style={brandStyle}>
            <span>Isoprism</span>
          </Link>
        )}

        <header style={headerStyle}>
          <div>
            <div style={eyebrowStyle}>Settings</div>
            <h1 style={titleStyle}>GitHub and repository</h1>
            <p style={copyStyle}>
              Manage your GitHub connection and choose the single repository Isoprism should index for the beta.
            </p>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            {readyRepo && !embedded && (
              <Link href={`/${readyRepo.full_name}`} style={secondaryActionStyle}>
                Exit settings
              </Link>
            )}
            {currentUser?.avatarURL && (
              <div
                aria-hidden="true"
                style={{
                  ...avatarStyle,
                  background: `url(${currentUser.avatarURL}) center / cover`,
                }}
              />
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
              <h2 style={sectionTitleStyle}>Current repository</h2>
              <p style={copyStyle}>
                Choose a different repository to swap the beta workspace. The new repository will be indexed before it opens.
              </p>
            </div>
            {readyRepo && (
              <Link href={`/${readyRepo.full_name}`} style={secondaryActionStyle}>
                Open current repo
              </Link>
            )}
          </div>

          {readyRepo ? (
            <div style={currentRepoStyle}>
              <GitBranch size={18} color="#555555" />
              <div style={{ minWidth: 0, flex: 1 }}>
                <div style={rowTitleStyle}>{readyRepo.full_name}</div>
                <div style={rowMetaStyle}>{readyRepo.default_branch} · indexed</div>
              </div>
            </div>
          ) : (
            <Notice tone="neutral">No repository is indexed yet. Select one below to start.</Notice>
          )}
        </section>

        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <div>
              <h2 style={sectionTitleStyle}>Swap repository</h2>
              <p style={copyStyle}>Select one repository, then index it. Isoprism will show indexing progress before opening it.</p>
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
                const selected = repo.id === selectedRepoID;
                return (
                  <button
                    key={repo.id}
                    style={repoButtonStyle(selected)}
                    onClick={() => setSelectedRepoID(repo.id)}
                  >
                    <GitBranch size={18} color={selected ? "#111111" : "#666666"} />
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <div style={rowTitleStyle}>{repo.full_name}</div>
                      <div style={rowMetaStyle}>{repo.default_branch} · {repo.index_status}</div>
                    </div>
                    <StatusChip>{repo.index_status === "ready" ? "Indexed" : repo.index_status}</StatusChip>
                  </button>
                );
              })
            )}
          </div>

          <div style={{ display: "flex", justifyContent: "flex-end", marginTop: 18 }}>
            <button
              onClick={indexSelectedRepo}
              disabled={!selectedRepo || indexingRepoID === selectedRepoID}
              style={indexButtonStyle(Boolean(selectedRepo))}
            >
              {indexingRepoID === selectedRepoID ? <Loader2 size={15} className="animate-spin" /> : <RefreshCw size={15} />}
              {selectedRepo?.index_status === "ready" ? "Reindex selected repo" : "Index selected repo"}
            </button>
          </div>
        </section>
      </main>
    </SettingsShell>
  );
}

function SettingsShell({ children, embedded = false }: { children: React.ReactNode; embedded?: boolean }) {
  return (
    <div style={{ minHeight: embedded ? "100%" : "100vh", background: "#EBE9E9", color: "#111111" }}>
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

function StatusChip({ children }: { children: React.ReactNode }) {
  return (
    <span style={{
      border: "1px solid #D6D6D6",
      borderRadius: 999,
      background: "#F3F3F3",
      color: "#555555",
      padding: "3px 8px",
      fontSize: 12,
      fontWeight: 650,
      whiteSpace: "nowrap",
    }}>
      {children}
    </span>
  );
}

const mainStyle: React.CSSProperties = {
  width: "min(860px, 100%)",
  margin: "0 auto",
  padding: "48px 24px",
};

const embeddedMainStyle: React.CSSProperties = {
  width: "min(860px, 100%)",
  margin: "0 auto",
  padding: "34px 24px 96px",
  position: "relative",
};

const closeButtonStyle: React.CSSProperties = {
  position: "absolute",
  top: 18,
  right: 24,
  width: 34,
  height: 34,
  borderRadius: 999,
  border: "1px solid #D4D4D4",
  background: "#FFFFFF",
  color: "#111111",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  cursor: "pointer",
  zIndex: 2,
};

const brandStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 8,
  color: "#111111",
  textDecoration: "none",
  fontSize: 15,
  fontWeight: 650,
  marginBottom: 44,
};

const headerStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  gap: 24,
  marginBottom: 24,
};

const eyebrowStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  fontWeight: 700,
  marginBottom: 7,
  textTransform: "uppercase",
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

const avatarStyle: React.CSSProperties = {
  width: 44,
  height: 44,
  borderRadius: "50%",
  flex: "0 0 44px",
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

const currentRepoStyle: React.CSSProperties = {
  minHeight: 58,
  border: "1px solid #E0E0E0",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: "10px 12px",
  display: "flex",
  alignItems: "center",
  gap: 12,
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

const indexButtonStyle = (enabled: boolean): React.CSSProperties => ({
  height: 40,
  borderRadius: 6,
  border: "none",
  background: "#111111",
  color: "#FFFFFF",
  padding: "0 14px",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 8,
  cursor: enabled ? "pointer" : "not-allowed",
  opacity: enabled ? 1 : 0.45,
  fontSize: 13,
  fontWeight: 650,
});

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
