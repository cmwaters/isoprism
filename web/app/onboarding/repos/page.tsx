"use client";

import { useEffect, useState, Suspense } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { Repository } from "@/lib/types";
import IndexingProgress from "@/components/onboarding/indexing-progress";

function ReposContent() {
  const router = useRouter();
  const supabase = createClient();

  const [repos, setRepos] = useState<Repository[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [indexing, setIndexing] = useState(false);
  const [indexingRepoID, setIndexingRepoID] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    async function load() {
      const { data: sessionData } = await supabase.auth.getSession();
      const token = sessionData.session?.access_token;
      if (!token) { router.push("/login"); return; }

      try {
        const { repos: repoList } = await apiFetch<{ repos: Repository[] }>("/api/v1/me/repos", token);
        setRepos(repoList ?? []);
      } catch {
        setError("Failed to load repositories.");
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  async function handleIndex() {
    if (!selected) return;
    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) return;

    setIndexing(true);
    setIndexingRepoID(selected);

    try {
      await apiFetch(`/api/v1/repos/${selected}/index`, token, { method: "POST" });
    } catch {
      // index call may already have started
    }
  }

  if (indexing && indexingRepoID) {
    return <IndexingProgress repoID={indexingRepoID} repoName={repos.find(r => r.id === indexingRepoID)?.full_name ?? ""} />;
  }

  const filtered = repos.filter((r) =>
    r.full_name.toLowerCase().includes(search.toLowerCase())
  );

  if (loading) {
    return (
      <div style={{ background: "#EBE9E9", minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center" }}>
        <div style={{ width: 24, height: 24, border: "2px solid #CCCCCC", borderTopColor: "#6366F1", borderRadius: "50%", animation: "spin 0.8s linear infinite" }} />
      </div>
    );
  }

  return (
    <div style={{ background: "#EBE9E9", minHeight: "100vh", display: "flex" }}>
      {/* Sidebar */}
      <div style={{ width: 240, background: "#E1E1E1", borderRight: "1px solid #D4D4D4", padding: 20, display: "flex", flexDirection: "column" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <GraphLogo />
          <span style={{ color: "#111111", fontSize: 15, fontWeight: 600 }}>Isoprism</span>
        </div>
      </div>

      {/* Main content */}
      <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", padding: 48 }}>
        <div style={{ maxWidth: 560, width: "100%" }}>
          <h1 style={{ color: "#111111", fontSize: 24, fontWeight: 600, margin: 0 }}>Select a repository</h1>
          <p style={{ color: "#666666", fontSize: 15, marginTop: 8, marginBottom: 24 }}>
            Isoprism will index this repository&apos;s pull requests.
          </p>

          {error && <p style={{ color: "#EF4444", fontSize: 14, marginBottom: 16 }}>{error}</p>}

          {/* Search */}
          <div style={{ position: "relative", marginBottom: 16 }}>
            <SearchIcon />
            <input
              type="text"
              placeholder="Search repositories…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{
                width: "100%",
                height: 44,
                background: "#FFFFFF",
                border: "1px solid #D4D4D4",
                borderRadius: 6,
                color: "#111111",
                fontSize: 14,
                paddingLeft: 36,
                paddingRight: 12,
                boxSizing: "border-box",
                outline: "none",
              }}
            />
          </div>

          {/* Repo list */}
          <div style={{ maxHeight: 400, overflowY: "auto", display: "flex", flexDirection: "column", gap: 1 }}>
            {filtered.length === 0 ? (
              <p style={{ color: "#888888", fontSize: 14, textAlign: "center", padding: "32px 0" }}>
                {repos.length === 0 ? "No repositories found. Make sure the GitHub App is installed on your account." : "No results match your search."}
              </p>
            ) : (
              filtered.map((repo) => {
                const isSelected = selected === repo.id;
                const [owner, name] = repo.full_name.split("/");
                return (
                  <button
                    key={repo.id}
                    onClick={() => setSelected(repo.id)}
                    style={{
                      height: 56,
                      background: isSelected ? "#D8D8D8" : "#FFFFFF",
                      border: "1px solid #D4D4D4",
                      borderLeft: isSelected ? "3px solid #6366F1" : "1px solid #D4D4D4",
                      borderRadius: 6,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      padding: "0 16px",
                      cursor: "pointer",
                      textAlign: "left",
                    }}
                    onMouseEnter={(e) => { if (!isSelected) (e.currentTarget as HTMLButtonElement).style.background = "#F0F0F0"; }}
                    onMouseLeave={(e) => { if (!isSelected) (e.currentTarget as HTMLButtonElement).style.background = "#FFFFFF"; }}
                  >
                    <div>
                      <span style={{ color: "#888888", fontSize: 13 }}>{owner}/</span>
                      <span style={{ color: isSelected ? "#6366F1" : "#111111", fontSize: 14, fontWeight: 600 }}>{name}</span>
                    </div>
                    <span style={{ background: "#EBE9E9", border: "1px solid #D4D4D4", borderRadius: 4, padding: "2px 6px", fontSize: 11, color: "#888888" }}>
                      {repo.default_branch}
                    </span>
                  </button>
                );
              })
            )}
          </div>

          {/* Continue button */}
          <div style={{ marginTop: 24, display: "flex", justifyContent: "flex-end" }}>
            <button
              onClick={handleIndex}
              disabled={!selected}
              style={{
                width: 180,
                height: 44,
                background: "#6366F1",
                color: "white",
                border: "none",
                borderRadius: 6,
                fontSize: 14,
                fontWeight: 500,
                cursor: selected ? "pointer" : "not-allowed",
                opacity: selected ? 1 : 0.4,
                transition: "opacity 150ms",
              }}
            >
              Index repository →
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function SearchIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#AAAAAA" strokeWidth="2" style={{ position: "absolute", left: 12, top: "50%", transform: "translateY(-50%)" }}>
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  );
}

function GraphLogo() {
  return (
    <svg width="24" height="24" viewBox="0 0 32 32" fill="none">
      <circle cx="8" cy="16" r="4" fill="#111111" />
      <circle cx="24" cy="8" r="4" fill="#111111" />
      <circle cx="24" cy="24" r="4" fill="#111111" />
      <line x1="12" y1="14" x2="20" y2="10" stroke="#111111" strokeWidth="1.5" />
      <line x1="12" y1="18" x2="20" y2="22" stroke="#111111" strokeWidth="1.5" />
    </svg>
  );
}

export default function ReposPage() {
  return (
    <Suspense>
      <ReposContent />
    </Suspense>
  );
}
