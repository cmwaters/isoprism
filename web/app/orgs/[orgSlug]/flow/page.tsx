"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { FlowResponse, FlowPR, FlowReviewer } from "@/lib/types";
import { AppSidebar } from "@/components/layout/app-sidebar";

const PERIODS = [
  { label: "7d", value: 7 },
  { label: "14d", value: 14 },
  { label: "30d", value: 30 },
];

export default function FlowPage() {
  const params = useParams();
  const orgSlug = params.orgSlug as string;

  const [data, setData] = useState<FlowResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [period, setPeriod] = useState(7);
  const [tab, setTab] = useState<"prs" | "reviewers">("prs");

  const load = useCallback(async () => {
    setLoading(true);
    const supabase = createClient();
    const { data: { session } } = await supabase.auth.getSession();
    if (!session) return;
    try {
      const resp = await apiFetch<FlowResponse>(
        `/api/v1/orgs/${orgSlug}/flow?period=${period}`,
        session.access_token
      );
      setData(resp);
    } catch (e) {
      console.error("flow fetch failed", e);
    } finally {
      setLoading(false);
    }
  }, [orgSlug, period]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="h-screen flex bg-neutral-50">
      <AppSidebar orgSlug={orgSlug} activeTab="flow" />

      <main className="flex-1 overflow-y-auto">
        <div className="max-w-5xl mx-auto px-6 py-10">

          {/* Header row */}
          <div className="flex items-center justify-between mb-6">
            <div>
              <h1 className="text-base font-semibold text-neutral-900">Flow</h1>
              <p className="text-xs text-neutral-400 mt-0.5">Team throughput and review health</p>
            </div>

            {/* Period selector */}
            <div className="flex items-center gap-1 bg-neutral-100 rounded-lg p-1">
              {PERIODS.map((p) => (
                <button
                  key={p.value}
                  onClick={() => setPeriod(p.value)}
                  className={`px-3 py-1 rounded-md text-xs font-medium transition-colors ${
                    period === p.value
                      ? "bg-white text-neutral-900 shadow-sm"
                      : "text-neutral-500 hover:text-neutral-700"
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          {/* Tab switcher */}
          <div className="flex items-center gap-1 border-b border-neutral-200 mb-6">
            <TabButton active={tab === "prs"} onClick={() => setTab("prs")}>
              PRs {data && <span className="ml-1 text-neutral-400">({data.prs.length})</span>}
            </TabButton>
            <TabButton active={tab === "reviewers"} onClick={() => setTab("reviewers")}>
              Reviewers {data && <span className="ml-1 text-neutral-400">({data.reviewers.length})</span>}
            </TabButton>
          </div>

          {loading ? (
            <div className="flex items-center justify-center py-24">
              <div className="h-4 w-4 border-2 border-neutral-300 border-t-neutral-600 rounded-full animate-spin" />
            </div>
          ) : !data || data.prs.length === 0 ? (
            <EmptyState />
          ) : tab === "prs" ? (
            <PRsTable prs={data.prs} />
          ) : (
            <ReviewersTable reviewers={data.reviewers} />
          )}
        </div>
      </main>
    </div>
  );
}

// ── Tabs ──────────────────────────────────────────────────────────────────────

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px ${
        active
          ? "border-neutral-900 text-neutral-900"
          : "border-transparent text-neutral-500 hover:text-neutral-700"
      }`}
    >
      {children}
    </button>
  );
}

// ── PR table ──────────────────────────────────────────────────────────────────

function PRsTable({ prs }: { prs: FlowPR[] }) {
  return (
    <div className="bg-white rounded-xl border border-neutral-200 overflow-hidden">
      {/* Legend */}
      <div className="flex items-center gap-4 px-5 py-3 border-b border-neutral-100 bg-neutral-50">
        <div className="flex items-center gap-1.5">
          <div className="h-2.5 w-5 rounded-sm bg-blue-200" />
          <span className="text-xs text-neutral-500">Waiting on reviewer</span>
        </div>
        <div className="flex items-center gap-1.5">
          <div className="h-2.5 w-5 rounded-sm bg-amber-200" />
          <span className="text-xs text-neutral-500">Waiting on author</span>
        </div>
      </div>

      <div className="divide-y divide-neutral-100">
        {prs.map((pr) => (
          <PRRow key={pr.id} pr={pr} />
        ))}
      </div>
    </div>
  );
}

function PRRow({ pr }: { pr: FlowPR }) {
  const stateColor =
    pr.state === "merged" ? "bg-violet-100 text-violet-700" :
    pr.state === "closed" ? "bg-neutral-100 text-neutral-500" :
    "bg-green-100 text-green-700";

  return (
    <div className="px-5 py-4">
      {/* Top row: info + duration */}
      <div className="flex items-start gap-3 mb-3">
        <img
          src={pr.author_avatar_url ?? `https://github.com/${pr.author_github_login}.png`}
          alt={pr.author_github_login}
          className="h-5 w-5 rounded-full mt-0.5 shrink-0 bg-neutral-100"
        />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 min-w-0">
            <a
              href={pr.html_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm font-medium text-neutral-900 hover:text-neutral-600 truncate"
            >
              {pr.title}
            </a>
            <span className={`shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded ${stateColor}`}>
              {pr.state}
            </span>
          </div>
          <p className="text-[11px] text-neutral-400 mt-0.5">
            {pr.repo_full_name} · #{pr.number} · {pr.author_github_login}
          </p>
        </div>
        <div className="shrink-0 text-right">
          <p className="text-xs font-medium text-neutral-700">{formatDuration(pr.total_hours)}</p>
          <p className="text-[10px] text-neutral-400 mt-0.5">total</p>
        </div>
      </div>

      {/* Timeline bar */}
      <Timeline pr={pr} />

      {/* Stats row */}
      <div className="flex items-center gap-4 mt-2">
        <Stat label="reviewer" hours={pr.reviewer_hours} color="text-blue-600" />
        <Stat label="author" hours={pr.author_hours} color="text-amber-600" />
        <span className="text-[10px] text-neutral-400 ml-auto">
          {new Date(pr.opened_at).toLocaleDateString("en-GB", { day: "numeric", month: "short" })}
          {pr.closed_at && (
            <> → {new Date(pr.closed_at).toLocaleDateString("en-GB", { day: "numeric", month: "short" })}</>
          )}
        </span>
      </div>
    </div>
  );
}

function Timeline({ pr }: { pr: FlowPR }) {
  if (!pr.segments || pr.segments.length === 0 || pr.total_hours <= 0) {
    return <div className="h-3 rounded-full bg-neutral-100" />;
  }

  return (
    <div className="flex h-3 rounded-full overflow-hidden gap-px bg-neutral-100">
      {pr.segments.map((seg, i) => {
        const segHours = (new Date(seg.end).getTime() - new Date(seg.start).getTime()) / 3_600_000;
        const pct = Math.max((segHours / pr.total_hours) * 100, 0.5);
        const isReviewer = seg.kind === "reviewer";
        return (
          <div
            key={i}
            title={`${isReviewer ? "Reviewer" : "Author"}: ${formatDuration(segHours)}`}
            style={{ width: `${pct}%` }}
            className={`h-full ${isReviewer ? "bg-blue-300" : "bg-amber-300"}`}
          />
        );
      })}
    </div>
  );
}

function Stat({ label, hours, color }: { label: string; hours: number; color: string }) {
  return (
    <span className="text-[10px] text-neutral-500">
      <span className={`font-medium ${color}`}>{formatDuration(hours)}</span>
      {" "}{label}
    </span>
  );
}

// ── Reviewers table ───────────────────────────────────────────────────────────

function ReviewersTable({ reviewers }: { reviewers: FlowReviewer[] }) {
  if (reviewers.length === 0) {
    return <EmptyState />;
  }

  return (
    <div className="bg-white rounded-xl border border-neutral-200 overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-neutral-100 bg-neutral-50">
            <th className="px-5 py-3 text-left text-xs font-medium text-neutral-400">Reviewer</th>
            <th className="px-5 py-3 text-right text-xs font-medium text-neutral-400">Reviews</th>
            <th className="px-5 py-3 text-right text-xs font-medium text-neutral-400">Approvals</th>
            <th className="px-5 py-3 text-right text-xs font-medium text-neutral-400">Changes requested</th>
            <th className="px-5 py-3 text-right text-xs font-medium text-neutral-400">Authored PRs</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100">
          {reviewers.map((r) => (
            <tr key={r.login}>
              <td className="px-5 py-3">
                <div className="flex items-center gap-2.5">
                  <img
                    src={r.avatar_url ?? `https://github.com/${r.login}.png`}
                    alt={r.login}
                    className="h-6 w-6 rounded-full bg-neutral-100"
                  />
                  <span className="font-medium text-neutral-900">{r.login}</span>
                </div>
              </td>
              <td className="px-5 py-3 text-right text-neutral-700 font-medium">{r.reviews}</td>
              <td className="px-5 py-3 text-right">
                <span className="text-green-700 font-medium">{r.approvals}</span>
              </td>
              <td className="px-5 py-3 text-right">
                <span className="text-amber-700 font-medium">{r.changes_requested}</span>
              </td>
              <td className="px-5 py-3 text-right text-neutral-500">{r.authored_prs}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Shared ────────────────────────────────────────────────────────────────────

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <div className="h-10 w-10 rounded-full bg-neutral-100 flex items-center justify-center mb-4">
        <svg className="h-5 w-5 text-neutral-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 0 1 3 19.875v-6.75ZM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V8.625ZM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V4.125Z" />
        </svg>
      </div>
      <p className="text-sm font-medium text-neutral-600">No data yet</p>
      <p className="text-xs text-neutral-400 mt-1">PRs will appear here as they come in.</p>
    </div>
  );
}

function formatDuration(hours: number): string {
  if (hours < 1) return `${Math.round(hours * 60)}m`;
  if (hours < 24) return `${Math.round(hours)}h`;
  const days = Math.floor(hours / 24);
  const rem = Math.round(hours % 24);
  return rem > 0 ? `${days}d ${rem}h` : `${days}d`;
}
