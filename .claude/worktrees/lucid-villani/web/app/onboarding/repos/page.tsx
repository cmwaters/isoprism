"use client";

import { useEffect, useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { Repository } from "@/lib/types";
import { Button } from "@/components/ui/button";

function ReposContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const supabase = createClient();

  const [repos, setRepos] = useState<Repository[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [syncing, setSyncing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const orgSlug = searchParams.get("org");

  useEffect(() => {
    async function load() {
      if (!orgSlug) {
        router.push("/onboarding");
        return;
      }

      const { data: sessionData } = await supabase.auth.getSession();
      const token = sessionData.session?.access_token;
      if (!token) {
        router.push("/login");
        return;
      }

      try {
        const { repos: repoList } = await apiFetch<{ repos: Repository[] }>(
          `/api/v1/orgs/${orgSlug}/repos`,
          token
        );
        setRepos(repoList);
        // Pre-select all repos by default
        setSelected(new Set(repoList.map((r) => r.id)));
      } catch {
        setError("Failed to load repositories. Please try again.");
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [orgSlug]);

  function toggleRepo(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function handleConfirm() {
    if (!orgSlug) return;
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    setSyncing(true);

    // Toggle repos on/off
    for (const repo of repos) {
      const isActive = selected.has(repo.id);
      if (isActive !== repo.is_active) {
        await apiFetch(`/api/v1/orgs/${orgSlug}/repos/${repo.id}`, token, {
          method: "PATCH",
          body: JSON.stringify({ is_active: isActive }),
        }).catch(() => null);
      }
    }

    // Sync open PRs for selected repos
    for (const repoId of selected) {
      await apiFetch(`/api/v1/orgs/${orgSlug}/repos/${repoId}/sync`, token, {
        method: "POST",
      }).catch(() => null);
    }

    router.push(`/orgs/${orgSlug}`);
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-neutral-50">
        <span className="h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-neutral-900" />
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-neutral-50">
      <div className="w-full max-w-md px-6">
        <div className="mb-12 flex items-center gap-2">
          <div className="h-6 w-6 rounded-full bg-neutral-900" />
          <span className="text-lg font-semibold tracking-tight">Aperture</span>
        </div>

        <div className="mb-8">
          <h1 className="text-2xl font-semibold tracking-tight text-neutral-900 mb-2">
            Select repositories
          </h1>
          <p className="text-sm text-neutral-500">
            Choose which repos Aperture should track for <span className="font-medium">{orgSlug}</span>. All are selected by default.
          </p>
        </div>

        {error && <p className="text-sm text-red-500 mb-4">{error}</p>}

        {repos.length === 0 ? (
          <p className="text-sm text-neutral-500 py-8 text-center">
            No repositories found. Make sure you granted access to at least one repo during installation.
          </p>
        ) : (
          <div className="space-y-2 mb-6">
            {repos.map((repo) => (
              <button
                key={repo.id}
                onClick={() => toggleRepo(repo.id)}
                className={`w-full flex items-center justify-between px-4 py-3 rounded-lg border text-left transition-colors ${
                  selected.has(repo.id)
                    ? "border-neutral-900 bg-neutral-900 text-white"
                    : "border-neutral-200 bg-white text-neutral-700 hover:border-neutral-300"
                }`}
              >
                <span className="text-sm font-medium">{repo.full_name}</span>
                {selected.has(repo.id) && (
                  <svg className="h-4 w-4 shrink-0" fill="none" stroke="currentColor" strokeWidth={2.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                  </svg>
                )}
              </button>
            ))}
          </div>
        )}

        <div className="flex items-center justify-between">
          <span className="text-xs text-neutral-400">
            {selected.size} of {repos.length} selected
          </span>
          <Button
            onClick={handleConfirm}
            disabled={selected.size === 0 || syncing}
            className="h-10 bg-neutral-900 hover:bg-neutral-700 text-white rounded-lg text-sm font-medium px-5 transition-colors disabled:opacity-40"
          >
            {syncing ? (
              <span className="flex items-center gap-2">
                <span className="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
                Setting up…
              </span>
            ) : (
              "Open Aperture"
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}

export default function ReposPage() {
  return (
    <Suspense>
      <ReposContent />
    </Suspense>
  );
}
