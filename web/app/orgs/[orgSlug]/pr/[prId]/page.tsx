import { redirect } from "next/navigation";
import Link from "next/link";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { PullRequest } from "@/lib/types";
import { AppHeader } from "@/components/layout/app-header";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ orgSlug: string; prId: string }>;
}

export default async function PRPage({ params }: Props) {
  const { orgSlug, prId } = await params;

  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  let pr: PullRequest | null = null;
  try {
    pr = await apiFetch<PullRequest>(`/api/v1/orgs/${orgSlug}/prs/${prId}`, token);
  } catch {
    redirect(`/orgs/${orgSlug}`);
  }

  if (!pr) redirect(`/orgs/${orgSlug}`);

  const sizeLabel = pr.analysis?.size_label ?? computeSize(pr.additions + pr.deletions);
  const openedDate = new Date(pr.opened_at).toLocaleDateString("en-GB", {
    day: "numeric", month: "short", year: "numeric",
  });

  return (
    <div className="min-h-screen bg-neutral-50">
      <AppHeader orgSlug={orgSlug} activeTab="queue" />
      <main className="max-w-3xl mx-auto px-6 py-10">

        {/* Back */}
        <Link
          href={`/orgs/${orgSlug}`}
          className="inline-flex items-center gap-1.5 text-sm text-neutral-400 hover:text-neutral-600 transition-colors mb-6"
        >
          <svg className="h-4 w-4" viewBox="0 0 16 16" fill="currentColor">
            <path d="M9.78 3.22a.75.75 0 0 1 0 1.06L6.56 7.5l3.22 3.22a.75.75 0 1 1-1.06 1.06L4.94 8.06a.75.75 0 0 1 0-1.06l3.78-3.78a.75.75 0 0 1 1.06 0Z" />
          </svg>
          Back to queue
        </Link>

        {/* Header card */}
        <div className="bg-white rounded-xl border border-neutral-200 p-6 mb-4">
          <div className="flex items-start gap-4">
            <Avatar className="h-9 w-9 mt-0.5 shrink-0">
              <AvatarImage src={pr.author_avatar_url} alt={pr.author_github_login} />
              <AvatarFallback className="text-xs bg-neutral-100 text-neutral-600">
                {pr.author_github_login.slice(0, 2).toUpperCase()}
              </AvatarFallback>
            </Avatar>
            <div className="flex-1 min-w-0">
              <h1 className="text-lg font-semibold text-neutral-900 leading-snug mb-1">
                {pr.title}
              </h1>
              <p className="text-sm text-neutral-400">
                <span className="font-medium text-neutral-500">{pr.repo_full_name}</span>
                {" · "}#{pr.number}
                {" · "}opened by{" "}
                <span className="font-medium text-neutral-600">{pr.author_github_login}</span>
                {" on "}{openedDate}
              </p>
              <div className="flex items-center gap-1.5 flex-wrap mt-3">
                {pr.draft && <StateBadge label="Draft" className="bg-neutral-50 text-neutral-500 border-neutral-200" />}
                <SizeBadge size={sizeLabel} />
                {pr.analysis?.risk_label && <RiskBadge risk={pr.analysis.risk_label} />}
                {pr.analysis?.impacted_areas?.map((area) => (
                  <Badge key={area} variant="outline" className="text-xs font-normal border-neutral-200 text-neutral-500 py-0 px-2">
                    {area}
                  </Badge>
                ))}
              </div>
            </div>
            <a
              href={pr.html_url}
              target="_blank"
              rel="noopener noreferrer"
              className="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-neutral-900 text-white hover:bg-neutral-700 transition-colors"
            >
              <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
              </svg>
              View on GitHub
            </a>
          </div>
        </div>

        {/* Stats row */}
        <div className="grid grid-cols-3 gap-3 mb-4">
          <StatCard label="Additions" value={`+${pr.additions}`} valueClass="text-green-600" />
          <StatCard label="Deletions" value={`-${pr.deletions}`} valueClass="text-red-500" />
          <StatCard label="Files changed" value={String(pr.changed_files)} valueClass="text-neutral-900" />
        </div>

        {/* Branch info */}
        <div className="bg-white rounded-xl border border-neutral-200 px-5 py-4 mb-4 flex items-center gap-2 text-sm text-neutral-500 font-mono">
          <span className="text-neutral-900">{pr.head_branch}</span>
          <svg className="h-4 w-4 text-neutral-300" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8.22 2.97a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.75.75 0 0 1-1.06-1.06l2.97-2.97H3.75a.75.75 0 0 1 0-1.5h7.44L8.22 4.03a.75.75 0 0 1 0-1.06Z" />
          </svg>
          <span>{pr.base_branch}</span>
        </div>

        {/* AI Analysis */}
        {pr.analysis && (
          <div className="bg-white rounded-xl border border-neutral-200 p-6 mb-4 space-y-4">
            <h2 className="text-sm font-semibold text-neutral-700">Analysis</h2>

            {pr.analysis.summary && (
              <div>
                <p className="text-xs font-medium text-neutral-400 uppercase tracking-wide mb-1">Summary</p>
                <p className="text-sm text-neutral-700 leading-relaxed">{pr.analysis.summary}</p>
              </div>
            )}

            {pr.analysis.why && (
              <div>
                <p className="text-xs font-medium text-neutral-400 uppercase tracking-wide mb-1">Why</p>
                <p className="text-sm text-neutral-700 leading-relaxed">{pr.analysis.why}</p>
              </div>
            )}

            {pr.analysis.key_files && pr.analysis.key_files.length > 0 && (
              <div>
                <p className="text-xs font-medium text-neutral-400 uppercase tracking-wide mb-2">Key files</p>
                <ul className="space-y-1">
                  {pr.analysis.key_files.map((f) => (
                    <li key={f} className="text-xs font-mono text-neutral-600 bg-neutral-50 rounded px-2 py-1">{f}</li>
                  ))}
                </ul>
              </div>
            )}

            {pr.analysis.risk_reasons && pr.analysis.risk_reasons.length > 0 && (
              <div>
                <p className="text-xs font-medium text-neutral-400 uppercase tracking-wide mb-2">Risk factors</p>
                <ul className="space-y-1">
                  {pr.analysis.risk_reasons.map((r, i) => (
                    <li key={i} className="text-sm text-neutral-600 flex items-start gap-2">
                      <span className="text-neutral-300 mt-0.5">·</span>{r}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}

        {/* Body */}
        {pr.body && pr.body.trim() && (
          <div className="bg-white rounded-xl border border-neutral-200 p-6">
            <h2 className="text-sm font-semibold text-neutral-700 mb-3">Description</h2>
            <p className="text-sm text-neutral-600 leading-relaxed whitespace-pre-wrap">{pr.body}</p>
          </div>
        )}
      </main>
    </div>
  );
}

function StatCard({ label, value, valueClass }: { label: string; value: string; valueClass: string }) {
  return (
    <div className="bg-white rounded-xl border border-neutral-200 px-5 py-4">
      <p className="text-xs text-neutral-400 mb-1">{label}</p>
      <p className={`text-lg font-semibold font-mono ${valueClass}`}>{value}</p>
    </div>
  );
}

function StateBadge({ label, className }: { label: string; className: string }) {
  return (
    <Badge variant="outline" className={`text-xs font-medium py-0 px-2 ${className}`}>{label}</Badge>
  );
}

function SizeBadge({ size }: { size: string }) {
  const map: Record<string, { label: string; className: string }> = {
    small: { label: "S", className: "bg-green-50 text-green-700 border-green-100" },
    medium: { label: "M", className: "bg-yellow-50 text-yellow-700 border-yellow-100" },
    large: { label: "L", className: "bg-orange-50 text-orange-700 border-orange-100" },
  };
  const { label, className } = map[size] ?? map.medium;
  return <Badge variant="outline" className={`text-xs font-medium py-0 px-2 ${className}`}>{label}</Badge>;
}

function RiskBadge({ risk }: { risk: string }) {
  const map: Record<string, { label: string; className: string }> = {
    low: { label: "Low risk", className: "bg-green-50 text-green-700 border-green-100" },
    medium: { label: "Med risk", className: "bg-yellow-50 text-yellow-700 border-yellow-100" },
    high: { label: "High risk", className: "bg-red-50 text-red-700 border-red-100" },
  };
  const { label, className } = map[risk] ?? { label: risk, className: "" };
  return <Badge variant="outline" className={`text-xs font-medium py-0 px-2 ${className}`}>{label}</Badge>;
}

function computeSize(lines: number): string {
  if (lines <= 100) return "small";
  if (lines <= 400) return "medium";
  return "large";
}
