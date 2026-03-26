"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { AppHeader } from "@/components/layout/app-header";
import { Organization, Repository } from "@/lib/types";

const GITHUB_APP_NAME_ENV = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const GITHUB_APP_NAME = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;

export default function SettingsPage() {
  const params = useParams();
  const router = useRouter();
  const orgSlug = params.orgSlug as string;

  const [token, setToken] = useState<string | null>(null);
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(true);
  const [toggling, setToggling] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [deletingAccount, setDeletingAccount] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const currentOrg = orgs.find((o) => o.slug === orgSlug);
  const currentAccountType = currentOrg?.github_account_type ?? null;

  // Load token + all orgs
  useEffect(() => {
    async function init() {
      const supabase = createClient();
      const { data: { session } } = await supabase.auth.getSession();
      if (!session) return;
      setToken(session.access_token);
      try {
        const res = await fetch(`${API_URL}/api/v1/me/orgs`, {
          headers: { Authorization: `Bearer ${session.access_token}` },
        });
        if (res.ok) {
          const data = await res.json();
          setOrgs(data.orgs ?? []);
        }
      } catch {}
    }
    init();
  }, []);

  // Load repos for current org
  useEffect(() => {
    if (!token) return;
    setLoadingRepos(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => res.ok ? res.json() : { repos: [] })
      .then((data) => setRepos(data.repos ?? []))
      .catch(() => {})
      .finally(() => setLoadingRepos(false));
  }, [orgSlug, token]);

  const removeRepo = useCallback(async (repo: Repository) => {
    if (!token || removing) return;
    setRemoving(repo.id);
    try {
      const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repo.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) setRepos((prev) => prev.filter((r) => r.id !== repo.id));
    } catch {}
    setRemoving(null);
  }, [token, removing, orgSlug]);

  const deleteAccount = useCallback(async () => {
    if (!token) return;
    setDeletingAccount(true);
    try {
      await fetch(`${API_URL}/api/v1/me`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      const supabase = createClient();
      await supabase.auth.signOut();
      router.push("/login");
    } catch {
      setDeletingAccount(false);
    }
  }, [token, router]);

  const toggleRepo = useCallback(async (repo: Repository) => {
    if (!token || toggling) return;
    setToggling(repo.id);
    const newActive = !repo.is_active;
    setRepos((prev) => prev.map((r) => (r.id === repo.id ? { ...r, is_active: newActive } : r)));
    try {
      const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repo.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ is_active: newActive }),
      });
      if (!res.ok) throw new Error();
    } catch {
      setRepos((prev) => prev.map((r) => (r.id === repo.id ? { ...r, is_active: repo.is_active } : r)));
    }
    setToggling(null);
  }, [token, toggling, orgSlug]);

  return (
    <div className="min-h-screen bg-neutral-50">
      <AppHeader orgSlug={orgSlug} />

      {/* Org switcher bar */}
      <div className="max-w-3xl mx-auto px-6 pt-8 pb-2">
        <div className="flex items-center gap-1 flex-wrap">
          {orgs.map((org) => {
            const isActive = org.slug === orgSlug;
            const letter = org.name[0]?.toUpperCase() ?? "?";
            const isUser = org.github_account_type === "User";
            return (
              <button
                key={org.id}
                onClick={() => router.push(`/orgs/${org.slug}/settings`)}
                className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-white shadow-sm border border-neutral-200 text-neutral-900"
                    : "text-neutral-500 hover:text-neutral-700 hover:bg-neutral-100"
                }`}
              >
                {org.avatar_url ? (
                  <div className="relative">
                    <img src={org.avatar_url} alt={org.name} className="h-5 w-5 rounded-full object-cover" />
                    {isUser && (
                      <span className="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full bg-neutral-400 border border-neutral-50 flex items-center justify-center">
                        <svg className="h-1.5 w-1.5 text-white" viewBox="0 0 16 16" fill="currentColor">
                          <path d="M8 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm-5 6a5 5 0 0 1 10 0H3Z" />
                        </svg>
                      </span>
                    )}
                  </div>
                ) : (
                  <div className="h-5 w-5 rounded-full bg-neutral-200 flex items-center justify-center">
                    <span className="text-[10px] font-semibold text-neutral-600">{letter}</span>
                  </div>
                )}
                {org.name}
              </button>
            );
          })}
          {GITHUB_APP_NAME && (
            <a
              href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
              className="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm text-neutral-400 hover:text-neutral-600 hover:bg-neutral-100 transition-colors"
            >
              <svg className="h-4 w-4" viewBox="0 0 16 16" fill="currentColor">
                <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
              </svg>
              Add org
            </a>
          )}
        </div>
      </div>

      <main className="max-w-3xl mx-auto px-6 py-6">
        {currentAccountType === "User" ? (
          /* User settings */
          <div className="space-y-6">
            {/* Repositories table */}
            <section>
              <div className="flex items-center justify-between mb-3">
                <h2 className="text-sm font-semibold text-neutral-500 uppercase tracking-wide">Repositories</h2>
                {GITHUB_APP_NAME_ENV && (
                  <a
                    href={`https://github.com/apps/${GITHUB_APP_NAME_ENV}/installations/new`}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 transition-colors"
                  >
                    <svg className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor">
                      <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
                    </svg>
                    Add repositories
                  </a>
                )}
              </div>
              <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">
                {loadingRepos ? (
                  <div className="px-5 py-8 text-center">
                    <p className="text-sm text-neutral-400">Loading repositories…</p>
                  </div>
                ) : repos.length === 0 ? (
                  <div className="px-5 py-8 text-center">
                    <p className="text-sm text-neutral-400">No repositories connected.</p>
                  </div>
                ) : (
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-neutral-100">
                        <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">Repository</th>
                        <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">Default branch</th>
                        <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">Tracking</th>
                        <th className="px-5 py-3" />
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-neutral-100">
                      {repos.map((repo) => (
                        <tr key={repo.id}>
                          <td className="px-5 py-3 font-medium text-neutral-900">{repo.full_name}</td>
                          <td className="px-5 py-3 font-mono text-neutral-500 text-xs">{repo.default_branch}</td>
                          <td className="px-5 py-3">
                            <button
                              onClick={() => toggleRepo(repo)}
                              disabled={toggling === repo.id}
                              role="switch"
                              aria-checked={repo.is_active}
                              className={`relative shrink-0 h-5 w-9 rounded-full transition-colors focus:outline-none ${
                                repo.is_active ? "bg-neutral-900" : "bg-neutral-200"
                              } ${toggling === repo.id ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}`}
                            >
                              <span className={`absolute top-0.5 left-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${repo.is_active ? "translate-x-4" : "translate-x-0"}`} />
                            </button>
                          </td>
                          <td className="px-5 py-3 text-right">
                            <button
                              onClick={() => removeRepo(repo)}
                              disabled={removing === repo.id}
                              className="text-xs text-neutral-400 hover:text-red-500 transition-colors disabled:opacity-50"
                            >
                              Remove
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </section>

            {/* Delete account */}
            <section>
              <h2 className="text-sm font-semibold text-neutral-500 mb-3 uppercase tracking-wide">Account</h2>
              <div className="rounded-xl border border-neutral-200 bg-white px-5 py-4 flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium text-neutral-900">Delete account</p>
                  <p className="text-xs text-neutral-400 mt-0.5">
                    Permanently removes your account and all associated data.
                  </p>
                </div>
                {confirmDelete ? (
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-neutral-500">Are you sure?</span>
                    <button
                      onClick={() => setConfirmDelete(false)}
                      className="px-3 py-1.5 rounded-lg text-xs font-medium border border-neutral-200 text-neutral-600 hover:bg-neutral-50 transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={deleteAccount}
                      disabled={deletingAccount}
                      className="px-3 py-1.5 rounded-lg text-xs font-medium bg-red-600 text-white hover:bg-red-700 transition-colors disabled:opacity-50"
                    >
                      {deletingAccount ? "Deleting…" : "Delete"}
                    </button>
                  </div>
                ) : (
                  <button
                    onClick={() => setConfirmDelete(true)}
                    className="px-3 py-1.5 rounded-lg text-xs font-medium border border-red-200 text-red-600 hover:bg-red-50 transition-colors"
                  >
                    Delete account
                  </button>
                )}
              </div>
            </section>
          </div>
        ) : (
          /* Org settings — repositories */
          <section>
            <h2 className="text-sm font-semibold text-neutral-500 mb-3 uppercase tracking-wide">
              Repositories
            </h2>
            <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">
              {loadingRepos ? (
                <div className="px-5 py-8 text-center">
                  <p className="text-sm text-neutral-400">Loading repositories…</p>
                </div>
              ) : repos.length === 0 ? (
                <div className="px-5 py-8 text-center">
                  <p className="text-sm text-neutral-400">No repositories connected.</p>
                </div>
              ) : (
                <ul className="divide-y divide-neutral-100">
                  {repos.map((repo) => (
                    <li key={repo.id} className="flex items-center justify-between px-5 py-4">
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-neutral-900 truncate">{repo.full_name}</p>
                        <p className="text-xs text-neutral-400 mt-0.5">
                          Default branch: <span className="font-mono">{repo.default_branch}</span>
                        </p>
                      </div>
                      <button
                        onClick={() => toggleRepo(repo)}
                        disabled={toggling === repo.id}
                        role="switch"
                        aria-checked={repo.is_active}
                        className={`relative ml-4 shrink-0 h-6 w-11 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-neutral-400 focus:ring-offset-2 ${
                          repo.is_active ? "bg-neutral-900" : "bg-neutral-200"
                        } ${toggling === repo.id ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}`}
                      >
                        <span
                          className={`absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${
                            repo.is_active ? "translate-x-5" : "translate-x-0"
                          }`}
                        />
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <p className="text-xs text-neutral-400 mt-2">
              Disabled repositories are excluded from the queue and won&apos;t receive analysis.
            </p>
          </section>
        )}
      </main>
    </div>
  );
}
