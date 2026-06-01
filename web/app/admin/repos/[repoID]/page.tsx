"use client";

import Link from "next/link";
import type React from "react";
import { useParams, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import GraphCanvas from "@/components/graph/graph-canvas";
import { API_URL } from "@/lib/api";
import type { QueueResponse, RepoGraphResponse, RepoProgramsResponse, Repository } from "@/lib/types";

// PilotUser describes user data used by pilot administration.
type PilotUser = {
  id: string;
  name: string;
  user_id?: string | null;
  selected_repo_id?: string | null;
  selected_repo_full_name?: string | null;
};

const PASSWORD_STORAGE_KEY = "isoprism_admin_password";

// AdminRepoPage renders the admin repo page for pilot administration.
export default function AdminRepoPage() {
  const params = useParams<{ repoID: string }>();
  const searchParams = useSearchParams();
  const repoID = params.repoID;
  const userID = searchParams.get("user") ?? "";
  const token = useMemo(() => adminViewToken(userID), [userID]);
  const [repo, setRepo] = useState<Repository | null>(null);
  const [graph, setGraph] = useState<RepoGraphResponse | null>(null);
  const [prs, setPRs] = useState<QueueResponse["prs"]>([]);
  const [message, setMessage] = useState("Loading repo...");
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    // load loads data for the enclosing route or component.
    async function load() {
      const password = window.localStorage.getItem(PASSWORD_STORAGE_KEY) ?? "";
      if (!password) {
        setError("Unlock /admin first, then open a pilot repo from the admin page.");
        setMessage("");
        return;
      }
      if (!repoID || !userID) {
        setError("This admin repo link is missing the pilot user context.");
        setMessage("");
        return;
      }

      try {
        const usersResponse = await fetch(`${API_URL}/api/v1/admin/pilot/users`, {
          headers: { "X-Admin-Password": password },
        });
        if (!usersResponse.ok) throw new Error(await usersResponse.text());
        const { testers } = await usersResponse.json() as { testers: PilotUser[] };
        const user = testers.find((tester) => tester.user_id === userID && tester.selected_repo_id === repoID);
        if (!user) throw new Error("This repo is not attached to a pilot user visible to the admin console.");

        const [repoResponse, queueResponse, programsResponse] = await Promise.all([
          fetch(`${API_URL}/api/v1/repos/${encodeURIComponent(repoID)}`, {
            headers: { Authorization: `Bearer ${token}` },
          }),
          fetch(`${API_URL}/api/v1/repos/${encodeURIComponent(repoID)}/queue`, {
            headers: { Authorization: `Bearer ${token}` },
          }),
          fetch(`${API_URL}/api/v1/repos/${encodeURIComponent(repoID)}/programs`, {
            headers: { Authorization: `Bearer ${token}` },
          }),
        ]);
        if (!repoResponse.ok) throw new Error(await repoResponse.text());

        const repoResult = await repoResponse.json() as Repository;
        const queueResult = queueResponse.ok ? await queueResponse.json() as QueueResponse : { prs: [] };
        const programsResult = programsResponse.ok ? await programsResponse.json() as RepoProgramsResponse : { repo: repoResult, programs: [] };

        if (!cancelled) {
          setRepo(repoResult);
          setGraph({ repo: repoResult, programs: programsResult.programs ?? [], nodes: [], edges: [] });
          setPRs(queueResult.prs ?? []);
          setMessage("");
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Could not open the pilot repo.");
          setMessage("");
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [repoID, token, userID]);

  if (error || message || !repo || !graph) {
    return (
      <main style={statePageStyle}>
        <div style={statePanelStyle}>
          <Link href="/admin" style={linkStyle}>Back to admin</Link>
          <h1 style={titleStyle}>Pilot repo</h1>
          <p style={error ? errorTextStyle : copyStyle}>{error || message}</p>
        </div>
      </main>
    );
  }

  return <GraphCanvas graph={graph} prs={prs} repoID={repo.id} repo={repo} token={token} />;
}

// adminViewToken builds an unsigned admin-view token for viewing a pilot repo.
function adminViewToken(userID: string) {
  const header = base64URL(JSON.stringify({ alg: "none", typ: "JWT" }));
  const payload = base64URL(JSON.stringify({ sub: userID }));
  return `${header}.${payload}.`;
}

// base64URL encodes a string with base64url characters.
function base64URL(value: string) {
  return btoa(value).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

const statePageStyle: React.CSSProperties = { minHeight: "100vh", background: "#EBE9E9", color: "#111", display: "grid", placeItems: "center", padding: 24 };
const statePanelStyle: React.CSSProperties = { width: "min(460px, 100%)", border: "1px solid #D4D4D4", borderRadius: 8, background: "#FFF", padding: 20 };
const titleStyle: React.CSSProperties = { margin: "14px 0 8px", fontSize: 24, lineHeight: 1.2 };
const copyStyle: React.CSSProperties = { margin: 0, color: "#666", fontSize: 14, lineHeight: 1.5 };
const errorTextStyle: React.CSSProperties = { ...copyStyle, color: "#991B1B" };
const linkStyle: React.CSSProperties = { color: "#111", textDecoration: "underline", fontSize: 13, fontWeight: 700 };
