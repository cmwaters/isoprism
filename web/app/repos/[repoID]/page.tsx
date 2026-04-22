import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { QueueResponse, Repository } from "@/lib/types";
import PRQueue from "@/components/queue/pr-queue";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ repoID: string }>;
}

export default async function RepoQueuePage({ params }: Props) {
  const { repoID } = await params;
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  let repo: Repository | null = null;
  let queue: QueueResponse = { prs: [] };

  try {
    repo = await apiFetch<Repository>(`/api/v1/repos/${repoID}`, token);
  } catch {
    redirect("/");
  }

  try {
    queue = await apiFetch<QueueResponse>(`/api/v1/repos/${repoID}/queue`, token);
  } catch {
    // queue may be empty if indexing just finished
  }

  return (
    <div style={{ background: "#0A0A0A", minHeight: "100vh", display: "flex" }}>
      {/* Sidebar */}
      <div style={{ width: 240, background: "#111111", borderRight: "1px solid #242424", padding: 20, display: "flex", flexDirection: "column" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 32 }}>
          <GraphLogo />
          <span style={{ color: "#F0F0F0", fontSize: 15, fontWeight: 600 }}>Isoprism</span>
        </div>
        <nav>
          <a
            href={`/repos/${repoID}`}
            style={{ display: "block", color: "#F0F0F0", fontSize: 14, fontWeight: 500, padding: "8px 12px", borderRadius: 6, background: "#1A1A1A", textDecoration: "none", marginBottom: 4 }}
          >
            Queue
          </a>
        </nav>
      </div>

      {/* Main */}
      <div style={{ flex: 1, padding: "48px 48px 48px 48px" }}>
        <div style={{ maxWidth: 720 }}>
          <p style={{ color: "#555555", fontSize: 13, marginBottom: 8 }}>{repo?.full_name}</p>
          <h1 style={{ color: "#F0F0F0", fontSize: 22, fontWeight: 600, marginBottom: 8 }}>Pull requests</h1>
          <p style={{ color: "#888888", fontSize: 14, marginBottom: 24 }}>
            Top {queue.prs.length > 0 ? queue.prs.length : 5} open PRs ranked by wait time and impact.
          </p>

          <PRQueue prs={queue.prs} repoID={repoID} />

          {queue.prs.length === 0 && (
            <div style={{ color: "#555555", fontSize: 14, textAlign: "center", padding: "64px 0" }}>
              No pull requests with graph data yet. Open a PR on GitHub and it will appear here once analysed.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function GraphLogo() {
  return (
    <svg width="24" height="24" viewBox="0 0 32 32" fill="none">
      <circle cx="8" cy="16" r="4" fill="white" />
      <circle cx="24" cy="8" r="4" fill="white" />
      <circle cx="24" cy="24" r="4" fill="white" />
      <line x1="12" y1="14" x2="20" y2="10" stroke="white" strokeWidth="1.5" />
      <line x1="12" y1="18" x2="20" y2="22" stroke="white" strokeWidth="1.5" />
    </svg>
  );
}
