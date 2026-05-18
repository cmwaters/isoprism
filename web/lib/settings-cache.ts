import { apiFetch } from "@/lib/api";
import type { Repository } from "@/lib/types";

type SettingsReposCacheEntry = {
  repos: Repository[];
  fetchedAt: number;
};

const CACHE_TTL_MS = 30_000;

let cachedRepos: SettingsReposCacheEntry | null = null;
let pendingRepos: Promise<Repository[]> | null = null;
let pendingToken = "";

export function getCachedSettingsRepos() {
  if (!cachedRepos) return null;
  if (Date.now() - cachedRepos.fetchedAt > CACHE_TTL_MS) return null;
  return cachedRepos.repos;
}

export function warmSettingsRepos(token: string) {
  if (!token) return Promise.resolve(null);

  if (pendingRepos && pendingToken === token) {
    return pendingRepos.then((repos) => repos).catch(() => null);
  }

  pendingToken = token;
  pendingRepos = apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token)
    .then(({ repos }) => {
      const nextRepos = repos ?? [];
      cachedRepos = { repos: nextRepos, fetchedAt: Date.now() };
      return nextRepos;
    })
    .finally(() => {
      pendingRepos = null;
      pendingToken = "";
    });

  return pendingRepos.catch(() => null);
}
