"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { useParams, useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { Repository, Team, OrgMember, Organization } from "@/lib/types";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const GITHUB_APP_NAME = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;

export default function SettingsPage() {
  const params = useParams();
  const router = useRouter();
  const orgSlug = params.orgSlug as string;

  const [token, setToken] = useState<string | null>(null);
  const [org, setOrg] = useState<Organization | null>(null);
  const [allOrgs, setAllOrgs] = useState<Organization[]>([]);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(true);
  const [loadingTeams, setLoadingTeams] = useState(true);
  const [loadingMembers, setLoadingMembers] = useState(true);
  const isOrgAccount = org?.github_account_type === "Organization";
  const [syncing, setSyncing] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [deletingAccount, setDeletingAccount] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [newTeamName, setNewTeamName] = useState("");
  const [addingTeam, setAddingTeam] = useState(false);
  const [showAddTeam, setShowAddTeam] = useState(false);
  const teamInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    async function init() {
      const supabase = createClient();
      const { data: { session } } = await supabase.auth.getSession();
      if (!session) return;
      setToken(session.access_token);
    }
    init();
  }, []);

  useEffect(() => {
    if (!token) return;

    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => r.ok ? r.json() : null)
      .then((d) => { if (d) setOrg(d); })
      .catch(() => {});

    fetch(`${API_URL}/api/v1/me/orgs`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => r.ok ? r.json() : { orgs: [] })
      .then((d) => setAllOrgs(d.orgs ?? []))
      .catch(() => {});

    setLoadingRepos(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => r.ok ? r.json() : { repos: [] })
      .then((d) => setRepos(d.repos ?? []))
      .catch(() => {})
      .finally(() => setLoadingRepos(false));

    setLoadingTeams(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/teams`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => r.ok ? r.json() : { teams: [] })
      .then((d) => setTeams(d.teams ?? []))
      .catch(() => {})
      .finally(() => setLoadingTeams(false));

    setLoadingMembers(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/members`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => r.ok ? r.json() : { members: [] })
      .then((d) => setMembers(d.members ?? []))
      .catch(() => {})
      .finally(() => setLoadingMembers(false));
  }, [orgSlug, token]);

  useEffect(() => {
    if (showAddTeam) teamInputRef.current?.focus();
  }, [showAddTeam]);

  const syncRepo = useCallback(async (repoID: string) => {
    if (!token || syncing) return;
    setSyncing(repoID);
    try {
      await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repoID}/sync`, {
        method: "POST", headers: { Authorization: `Bearer ${token}` },
      });
    } finally { setSyncing(null); }
  }, [token, syncing, orgSlug]);

  const syncAll = useCallback(async () => {
    if (!token || syncing) return;
    setSyncing("all");
    try {
      await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/sync`, {
        method: "POST", headers: { Authorization: `Bearer ${token}` },
      });
    } finally { setSyncing(null); }
  }, [token, syncing, orgSlug]);

  const removeRepo = useCallback(async (repo: Repository) => {
    if (!token || removing) return;
    setRemoving(repo.id);
    try {
      const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repo.id}`, {
        method: "DELETE", headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) setRepos((prev) => prev.filter((r) => r.id !== repo.id));
    } finally { setRemoving(null); }
  }, [token, removing, orgSlug]);

  const addTeam = useCallback(async () => {
    const name = newTeamName.trim();
    if (!token || !name || addingTeam) return;
    setAddingTeam(true);
    try {
      const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/teams`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (res.ok) {
        const team = await res.json();
        setTeams((prev) => [...prev, team]);
        setNewTeamName("");
        setShowAddTeam(false);
      }
    } finally { setAddingTeam(false); }
  }, [token, newTeamName, addingTeam, orgSlug]);

  const deleteTeam = useCallback(async (teamID: string) => {
    if (!token) return;
    const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/teams/${teamID}`, {
      method: "DELETE", headers: { Authorization: `Bearer ${token}` },
    });
    if (res.ok) setTeams((prev) => prev.filter((t) => t.id !== teamID));
  }, [token, orgSlug]);

  const deleteAccount = useCallback(async () => {
    if (!token) return;
    setDeletingAccount(true);
    try {
      await fetch(`${API_URL}/api/v1/me`, { method: "DELETE", headers: { Authorization: `Bearer ${token}` } });
      const supabase = createClient();
      await supabase.auth.signOut();
      router.push("/login");
    } catch { setDeletingAccount(false); }
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
              const letter = o.name[0]?.toUpperCase() ?? "?";
              return (
                <button
                  key={o.id}
                  onClick={() => router.push(`/orgs/${o.slug}/settings`)}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                    isActive
                      ? "bg-neutral-100 text-neutral-900"
                      : "text-neutral-500 hover:text-neutral-700 hover:bg-neutral-50"
                  }`}
                >
                  {o.avatar_url ? (
                    <img src={o.avatar_url} alt={o.name} className="h-5 w-5 rounded-full object-cover" />
                  ) : (
                    <div className="h-5 w-5 rounded-full bg-neutral-200 flex items-center justify-center">
                      <span className="text-[10px] font-semibold text-neutral-600">{letter}</span>
                    </div>
                  )}
                  {o.name}
                </button>
              );
            })}
            {GITHUB_APP_NAME && (
              <a
                href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm text-neutral-400 hover:text-neutral-600 hover:bg-neutral-50 transition-colors"
              >
                <svg className="h-4 w-4" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
                </svg>
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
                <button
                  onClick={syncAll}
                  disabled={syncing !== null}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 transition-colors disabled:opacity-50"
                >
                  <svg className={`h-3.5 w-3.5 ${syncing === "all" ? "animate-spin" : ""}`} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <path d="M13.5 8a5.5 5.5 0 1 1-1.3-3.5" strokeLinecap="round" />
                    <path d="M13.5 2v3.5H10" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                  {syncing === "all" ? "Syncing…" : "Sync all"}
                </button>
                {GITHUB_APP_NAME && (
                  <a
                    href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 transition-colors"
                  >
                    <svg className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor">
                      <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
                    </svg>
                    Add repositories
                  </a>
                )}
              </div>
            </div>
            <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">
              {loadingRepos ? (
                <LoadingRow />
              ) : repos.length === 0 ? (
                <EmptyRow message="No repositories connected.">
                  {GITHUB_APP_NAME && (
                    <a href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
                      className="mt-2 inline-flex items-center gap-1 text-xs text-neutral-500 hover:text-neutral-800 transition-colors">
                      Add via GitHub
                      <svg className="h-3 w-3" viewBox="0 0 16 16" fill="currentColor"><path d="M6.22 3.22a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.75.75 0 0 1-1.06-1.06L9.94 8 6.22 4.28a.75.75 0 0 1 0-1.06Z" /></svg>
                    </a>
                  )}
                </EmptyRow>
              ) : (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-neutral-100">
                      <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">Repository</th>
                      <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">Default branch</th>
                      <th className="px-5 py-3" />
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-neutral-100">
                    {repos.map((repo) => (
                      <tr key={repo.id}>
                        <td className="px-5 py-3 font-medium text-neutral-900">{repo.full_name}</td>
                        <td className="px-5 py-3 font-mono text-neutral-500 text-xs">{repo.default_branch}</td>
                        <td className="px-5 py-3 text-right">
                          <div className="flex items-center justify-end gap-3">
                            <button onClick={() => syncRepo(repo.id)} disabled={syncing !== null}
                              className="text-xs text-neutral-400 hover:text-neutral-700 transition-colors disabled:opacity-50">
                              {syncing === repo.id ? "Syncing…" : "Sync"}
                            </button>
                            <button onClick={() => removeRepo(repo)} disabled={removing === repo.id}
                              className="text-xs text-neutral-400 hover:text-red-500 transition-colors disabled:opacity-50">
                              {removing === repo.id ? "Removing…" : "Remove"}
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
            <p className="text-xs text-neutral-400 mt-2">
              Sync fetches open PRs and the last 30 days of closed PRs with reviews.
            </p>
          </section>

          {/* ── Teams — org accounts only ── */}
          {isOrgAccount && <section>
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-semibold text-neutral-900">Teams</h2>
              <button
                onClick={() => setShowAddTeam((v) => !v)}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 transition-colors"
              >
                <svg className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
                </svg>
                Add team
              </button>
            </div>

            <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">
              {loadingTeams ? (
                <LoadingRow />
              ) : teams.length === 0 && !showAddTeam ? (
                <div className="px-5 py-5">
                  <p className="text-sm text-neutral-500">Currently using the entire org as the team.</p>
                  <p className="text-xs text-neutral-400 mt-1">Add a team to scope the queue and flow metrics to a subset of members.</p>
                </div>
              ) : (
                <ul className="divide-y divide-neutral-100">
                  {teams.map((team) => (
                    <li key={team.id} className="flex items-center justify-between px-5 py-3">
                      <div className="flex items-center gap-2">
                        <div className="h-5 w-5 rounded bg-neutral-100 flex items-center justify-center">
                          <svg className="h-3 w-3 text-neutral-500" fill="none" stroke="currentColor" strokeWidth={1.75} viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" d="M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z" />
                          </svg>
                        </div>
                        <span className="text-sm font-medium text-neutral-900">{team.name}</span>
                      </div>
                      <button onClick={() => deleteTeam(team.id)}
                        className="text-xs text-neutral-400 hover:text-red-500 transition-colors">
                        Remove
                      </button>
                    </li>
                  ))}
                </ul>
              )}

              {/* Inline add-team form */}
              {showAddTeam && (
                <div className={`px-5 py-3 flex items-center gap-2 ${teams.length > 0 ? "border-t border-neutral-100" : ""}`}>
                  <input
                    ref={teamInputRef}
                    type="text"
                    value={newTeamName}
                    onChange={(e) => setNewTeamName(e.target.value)}
                    onKeyDown={(e) => { if (e.key === "Enter") addTeam(); if (e.key === "Escape") { setShowAddTeam(false); setNewTeamName(""); } }}
                    placeholder="Team name"
                    className="flex-1 text-sm px-3 py-1.5 rounded-lg border border-neutral-200 bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-neutral-300 placeholder:text-neutral-400"
                  />
                  <button onClick={addTeam} disabled={!newTeamName.trim() || addingTeam}
                    className="px-3 py-1.5 rounded-lg text-xs font-medium bg-neutral-900 text-white hover:bg-neutral-700 transition-colors disabled:opacity-40">
                    {addingTeam ? "Adding…" : "Add"}
                  </button>
                  <button onClick={() => { setShowAddTeam(false); setNewTeamName(""); }}
                    className="px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 text-neutral-600 hover:bg-neutral-50 transition-colors">
                    Cancel
                  </button>
                </div>
              )}
            </div>
          </section>}

          {/* ── Members — org accounts only ── */}
          {isOrgAccount && <section>
            <h2 className="text-sm font-semibold text-neutral-900 mb-3">Members</h2>
            <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">
              {loadingMembers ? (
                <LoadingRow />
              ) : members.length === 0 ? (
                <EmptyRow message="No members have signed up yet." />
              ) : (
                <ul className="divide-y divide-neutral-100">
                  {members.map((m) => {
                    const name = m.display_name ?? m.github_username ?? m.email;
                    const initials = name.slice(0, 2).toUpperCase();
                    return (
                      <li key={m.id} className="flex items-center gap-3 px-5 py-3">
                        {m.avatar_url ? (
                          <img src={m.avatar_url} alt={name} className="h-7 w-7 rounded-full bg-neutral-100 shrink-0" />
                        ) : (
                          <div className="h-7 w-7 rounded-full bg-neutral-200 flex items-center justify-center shrink-0">
                            <span className="text-xs font-semibold text-neutral-600">{initials}</span>
                          </div>
                        )}
                        <div className="flex-1 min-w-0">
                          <p className="text-sm font-medium text-neutral-900 truncate">{name}</p>
                          {m.github_username && (
                            <p className="text-xs text-neutral-400">@{m.github_username}</p>
                          )}
                        </div>
                        {m.role === "org_admin" ? (
                          <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-neutral-900 text-white">Admin</span>
                        ) : (
                          <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-neutral-100 text-neutral-500">Member</span>
                        )}
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
            <p className="text-xs text-neutral-400 mt-2">
              Only members who have signed into Aperture are listed.
            </p>
          </section>}

          {/* ── Account ── */}
          <section>
            <h2 className="text-sm font-semibold text-neutral-900 mb-3">Account</h2>
            <div className="rounded-xl border border-neutral-200 bg-white px-5 py-4 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-neutral-900">Delete account</p>
                <p className="text-xs text-neutral-400 mt-0.5">Permanently removes your account and all associated data.</p>
              </div>
              {confirmDelete ? (
                <div className="flex items-center gap-2">
                  <span className="text-xs text-neutral-500">Are you sure?</span>
                  <button onClick={() => setConfirmDelete(false)}
                    className="px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 text-neutral-600 hover:bg-neutral-50 transition-colors">
                    Cancel
                  </button>
                  <button onClick={deleteAccount} disabled={deletingAccount}
                    className="px-3 py-1.5 rounded-lg text-xs font-medium bg-red-600 text-white hover:bg-red-700 transition-colors disabled:opacity-50">
                    {deletingAccount ? "Deleting…" : "Delete"}
                  </button>
                </div>
              ) : (
                <button onClick={() => setConfirmDelete(true)}
                  className="px-3 py-1.5 rounded-lg text-xs font-medium border border-red-200 text-red-600 hover:bg-red-50 transition-colors">
                  Delete account
                </button>
              )}
            </div>
          </section>

        </div>
      </main>
    </div>
  );
}

function LoadingRow() {
  return (
    <div className="px-5 py-6 text-center">
      <p className="text-sm text-neutral-400">Loading…</p>
    </div>
  );
}

function EmptyRow({ message, children }: { message: string; children?: React.ReactNode }) {
  return (
    <div className="px-5 py-6 text-center">
      <p className="text-sm text-neutral-400">{message}</p>
      {children}
    </div>
  );
}
