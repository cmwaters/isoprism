"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { useParams, useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { Repository, Team, OrgMember, Organization } from "@/lib/types";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const GITHUB_APP_NAME = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;

// ── Types ─────────────────────────────────────────────────────────────────────

interface GHTeam   { id: number; name: string; slug: string }
interface GHMember { login: string; avatar_url: string; on_aperture: boolean; already_in: boolean }

// ── Page ──────────────────────────────────────────────────────────────────────

export default function SettingsPage() {
  const params  = useParams();
  const router  = useRouter();
  const orgSlug = params.orgSlug as string;

  const [token,       setToken]       = useState<string | null>(null);
  const [org,         setOrg]         = useState<Organization | null>(null);
  const [allOrgs,     setAllOrgs]     = useState<Organization[]>([]);
  const [repos,       setRepos]       = useState<Repository[]>([]);
  const [teams,       setTeams]       = useState<Team[]>([]);
  const [members,     setMembers]     = useState<OrgMember[]>([]);
  const [loadingRepos,    setLoadingRepos]    = useState(true);
  const [loadingTeams,    setLoadingTeams]    = useState(true);
  const [loadingMembers,  setLoadingMembers]  = useState(true);
  const [syncing,  setSyncing]  = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [confirmDelete,   setConfirmDelete]   = useState(false);
  const [deletingAccount, setDeletingAccount] = useState(false);

  const isOrgAccount = org?.github_account_type === "Organization";

  // Auth
  useEffect(() => {
    createClient().auth.getSession().then(({ data: { session } }) => {
      if (session) setToken(session.access_token);
    });
  }, []);

  // Data fetch
  useEffect(() => {
    if (!token) return;
    const h = { Authorization: `Bearer ${token}` };

    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/`,       { headers: h }).then(r => r.ok ? r.json() : null).then(d => { if (d) setOrg(d); });
    fetch(`${API_URL}/api/v1/me/orgs`,                { headers: h }).then(r => r.ok ? r.json() : { orgs: [] }).then(d => setAllOrgs(d.orgs ?? []));

    setLoadingRepos(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos`,  { headers: h }).then(r => r.ok ? r.json() : { repos: [] }).then(d => setRepos(d.repos ?? [])).finally(() => setLoadingRepos(false));

    setLoadingTeams(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/teams`,  { headers: h }).then(r => r.ok ? r.json() : { teams: [] }).then(d => setTeams(d.teams ?? [])).finally(() => setLoadingTeams(false));

    setLoadingMembers(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/members`,{ headers: h }).then(r => r.ok ? r.json() : { members: [] }).then(d => setMembers(d.members ?? [])).finally(() => setLoadingMembers(false));
  }, [orgSlug, token]);

  // Repo actions
  const syncRepo = useCallback(async (id: string) => {
    if (!token || syncing) return;
    setSyncing(id);
    await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${id}/sync`, { method: "POST", headers: { Authorization: `Bearer ${token}` } });
    setSyncing(null);
  }, [token, syncing, orgSlug]);

  const syncAll = useCallback(async () => {
    if (!token || syncing) return;
    setSyncing("all");
    await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/sync`, { method: "POST", headers: { Authorization: `Bearer ${token}` } });
    setSyncing(null);
  }, [token, syncing, orgSlug]);

  const removeRepo = useCallback(async (repo: Repository) => {
    if (!token || removing) return;
    setRemoving(repo.id);
    const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repo.id}`, { method: "DELETE", headers: { Authorization: `Bearer ${token}` } });
    if (res.ok) setRepos(p => p.filter(r => r.id !== repo.id));
    setRemoving(null);
  }, [token, removing, orgSlug]);

  // Team actions
  const addTeam = useCallback(async (ghTeam: GHTeam) => {
    if (!token) return;
    const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/teams`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ name: ghTeam.name }),
    });
    if (res.ok) {
      const team = await res.json();
      setTeams(p => [...p, team]);
    }
  }, [token, orgSlug]);

  const deleteTeam = useCallback(async (id: string) => {
    if (!token) return;
    const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/teams/${id}`, { method: "DELETE", headers: { Authorization: `Bearer ${token}` } });
    if (res.ok) setTeams(p => p.filter(t => t.id !== id));
  }, [token, orgSlug]);

  // Member actions
  const addMember = useCallback(async (login: string): Promise<"ok" | "not_on_aperture"> => {
    if (!token) return "not_on_aperture";
    const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/members`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ github_login: login }),
    });
    if (res.status === 404) return "not_on_aperture";
    if (res.ok) {
      // Refresh members list
      fetch(`${API_URL}/api/v1/orgs/${orgSlug}/members`, { headers: { Authorization: `Bearer ${token}` } })
        .then(r => r.ok ? r.json() : { members: [] }).then(d => setMembers(d.members ?? []));
      return "ok";
    }
    return "not_on_aperture";
  }, [token, orgSlug]);

  // Account delete
  const deleteAccount = useCallback(async () => {
    if (!token) return;
    setDeletingAccount(true);
    await fetch(`${API_URL}/api/v1/me`, { method: "DELETE", headers: { Authorization: `Bearer ${token}` } });
    await createClient().auth.signOut();
    router.push("/login");
  }, [token, router]);

  return (
    <div className="h-screen flex bg-neutral-50">
      <AppSidebar orgSlug={orgSlug} activeTab="settings" />

      <main className="flex-1 overflow-y-auto">

        {/* Org switcher bar */}
        <div className="border-b border-neutral-100 bg-white px-6 py-3">
          <div className="max-w-2xl mx-auto flex items-center gap-1 flex-wrap">
            {allOrgs.map((o) => {
              const isActive = o.slug === orgSlug;
              return (
                <button key={o.id} onClick={() => router.push(`/orgs/${o.slug}/settings`)}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                    isActive ? "bg-neutral-100 text-neutral-900" : "text-neutral-500 hover:text-neutral-700 hover:bg-neutral-50"
                  }`}
                >
                  {o.avatar_url
                    ? <img src={o.avatar_url} alt={o.name} className="h-5 w-5 rounded-full object-cover" />
                    : <div className="h-5 w-5 rounded-full bg-neutral-200 flex items-center justify-center"><span className="text-[10px] font-semibold text-neutral-600">{o.name[0]?.toUpperCase()}</span></div>
                  }
                  {o.name}
                </button>
              );
            })}
            {GITHUB_APP_NAME && (
              <a href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm text-neutral-400 hover:text-neutral-600 hover:bg-neutral-50 transition-colors">
                <svg className="h-4 w-4" viewBox="0 0 16 16" fill="currentColor"><path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" /></svg>
                Add org
              </a>
            )}
          </div>
        </div>

        <div className="max-w-2xl mx-auto px-6 py-10 space-y-10">

          {/* ── Repositories ── */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-semibold text-neutral-900">Repositories</h2>
              <div className="flex items-center gap-2">
                <Btn onClick={syncAll} loading={syncing === "all"} disabled={syncing !== null} icon="sync">
                  {syncing === "all" ? "Syncing…" : "Sync all"}
                </Btn>
                {GITHUB_APP_NAME && (
                  <a href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 transition-colors">
                    <PlusIcon /> Add repositories
                  </a>
                )}
              </div>
            </div>
            <Card>
              {loadingRepos ? <LoadingRow /> : repos.length === 0 ? <EmptyRow message="No repositories connected." /> : (
                <table className="w-full text-sm">
                  <thead><tr className="border-b border-neutral-100">
                    <Th>Repository</Th><Th>Default branch</Th><th className="px-5 py-3" />
                  </tr></thead>
                  <tbody className="divide-y divide-neutral-100">
                    {repos.map(repo => (
                      <tr key={repo.id}>
                        <td className="px-5 py-3 font-medium text-neutral-900">{repo.full_name}</td>
                        <td className="px-5 py-3 font-mono text-neutral-500 text-xs">{repo.default_branch}</td>
                        <td className="px-5 py-3 text-right">
                          <div className="flex items-center justify-end gap-3">
                            <GhostBtn onClick={() => syncRepo(repo.id)} disabled={syncing !== null}>{syncing === repo.id ? "Syncing…" : "Sync"}</GhostBtn>
                            <GhostBtn onClick={() => removeRepo(repo)} disabled={removing === repo.id} danger>{removing === repo.id ? "Removing…" : "Remove"}</GhostBtn>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </Card>
            <p className="text-xs text-neutral-400 mt-2">Sync fetches open PRs and the last 30 days of closed PRs with reviews.</p>
          </section>

          {/* ── Teams — org accounts only ── */}
          {isOrgAccount && (
            <section>
              <div className="flex items-center justify-between mb-3">
                <h2 className="text-sm font-semibold text-neutral-900">Teams</h2>
              </div>
              <Card>
                {loadingTeams ? <LoadingRow /> : teams.length === 0
                  ? <div className="px-5 py-5">
                      <p className="text-sm text-neutral-500">Currently using the entire org as the team.</p>
                      <p className="text-xs text-neutral-400 mt-1">Add a team to scope the queue and flow metrics to a subset of members.</p>
                    </div>
                  : <ul className="divide-y divide-neutral-100">
                      {teams.map(team => (
                        <li key={team.id} className="flex items-center gap-3 px-5 py-3">
                          <TeamIcon />
                          <span className="flex-1 text-sm font-medium text-neutral-900">{team.name}</span>
                          <GhostBtn onClick={() => deleteTeam(team.id)} danger>Remove</GhostBtn>
                        </li>
                      ))}
                    </ul>
                }
                {/* Inline GitHub team search */}
                <div className={teams.length > 0 || !loadingTeams ? "border-t border-neutral-100" : ""}>
                  <GitHubTeamSearch
                    token={token}
                    orgSlug={orgSlug}
                    alreadyAdded={teams.map(t => t.name.toLowerCase())}
                    onSelect={addTeam}
                  />
                </div>
              </Card>
            </section>
          )}

          {/* ── Members — org accounts only ── */}
          {isOrgAccount && (
            <section>
              <h2 className="text-sm font-semibold text-neutral-900 mb-3">Members</h2>
              <Card>
                {loadingMembers ? <LoadingRow /> : members.length === 0
                  ? <EmptyRow message="No members have signed up yet." />
                  : <ul className="divide-y divide-neutral-100">
                      {members.map(m => {
                        const name = m.display_name ?? m.github_username ?? m.email;
                        return (
                          <li key={m.id} className="flex items-center gap-3 px-5 py-3">
                            {m.avatar_url
                              ? <img src={m.avatar_url} alt={name} className="h-7 w-7 rounded-full bg-neutral-100 shrink-0" />
                              : <div className="h-7 w-7 rounded-full bg-neutral-200 flex items-center justify-center shrink-0"><span className="text-xs font-semibold text-neutral-600">{name.slice(0,2).toUpperCase()}</span></div>
                            }
                            <div className="flex-1 min-w-0">
                              <p className="text-sm font-medium text-neutral-900 truncate">{name}</p>
                              {m.github_username && <p className="text-xs text-neutral-400">@{m.github_username}</p>}
                            </div>
                            {m.role === "org_admin"
                              ? <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-neutral-900 text-white">Admin</span>
                              : <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-neutral-100 text-neutral-500">Member</span>
                            }
                          </li>
                        );
                      })}
                    </ul>
                }
                {/* Invite by GitHub username search */}
                <div className={members.length > 0 || !loadingMembers ? "border-t border-neutral-100" : ""}>
                  <GitHubMemberSearch
                    token={token}
                    orgSlug={orgSlug}
                    alreadyIn={members.map(m => m.github_username?.toLowerCase() ?? "")}
                    onAdd={addMember}
                  />
                </div>
              </Card>
              <p className="text-xs text-neutral-400 mt-2">Only members who have signed into Aperture are listed.</p>
            </section>
          )}

          {/* ── Account ── */}
          <section>
            <h2 className="text-sm font-semibold text-neutral-900 mb-3">Account</h2>
            <Card>
              <div className="px-5 py-4 flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium text-neutral-900">Delete account</p>
                  <p className="text-xs text-neutral-400 mt-0.5">Permanently removes your account and all associated data.</p>
                </div>
                {confirmDelete ? (
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-neutral-500">Are you sure?</span>
                    <button onClick={() => setConfirmDelete(false)} className="px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 text-neutral-600 hover:bg-neutral-50 transition-colors">Cancel</button>
                    <button onClick={deleteAccount} disabled={deletingAccount} className="px-3 py-1.5 rounded-lg text-xs font-medium bg-red-600 text-white hover:bg-red-700 transition-colors disabled:opacity-50">{deletingAccount ? "Deleting…" : "Delete"}</button>
                  </div>
                ) : (
                  <button onClick={() => setConfirmDelete(true)} className="px-3 py-1.5 rounded-lg text-xs font-medium border border-red-200 text-red-600 hover:bg-red-50 transition-colors">Delete account</button>
                )}
              </div>
            </Card>
          </section>

        </div>
      </main>
    </div>
  );
}

// ── GitHub team search combobox ───────────────────────────────────────────────

function GitHubTeamSearch({ token, orgSlug, alreadyAdded, onSelect }: {
  token: string | null;
  orgSlug: string;
  alreadyAdded: string[];
  onSelect: (team: GHTeam) => Promise<void>;
}) {
  const [query,   setQuery]   = useState("");
  const [results, setResults] = useState<GHTeam[]>([]);
  const [loading, setLoading] = useState(false);
  const [adding,  setAdding]  = useState<number | null>(null);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!token) return;
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(async () => {
      setLoading(true);
      try {
        const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/github/teams?q=${encodeURIComponent(query)}`, { headers: { Authorization: `Bearer ${token}` } });
        const d = res.ok ? await res.json() : { teams: [] };
        setResults((d.teams ?? []).filter((t: GHTeam) => !alreadyAdded.includes(t.name.toLowerCase())));
      } finally { setLoading(false); }
    }, 300);
  }, [query, token, orgSlug, alreadyAdded]);

  const select = async (team: GHTeam) => {
    setAdding(team.id);
    await onSelect(team);
    setResults(p => p.filter(t => t.id !== team.id));
    setAdding(null);
  };

  return (
    <div className="px-5 py-3">
      <input
        type="text"
        value={query}
        onChange={e => setQuery(e.target.value)}
        placeholder="Search GitHub teams to add…"
        className="w-full text-sm px-3 py-1.5 rounded-lg border border-neutral-200 bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-neutral-300 placeholder:text-neutral-400"
      />
      {(results.length > 0 || loading) && (
        <ul className="mt-2 divide-y divide-neutral-100 rounded-lg border border-neutral-200 bg-white overflow-hidden">
          {loading && results.length === 0 && <li className="px-3 py-2 text-xs text-neutral-400">Searching…</li>}
          {results.map(team => (
            <li key={team.id} className="flex items-center justify-between px-3 py-2 hover:bg-neutral-50">
              <div className="flex items-center gap-2">
                <TeamIcon small />
                <span className="text-sm text-neutral-900">{team.name}</span>
                <span className="text-xs text-neutral-400">/{team.slug}</span>
              </div>
              <button
                onClick={() => select(team)}
                disabled={adding === team.id}
                className="text-xs font-medium text-neutral-600 hover:text-neutral-900 disabled:opacity-50"
              >
                {adding === team.id ? "Adding…" : "Add"}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ── GitHub member search combobox ─────────────────────────────────────────────

function GitHubMemberSearch({ token, orgSlug, alreadyIn, onAdd }: {
  token: string | null;
  orgSlug: string;
  alreadyIn: string[];
  onAdd: (login: string) => Promise<"ok" | "not_on_aperture">;
}) {
  const [query,   setQuery]   = useState("");
  const [results, setResults] = useState<GHMember[]>([]);
  const [loading, setLoading] = useState(false);
  const [adding,  setAdding]  = useState<string | null>(null);
  const [notice,  setNotice]  = useState<{ login: string; msg: string } | null>(null);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!token || !query.trim()) { setResults([]); return; }
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(async () => {
      setLoading(true);
      try {
        const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/github/members?q=${encodeURIComponent(query)}`, { headers: { Authorization: `Bearer ${token}` } });
        const d = res.ok ? await res.json() : { members: [] };
        setResults((d.members ?? []).filter((m: GHMember) => !alreadyIn.includes(m.login.toLowerCase())));
      } finally { setLoading(false); }
    }, 300);
  }, [query, token, orgSlug, alreadyIn]);

  const invite = async (m: GHMember) => {
    if (!m.on_aperture) {
      setNotice({ login: m.login, msg: `@${m.login} hasn't signed into Aperture yet.` });
      setTimeout(() => setNotice(null), 3000);
      return;
    }
    setAdding(m.login);
    const result = await onAdd(m.login);
    if (result === "ok") {
      setResults(p => p.filter(r => r.login !== m.login));
      setQuery("");
    } else {
      setNotice({ login: m.login, msg: `@${m.login} hasn't signed into Aperture yet.` });
      setTimeout(() => setNotice(null), 3000);
    }
    setAdding(null);
  };

  return (
    <div className="px-5 py-3">
      <input
        type="text"
        value={query}
        onChange={e => setQuery(e.target.value)}
        placeholder="Search GitHub members to invite…"
        className="w-full text-sm px-3 py-1.5 rounded-lg border border-neutral-200 bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-neutral-300 placeholder:text-neutral-400"
      />
      {notice && (
        <p className="mt-1.5 text-xs text-amber-600">{notice.msg}</p>
      )}
      {(results.length > 0 || loading) && (
        <ul className="mt-2 divide-y divide-neutral-100 rounded-lg border border-neutral-200 bg-white overflow-hidden">
          {loading && results.length === 0 && <li className="px-3 py-2 text-xs text-neutral-400">Searching…</li>}
          {results.map(m => (
            <li key={m.login} className="flex items-center justify-between px-3 py-2 hover:bg-neutral-50">
              <div className="flex items-center gap-2">
                <img src={m.avatar_url || `https://github.com/${m.login}.png`} alt={m.login} className="h-5 w-5 rounded-full bg-neutral-100" />
                <span className="text-sm text-neutral-900">@{m.login}</span>
                {!m.on_aperture && (
                  <span className="text-[10px] text-neutral-400 bg-neutral-100 px-1.5 py-0.5 rounded-full">Not on Aperture</span>
                )}
              </div>
              <button
                onClick={() => invite(m)}
                disabled={adding === m.login || m.already_in}
                className={`text-xs font-medium disabled:opacity-40 ${m.on_aperture ? "text-neutral-600 hover:text-neutral-900" : "text-neutral-400 cursor-default"}`}
              >
                {adding === m.login ? "Adding…" : m.already_in ? "Already added" : m.on_aperture ? "Add" : "Invite"}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ── Primitives ────────────────────────────────────────────────────────────────

function Card({ children }: { children: React.ReactNode }) {
  return <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">{children}</div>;
}

function Th({ children }: { children: React.ReactNode }) {
  return <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">{children}</th>;
}

function LoadingRow() {
  return <div className="px-5 py-6 text-center"><p className="text-sm text-neutral-400">Loading…</p></div>;
}

function EmptyRow({ message }: { message: string }) {
  return <div className="px-5 py-6 text-center"><p className="text-sm text-neutral-400">{message}</p></div>;
}

function GhostBtn({ onClick, disabled, danger, children }: { onClick: () => void; disabled?: boolean; danger?: boolean; children: React.ReactNode }) {
  return (
    <button onClick={onClick} disabled={disabled}
      className={`text-xs transition-colors disabled:opacity-50 ${danger ? "text-neutral-400 hover:text-red-500" : "text-neutral-400 hover:text-neutral-700"}`}>
      {children}
    </button>
  );
}

function Btn({ onClick, loading, disabled, icon, children }: { onClick: () => void; loading?: boolean; disabled?: boolean; icon?: string; children: React.ReactNode }) {
  return (
    <button onClick={onClick} disabled={disabled}
      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 transition-colors disabled:opacity-50">
      {icon === "sync" && (
        <svg className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M13.5 8a5.5 5.5 0 1 1-1.3-3.5" strokeLinecap="round" /><path d="M13.5 2v3.5H10" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      )}
      {children}
    </button>
  );
}

function PlusIcon() {
  return <svg className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor"><path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" /></svg>;
}

function TeamIcon({ small }: { small?: boolean }) {
  const s = small ? "h-3.5 w-3.5" : "h-4 w-4";
  return (
    <svg className={`${s} text-neutral-400`} fill="none" stroke="currentColor" strokeWidth={1.75} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z" />
    </svg>
  );
}
