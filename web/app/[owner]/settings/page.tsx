"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowUpRight,
  Building2,
  Check,
  ChevronRight,
  Github,
  GitBranch,
  Loader2,
  RefreshCw,
  Search,
  ShieldAlert,
  User,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { apiFetch } from "@/lib/api";
import { createClient } from "@/lib/supabase/client";
import { Repository } from "@/lib/types";

type Category = "overview" | "github" | "repositories";

type GitHubOrg = {
  login: string;
  id: number;
  avatar_url: string;
  description?: string | null;
};

type GitHubUser = {
  login: string;
  name?: string;
  avatarURL?: string;
};

type AccountSummary = {
  login: string;
  name: string;
  avatarURL?: string;
  type: "user" | "org";
};

const categories: { id: Category; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "github", label: "GitHub" },
  { id: "repositories", label: "Repositories" },
];

export default function SettingsPage() {
  const params = useParams<{ owner: string }>();
  const router = useRouter();
  const supabase = useMemo(() => createClient(), []);
  const account = decodeURIComponent(params.owner);

  const [category, setCategory] = useState<Category>("overview");
  const [currentUser, setCurrentUser] = useState<GitHubUser | null>(null);
  const [orgs, setOrgs] = useState<GitHubOrg[]>([]);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [loading, setLoading] = useState(true);
  const [repoActionID, setRepoActionID] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [error, setError] = useState("");
  const [orgNotice, setOrgNotice] = useState("");

  useEffect(() => {
    let active = true;

    async function loadSettings() {
      setLoading(true);
      setError("");
      setOrgNotice("");

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

      const user: GitHubUser = {
        login,
        name: metadata.full_name ?? metadata.name ?? login,
        avatarURL: metadata.avatar_url ?? metadata.picture,
      };

      try {
        const [{ repos: repoList }, discoveredOrgs] = await Promise.all([
          apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token),
          loadGitHubOrgs(session.provider_token ?? undefined),
        ]);

        if (!active) return;
        setCurrentUser(user);
        setRepos(repoList ?? []);
        setOrgs(discoveredOrgs.orgs);
        setOrgNotice(discoveredOrgs.notice);
      } catch {
        if (!active) return;
        setCurrentUser(user);
        setError("Settings could not be fully loaded.");
      } finally {
        if (active) setLoading(false);
      }
    }

    loadSettings();

    return () => {
      active = false;
    };
  }, [account, router, supabase]);

  const installedOwners = useMemo(() => {
    const owners = new Set<string>();
    for (const repo of repos) {
      const owner = repo.full_name.split("/")[0];
      if (owner) owners.add(owner.toLowerCase());
    }
    return owners;
  }, [repos]);

  const accountSummary = useMemo<AccountSummary>(() => {
    const org = orgs.find((candidate) => sameLogin(candidate.login, account));
    if (org) {
      return {
        login: org.login,
        name: org.login,
        avatarURL: org.avatar_url,
        type: "org",
      };
    }

    if (currentUser && sameLogin(currentUser.login, account)) {
      return {
        login: currentUser.login,
        name: currentUser.name ?? currentUser.login,
        avatarURL: currentUser.avatarURL,
        type: "user",
      };
    }

    return {
      login: account,
      name: account,
      type: sameLogin(account, currentUser?.login ?? "") ? "user" : "org",
    };
  }, [account, currentUser, orgs]);

  const accountRepos = useMemo(() => {
    return repos
      .filter((repo) => sameLogin(repo.full_name.split("/")[0], accountSummary.login))
      .filter((repo) => repo.full_name.toLowerCase().includes(search.toLowerCase()));
  }, [accountSummary.login, repos, search]);

  const personalRepos = useMemo(() => {
    if (!currentUser) return [];
    return repos.filter((repo) => sameLogin(repo.full_name.split("/")[0], currentUser.login));
  }, [currentUser, repos]);

  const isInstalled = accountRepos.length > 0 || installedOwners.has(accountSummary.login.toLowerCase());
  const isCurrentUserPage = currentUser ? sameLogin(accountSummary.login, currentUser.login) : false;
  const appName = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;

  async function handleIndex(repo: Repository) {
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    setRepoActionID(repo.id);
    try {
      await apiFetch(`/api/v1/repos/${repo.id}/index`, token, { method: "POST" });
      setRepos((current) =>
        current.map((candidate) =>
          candidate.id === repo.id ? { ...candidate, index_status: "running" } : candidate
        )
      );
    } catch {
      setError("Repository indexing could not be started.");
    } finally {
      setRepoActionID(null);
    }
  }

  if (loading) {
    return (
      <SettingsFrame>
        <div style={loadingStyle}>
          <Loader2 size={22} className="animate-spin" />
          <span>Loading settings</span>
        </div>
      </SettingsFrame>
    );
  }

  return (
    <SettingsFrame>
      <aside className="settings-sidebar" style={sidebarStyle}>
        <Link href="/" style={brandStyle}>
          <GraphLogo />
          <span>Isoprism</span>
        </Link>

        <div style={accountCardStyle}>
          <Avatar account={accountSummary} size={44} />
          <div style={{ minWidth: 0 }}>
            <div style={accountNameStyle}>{accountSummary.name}</div>
            <div style={mutedTextStyle}>{accountSummary.type === "user" ? "User settings" : "Organization settings"}</div>
          </div>
        </div>

        <nav style={{ display: "grid", gap: 4 }}>
          {categories.map((item) => (
            <button
              key={item.id}
              onClick={() => setCategory(item.id)}
              style={navButtonStyle(category === item.id)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      <main className="settings-main" style={mainStyle}>
        <header style={headerStyle}>
          <div>
            <div style={eyebrowStyle}>{accountSummary.type === "user" ? "User account" : "Organization"}</div>
            <h1 style={titleStyle}>Settings</h1>
          </div>
          <StatusChip tone={isInstalled ? "success" : "neutral"}>
            {isInstalled ? "GitHub App installed" : "GitHub App not installed"}
          </StatusChip>
        </header>

        {error && <Notice tone="error">{error}</Notice>}

        {category === "overview" && (
          <OverviewPanel
            account={accountSummary}
            currentUser={currentUser}
            orgs={orgs}
            orgNotice={orgNotice}
            repos={accountRepos}
            personalRepos={personalRepos}
            installedOwners={installedOwners}
            isCurrentUserPage={isCurrentUserPage}
          />
        )}

        {category === "github" && (
          <GitHubPanel
            account={accountSummary}
            isInstalled={isInstalled}
            appName={appName}
          />
        )}

        {category === "repositories" && (
          <RepositoriesPanel
            repos={accountRepos}
            search={search}
            onSearch={setSearch}
            account={accountSummary}
            isInstalled={isInstalled}
            repoActionID={repoActionID}
            onIndex={handleIndex}
            appName={appName}
          />
        )}
      </main>
    </SettingsFrame>
  );
}

async function loadGitHubOrgs(providerToken?: string): Promise<{ orgs: GitHubOrg[]; notice: string }> {
  if (!providerToken) {
    return {
      orgs: [],
      notice: "GitHub did not provide an org discovery token for this session. Sign out and back in to refresh GitHub permissions.",
    };
  }

  try {
    const response = await fetch("https://api.github.com/user/orgs?per_page=100", {
      headers: {
        Accept: "application/vnd.github+json",
        Authorization: `Bearer ${providerToken}`,
        "X-GitHub-Api-Version": "2022-11-28",
      },
    });

    if (!response.ok) {
      return {
        orgs: [],
        notice: "GitHub org memberships could not be loaded for this session.",
      };
    }

    const orgs = (await response.json()) as GitHubOrg[];
    return { orgs, notice: "" };
  } catch {
    return {
      orgs: [],
      notice: "GitHub org memberships could not be loaded for this session.",
    };
  }
}

function OverviewPanel({
  account,
  currentUser,
  orgs,
  orgNotice,
  repos,
  personalRepos,
  installedOwners,
  isCurrentUserPage,
}: {
  account: AccountSummary;
  currentUser: GitHubUser | null;
  orgs: GitHubOrg[];
  orgNotice: string;
  repos: Repository[];
  personalRepos: Repository[];
  installedOwners: Set<string>;
  isCurrentUserPage: boolean;
}) {
  return (
    <div style={panelStackStyle}>
      <section style={sectionStyle}>
        <div style={sectionHeaderStyle}>
          <div>
            <h2 style={sectionTitleStyle}>Account</h2>
            <p style={sectionCopyStyle}>
              Settings are scoped to this GitHub {account.type === "user" ? "user" : "organization"} account.
            </p>
          </div>
        </div>
        <div style={summaryGridStyle}>
          <SummaryMetric label="GitHub account" value={account.login} />
          <SummaryMetric label="Accessible repos" value={String(repos.length)} />
          <SummaryMetric label="Context" value={account.type === "user" ? "User" : "Organization"} />
        </div>
      </section>

      {isCurrentUserPage && (
        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <div>
              <h2 style={sectionTitleStyle}>Organizations</h2>
              <p style={sectionCopyStyle}>
                Open an organization settings page before changing GitHub App access.
              </p>
            </div>
          </div>

          {orgNotice && <Notice tone="neutral">{orgNotice}</Notice>}

          <div style={listStyle}>
            {orgs.length === 0 ? (
              <EmptyState
                icon={<Building2 size={18} />}
                title="No organizations found"
                copy="GitHub has not exposed any organization memberships to this session."
              />
            ) : (
              orgs.map((org) => {
                const installed = installedOwners.has(org.login.toLowerCase());
                return (
                  <Link key={org.id} href={`/${encodeURIComponent(org.login)}/settings`} style={rowLinkStyle}>
                    <Avatar account={{ login: org.login, name: org.login, avatarURL: org.avatar_url, type: "org" }} size={36} />
                    <div style={rowContentStyle}>
                      <div style={rowTitleStyle}>{org.login}</div>
                      <div style={rowMetaStyle}>
                        Member · {installed ? "GitHub App installed" : "GitHub App not installed"}
                      </div>
                    </div>
                    <StatusChip tone={installed ? "success" : "neutral"}>
                      {installed ? "Installed" : "Open settings"}
                    </StatusChip>
                    <ChevronRight size={16} color="#777777" />
                  </Link>
                );
              })
            )}
          </div>
        </section>
      )}

      {!isCurrentUserPage && currentUser && (
        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <div>
              <h2 style={sectionTitleStyle}>Signed in as</h2>
              <p style={sectionCopyStyle}>Your personal settings remain available from the account pill.</p>
            </div>
            <Link href={`/${encodeURIComponent(currentUser.login)}/settings`} style={secondaryActionStyle}>
              <User size={14} />
              {currentUser.login}
            </Link>
          </div>
        </section>
      )}

      {isCurrentUserPage && personalRepos.length > 0 && (
        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <div>
              <h2 style={sectionTitleStyle}>Personal repositories</h2>
              <p style={sectionCopyStyle}>Repositories available through your personal GitHub App installation.</p>
            </div>
          </div>
          <RepoPreview repos={personalRepos} />
        </section>
      )}
    </div>
  );
}

function GitHubPanel({
  account,
  isInstalled,
  appName,
}: {
  account: AccountSummary;
  isInstalled: boolean;
  appName?: string;
}) {
  const installURL = appName ? `https://github.com/apps/${appName}/installations/new` : undefined;
  const manageURL =
    account.type === "org"
      ? `https://github.com/organizations/${encodeURIComponent(account.login)}/settings/installations`
      : "https://github.com/settings/installations";

  return (
    <div style={panelStackStyle}>
      <section style={sectionStyle}>
        <div style={sectionHeaderStyle}>
          <div>
            <h2 style={sectionTitleStyle}>GitHub App</h2>
            <p style={sectionCopyStyle}>
              GitHub controls installation and repository permission boundaries for this account.
            </p>
          </div>
          <StatusChip tone={isInstalled ? "success" : "neutral"}>
            {isInstalled ? "Installed" : "Not installed"}
          </StatusChip>
        </div>

        <div style={githubStateStyle}>
          <Github size={28} />
          <div>
            <div style={{ fontSize: 15, fontWeight: 650, color: "#111111" }}>
              Isoprism for {account.login}
            </div>
            <div style={sectionCopyStyle}>
              {isInstalled
                ? "This account has repositories available to Isoprism."
                : "Install the GitHub App for this account to grant repository access."}
            </div>
          </div>
        </div>

        <div style={actionRowStyle}>
          {isInstalled ? (
            <a href={manageURL} style={primaryActionStyle}>
              <ArrowUpRight size={15} />
              Manage GitHub App
            </a>
          ) : installURL ? (
            <a href={installURL} style={primaryActionStyle}>
              <ArrowUpRight size={15} />
              Install GitHub App
            </a>
          ) : (
            <button disabled style={disabledActionStyle}>
              GitHub App not configured
            </button>
          )}
        </div>
      </section>

      <section style={sectionStyle}>
        <div style={sectionHeaderStyle}>
          <div>
            <h2 style={sectionTitleStyle}>Permission model</h2>
            <p style={sectionCopyStyle}>
              GitHub App access and Isoprism repository enablement are separate states.
            </p>
          </div>
        </div>
        <div style={permissionGridStyle}>
          <PermissionCard
            title="GitHub controls"
            copy="Which account the App is installed on, and whether it can access all or selected repositories."
          />
          <PermissionCard
            title="Isoprism controls"
            copy="Which accessible repositories are indexed, shown, and used inside the product."
          />
        </div>
      </section>
    </div>
  );
}

function RepositoriesPanel({
  repos,
  search,
  onSearch,
  account,
  isInstalled,
  repoActionID,
  onIndex,
  appName,
}: {
  repos: Repository[];
  search: string;
  onSearch: (value: string) => void;
  account: AccountSummary;
  isInstalled: boolean;
  repoActionID: string | null;
  onIndex: (repo: Repository) => void;
  appName?: string;
}) {
  const installURL = appName ? `https://github.com/apps/${appName}/installations/new` : undefined;

  return (
    <section style={sectionStyle}>
      <div style={sectionHeaderStyle}>
        <div>
          <h2 style={sectionTitleStyle}>Repositories</h2>
          <p style={sectionCopyStyle}>
            Add repositories by indexing them. GitHub-side removals are managed through the GitHub App installation.
          </p>
        </div>
      </div>

      <label style={searchBoxStyle}>
        <Search size={15} color="#888888" />
        <input
          value={search}
          onChange={(event) => onSearch(event.target.value)}
          placeholder="Search repositories"
          style={searchInputStyle}
        />
      </label>

      {!isInstalled && repos.length === 0 ? (
        <EmptyState
          icon={<ShieldAlert size={18} />}
          title="GitHub App not installed"
          copy={`Install the GitHub App for ${account.login} before repositories can appear here.`}
          action={
            installURL ? (
              <a href={installURL} style={primaryActionStyle}>
                <ArrowUpRight size={15} />
                Install GitHub App
              </a>
            ) : undefined
          }
        />
      ) : repos.length === 0 ? (
        <EmptyState
          icon={<GitBranch size={18} />}
          title="No repositories match"
          copy="Try another search or update GitHub App repository access."
        />
      ) : (
        <div style={listStyle}>
          {repos.map((repo) => (
            <div key={repo.id} style={rowStyle}>
              <GitBranch size={18} color="#555555" />
              <div style={rowContentStyle}>
                <div style={rowTitleStyle}>{repo.full_name}</div>
                <div style={rowMetaStyle}>
                  {repo.default_branch || "default branch"} · {repo.index_status}
                </div>
              </div>
              <StatusChip tone={repo.index_status === "ready" ? "success" : "neutral"}>
                {repo.index_status === "ready" ? "Indexed" : repo.index_status}
              </StatusChip>
              {repo.index_status === "ready" ? (
                <Link href={`/${repo.full_name}`} style={secondaryActionStyle}>
                  Open
                </Link>
              ) : (
                <button
                  onClick={() => onIndex(repo)}
                  disabled={repoActionID === repo.id}
                  style={secondaryButtonStyle}
                >
                  {repoActionID === repo.id ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                  {repo.index_status === "failed" ? "Retry" : "Add"}
                </button>
              )}
              <a
                href={
                  account.type === "org"
                    ? `https://github.com/organizations/${encodeURIComponent(account.login)}/settings/installations`
                    : "https://github.com/settings/installations"
                }
                style={subtleLinkStyle}
              >
                Remove
              </a>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function RepoPreview({ repos }: { repos: Repository[] }) {
  return (
    <div style={listStyle}>
      {repos.slice(0, 5).map((repo) => (
        <Link key={repo.id} href={`/${repo.full_name}`} style={rowLinkStyle}>
          <GitBranch size={18} color="#555555" />
          <div style={rowContentStyle}>
            <div style={rowTitleStyle}>{repo.full_name}</div>
            <div style={rowMetaStyle}>
              {repo.default_branch} · {repo.index_status}
            </div>
          </div>
          <ChevronRight size={16} color="#777777" />
        </Link>
      ))}
    </div>
  );
}

function SummaryMetric({ label, value }: { label: string; value: string }) {
  return (
    <div style={metricStyle}>
      <div style={metricLabelStyle}>{label}</div>
      <div style={metricValueStyle}>{value}</div>
    </div>
  );
}

function PermissionCard({ title, copy }: { title: string; copy: string }) {
  return (
    <div style={permissionCardStyle}>
      <Check size={17} color="#166534" />
      <div>
        <div style={rowTitleStyle}>{title}</div>
        <div style={rowMetaStyle}>{copy}</div>
      </div>
    </div>
  );
}

function EmptyState({
  icon,
  title,
  copy,
  action,
}: {
  icon: React.ReactNode;
  title: string;
  copy: string;
  action?: React.ReactNode;
}) {
  return (
    <div style={emptyStyle}>
      <div style={emptyIconStyle}>{icon}</div>
      <div style={{ fontSize: 14, fontWeight: 650, color: "#111111" }}>{title}</div>
      <p style={{ ...sectionCopyStyle, margin: "6px 0 0", maxWidth: 420 }}>{copy}</p>
      {action && <div style={{ marginTop: 16 }}>{action}</div>}
    </div>
  );
}

function Notice({ children, tone }: { children: React.ReactNode; tone: "error" | "neutral" }) {
  return (
    <div
      className="settings-shell"
      style={{
        border: `1px solid ${tone === "error" ? "#F3B4B4" : "#D6D6D6"}`,
        background: tone === "error" ? "#FFF1F1" : "#F6F6F6",
        borderRadius: 8,
        padding: "10px 12px",
        color: tone === "error" ? "#991B1B" : "#555555",
        fontSize: 13,
        lineHeight: 1.45,
      }}
    >
      {children}
    </div>
  );
}

function StatusChip({ children, tone }: { children: React.ReactNode; tone: "success" | "neutral" }) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        minHeight: 24,
        borderRadius: 999,
        padding: "3px 9px",
        border: `1px solid ${tone === "success" ? "#BFE2C5" : "#D6D6D6"}`,
        background: tone === "success" ? "#EEF8F0" : "#F3F3F3",
        color: tone === "success" ? "#166534" : "#555555",
        fontSize: 12,
        fontWeight: 600,
        whiteSpace: "nowrap",
      }}
    >
      {children}
    </span>
  );
}

function Avatar({ account, size }: { account: AccountSummary; size: number }) {
  return (
    <div
      aria-hidden="true"
      style={{
        width: size,
        height: size,
        flex: `0 0 ${size}px`,
        borderRadius: account.type === "org" ? 8 : "50%",
        background: account.avatarURL ? `url(${account.avatarURL}) center / cover` : "#111111",
        color: "#FFFFFF",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        fontSize: Math.max(12, size * 0.32),
        fontWeight: 700,
      }}
    >
      {!account.avatarURL ? account.login.slice(0, 1).toUpperCase() : null}
    </div>
  );
}

function SettingsFrame({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        minHeight: "100vh",
        background: "#EBE9E9",
        color: "#111111",
        display: "flex",
      }}
    >
      {children}
    </div>
  );
}

function GraphLogo() {
  return (
    <svg width="24" height="24" viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <circle cx="8" cy="16" r="4" fill="#111111" />
      <circle cx="24" cy="8" r="4" fill="#111111" />
      <circle cx="24" cy="24" r="4" fill="#111111" />
      <line x1="12" y1="14" x2="20" y2="10" stroke="#111111" strokeWidth="1.5" />
      <line x1="12" y1="18" x2="20" y2="22" stroke="#111111" strokeWidth="1.5" />
    </svg>
  );
}

function sameLogin(left: string, right: string) {
  return left.toLowerCase() === right.toLowerCase();
}

const sidebarStyle: React.CSSProperties = {
  width: 264,
  flex: "0 0 264px",
  minHeight: "100vh",
  borderRight: "1px solid #D4D4D4",
  background: "#E1E1E1",
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 20,
};

const brandStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 8,
  color: "#111111",
  textDecoration: "none",
  fontSize: 15,
  fontWeight: 650,
};

const accountCardStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 11,
  padding: 12,
  border: "1px solid #D4D4D4",
  borderRadius: 8,
  background: "#F2F2F2",
};

const accountNameStyle: React.CSSProperties = {
  color: "#111111",
  fontSize: 14,
  fontWeight: 650,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const mutedTextStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  marginTop: 2,
};

const navButtonStyle = (active: boolean): React.CSSProperties => ({
  height: 36,
  border: "none",
  borderRadius: 6,
  background: active ? "#D6D6D6" : "transparent",
  color: active ? "#111111" : "#555555",
  padding: "0 10px",
  textAlign: "left",
  fontSize: 13,
  fontWeight: active ? 650 : 500,
  cursor: "pointer",
});

const mainStyle: React.CSSProperties = {
  flex: 1,
  maxWidth: 1040,
  padding: "72px 48px 48px",
};

const headerStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  gap: 24,
  marginBottom: 28,
};

const eyebrowStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  fontWeight: 650,
  marginBottom: 6,
  textTransform: "uppercase",
};

const titleStyle: React.CSSProperties = {
  margin: 0,
  color: "#111111",
  fontSize: 28,
  lineHeight: 1.18,
  fontWeight: 700,
};

const panelStackStyle: React.CSSProperties = {
  display: "grid",
  gap: 16,
};

const sectionStyle: React.CSSProperties = {
  border: "1px solid #D4D4D4",
  borderRadius: 8,
  background: "#FFFFFF",
  padding: 20,
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

const sectionCopyStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 13,
  lineHeight: 1.5,
  margin: "5px 0 0",
};

const summaryGridStyle: React.CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(170px, 1fr))",
  gap: 10,
};

const metricStyle: React.CSSProperties = {
  border: "1px solid #E0E0E0",
  borderRadius: 8,
  background: "#F8F8F8",
  padding: 12,
};

const metricLabelStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  marginBottom: 6,
};

const metricValueStyle: React.CSSProperties = {
  color: "#111111",
  fontSize: 15,
  fontWeight: 700,
  overflow: "hidden",
  textOverflow: "ellipsis",
};

const listStyle: React.CSSProperties = {
  display: "grid",
  gap: 8,
};

const rowStyle: React.CSSProperties = {
  minHeight: 58,
  border: "1px solid #E0E0E0",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: "10px 12px",
  display: "flex",
  alignItems: "center",
  gap: 12,
};

const rowLinkStyle: React.CSSProperties = {
  ...rowStyle,
  color: "#111111",
  textDecoration: "none",
};

const rowContentStyle: React.CSSProperties = {
  minWidth: 0,
  flex: 1,
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
  minHeight: 32,
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

const secondaryButtonStyle: React.CSSProperties = {
  ...secondaryActionStyle,
  cursor: "pointer",
};

const disabledActionStyle: React.CSSProperties = {
  ...secondaryActionStyle,
  opacity: 0.5,
  cursor: "not-allowed",
};

const subtleLinkStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 12,
  textDecoration: "none",
  whiteSpace: "nowrap",
};

const actionRowStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "flex-start",
  marginTop: 18,
};

const githubStateStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 14,
  border: "1px solid #E0E0E0",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: 14,
};

const permissionGridStyle: React.CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))",
  gap: 10,
};

const permissionCardStyle: React.CSSProperties = {
  border: "1px solid #E0E0E0",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: 12,
  display: "flex",
  gap: 10,
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

const emptyStyle: React.CSSProperties = {
  border: "1px dashed #D4D4D4",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: "32px 20px",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  textAlign: "center",
};

const emptyIconStyle: React.CSSProperties = {
  width: 34,
  height: 34,
  borderRadius: 8,
  background: "#EEEEEE",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  color: "#555555",
  marginBottom: 10,
};

const loadingStyle: React.CSSProperties = {
  minHeight: "100vh",
  width: "100%",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 10,
  color: "#555555",
  fontSize: 14,
};
