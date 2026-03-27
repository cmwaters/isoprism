"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { Repository } from "@/lib/types";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const GITHUB_APP_NAME = process.env.NEXT_PUBLIC_GITHUB_APP_NAME;

export default function SettingsPage() {
  const params = useParams();
  const router = useRouter();
  const orgSlug = params.orgSlug as string;

  const [token, setToken] = useState<string | null>(null);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(true);
  const [syncing, setSyncing] = useState<string | null>(null); // repoID | "all"
  const [removing, setRemoving] = useState<string | null>(null);
  const [deletingAccount, setDeletingAccount] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

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
    setLoadingRepos(true);
    fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((r) => r.ok ? r.json() : { repos: [] })
      .then((d) => setRepos(d.repos ?? []))
      .catch(() => {})
      .finally(() => setLoadingRepos(false));
  }, [orgSlug, token]);

  const syncRepo = useCallback(async (repoID: string) => {
    if (!token || syncing) return;
    setSyncing(repoID);
    try {
      await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repoID}/sync`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
    } finally { setSyncing(null); }
  }, [token, syncing, orgSlug]);

  const syncAll = useCallback(async () => {
    if (!token || syncing) return;
    setSyncing("all");
    try {
      await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/sync`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
    } finally { setSyncing(null); }
  }, [token, syncing, orgSlug]);

  const removeRepo = useCallback(async (repo: Repository) => {
    if (!token || removing) return;
    setRemoving(repo.id);
    try {
      const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos/${repo.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) setRepos((prev) => prev.filter((r) => r.id !== repo.id));
    } finally { setRemoving(null); }
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
    } catch { setDeletingAccount(false); }
  }, [token, router]);

  return (
    <div className="h-screen flex bg-neutral-50">
      <AppSidebar orgSlug={orgSlug} activeTab="settings" />

      <main className="flex-1 overflow-y-auto">
        <div className="max-w-2xl mx-auto px-6 py-10 space-y-8">

          {/* Repositories */}
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
                <div className="px-5 py-8 text-center">
                  <p className="text-sm text-neutral-400">Loading…</p>
                </div>
              ) : repos.length === 0 ? (
                <div className="px-5 py-8 text-center">
                  <p className="text-sm text-neutral-400">No repositories connected.</p>
                  {GITHUB_APP_NAME && (
                    <a
                      href={`https://github.com/apps/${GITHUB_APP_NAME}/installations/new`}
                      className="inline-flex items-center gap-1 mt-3 text-xs text-neutral-500 hover:text-neutral-800 transition-colors"
                    >
                      Add repositories via GitHub
                      <svg className="h-3 w-3" viewBox="0 0 16 16" fill="currentColor">
                        <path d="M6.22 3.22a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.75.75 0 0 1-1.06-1.06L9.94 8 6.22 4.28a.75.75 0 0 1 0-1.06Z" />
                      </svg>
                    </a>
                  )}
                </div>
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
                            <button
                              onClick={() => syncRepo(repo.id)}
                              disabled={syncing !== null}
                              className="text-xs text-neutral-400 hover:text-neutral-700 transition-colors disabled:opacity-50"
                            >
                              {syncing === repo.id ? "Syncing…" : "Sync"}
                            </button>
                            <button
                              onClick={() => removeRepo(repo)}
                              disabled={removing === repo.id}
                              className="text-xs text-neutral-400 hover:text-red-500 transition-colors disabled:opacity-50"
                            >
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
              Per-repo sync fetches open PRs and backfills the last 30 days of closed PRs and reviews.
            </p>
          </section>

          {/* Account */}
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
      </main>
    </div>
  );
}
