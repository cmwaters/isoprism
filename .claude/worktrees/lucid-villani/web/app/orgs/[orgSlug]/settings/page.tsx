"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { AppHeader } from "@/components/layout/app-header";
import { Repository } from "@/lib/types";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export default function SettingsPage() {
  const params = useParams();
  const orgSlug = params.orgSlug as string;

  const [repos, setRepos] = useState<Repository[]>([]);
  const [loading, setLoading] = useState(true);
  const [toggling, setToggling] = useState<string | null>(null);
  const [token, setToken] = useState<string | null>(null);

  useEffect(() => {
    const init = async () => {
      const supabase = createClient();
      const { data: { session } } = await supabase.auth.getSession();
      if (!session) return;
      setToken(session.access_token);

      const res = await fetch(`${API_URL}/api/v1/orgs/${orgSlug}/repos`, {
        headers: { Authorization: `Bearer ${session.access_token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setRepos(data.repos ?? []);
      }
      setLoading(false);
    };
    init();
  }, [orgSlug]);

  const toggleRepo = useCallback(async (repo: Repository) => {
    if (!token || toggling) return;
    setToggling(repo.id);

    const newActive = !repo.is_active;
    setRepos((prev) =>
      prev.map((r) => (r.id === repo.id ? { ...r, is_active: newActive } : r))
    );

    const res = await fetch(
      `${API_URL}/api/v1/orgs/${orgSlug}/repos/${repo.id}`,
      {
        method: "PATCH",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ is_active: newActive }),
      }
    );

    if (!res.ok) {
      // Revert on failure
      setRepos((prev) =>
        prev.map((r) => (r.id === repo.id ? { ...r, is_active: repo.is_active } : r))
      );
    }
    setToggling(null);
  }, [token, toggling, orgSlug]);

  return (
    <div className="min-h-screen bg-neutral-50">
      <AppHeader orgSlug={orgSlug} activeTab="settings" />
      <main className="max-w-3xl mx-auto px-6 py-10">
        <div className="mb-8">
          <h1 className="text-xl font-semibold tracking-tight text-neutral-900">
            Settings
          </h1>
          <p className="text-sm text-neutral-500 mt-1">
            Manage repositories and preferences for {orgSlug}.
          </p>
        </div>

        {/* Repositories section */}
        <section>
          <h2 className="text-sm font-semibold text-neutral-700 mb-3 uppercase tracking-wide">
            Repositories
          </h2>
          <div className="rounded-xl border border-neutral-200 bg-white overflow-hidden">
            {loading ? (
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
                  <li
                    key={repo.id}
                    className="flex items-center justify-between px-5 py-4"
                  >
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-neutral-900 truncate">
                        {repo.full_name}
                      </p>
                      <p className="text-xs text-neutral-400 mt-0.5">
                        Default branch: <span className="font-mono">{repo.default_branch}</span>
                      </p>
                    </div>
                    <button
                      onClick={() => toggleRepo(repo)}
                      disabled={toggling === repo.id}
                      className={`relative ml-4 shrink-0 h-6 w-11 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-neutral-400 focus:ring-offset-2 ${
                        repo.is_active
                          ? "bg-neutral-900"
                          : "bg-neutral-200"
                      } ${toggling === repo.id ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}`}
                      role="switch"
                      aria-checked={repo.is_active}
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
            Disabled repositories are excluded from the queue and won't receive analysis.
          </p>
        </section>
      </main>
    </div>
  );
}
