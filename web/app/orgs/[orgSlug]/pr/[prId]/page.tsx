import { redirect } from "next/navigation";
import Link from "next/link";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { AppHeader } from "@/components/layout/app-header";
import { Badge } from "@/components/ui/badge";
import { QueueItem } from "@/lib/types";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ orgSlug: string; prId: string }>;
}

export default async function PRDetailPage({ params }: Props) {
  const { orgSlug, prId } = await params;

  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  let pr: QueueItem | null = null;
  try {
    pr = await apiFetch<QueueItem>(`/api/v1/orgs/${orgSlug}/queue/${prId}`, token);
  } catch {
    // PR not found or not accessible
  }

  if (!pr) {
    return (
      <div className="min-h-screen bg-neutral-50">
        <AppHeader orgSlug={orgSlug} />
        <main className="max-w-3xl mx-auto px-6 py-10">
          <p className="text-sm text-neutral-500">Pull request not found.</p>
        </main>
      </div>
    );
  }

  const sizeLabel = pr.analysis?.size_label ?? computeSizeLabel(pr.additions + pr.deletions);
  const waitLabel = formatWait(pr.waiting_hours);

  return (
    <div className="min-h-screen bg-neutral-50">
      <AppHeader orgSlug={orgSlug} />
      <main className="max-w-3xl mx-auto px-6 py-10">
        {/* Back */}
        <Link
          href={`/orgs/${orgSlug}`}
          className="inline-flex items-center gap-1.5 text-sm text-neutral-400 hover:text-neutral-600 mb-6 transition-colors"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" />
          </svg>
          Back to queue
        </Link>

        {/* Header */}
        <div className="bg-white rounded-xl border border-neutral-200 px-6 py-5 mb-4">
          <div className="flex items-start justify-between gap-4 mb-3">
            <div className="min-w-0">
              <h1 className="text-lg font-semibold text-neutral-900 leading-snug">
                {pr.title}
              </h1>
              <p className="text-sm text-neutral-400 mt-1">
                <span className="font-medium text-neutral-500">{pr.repo_full_name}</span>
                {" · "}#{pr.number}
                {" · "}opened by{" "}
                <span className="font-medium text-neutral-500">{pr.author_github_login}</span>
                {" · "}{waitLabel} ago
              </p>
            </div>
            <a
              href={pr.html_url}
              target="_blank"
              rel="noopener noreferrer"
              className="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg border border-neutral-200 text-neutral-600 hover:bg-neutral-50 transition-colors"
            >
              View on GitHub
              <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 6H5.25A2.25 2.25 0 003 8.25v10.5A2.25 2.25 0 005.25 21h10.5A2.25 2.25 0 0018 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
              </svg>
            </a>
          </div>

          {/* Badges */}
          <div className="flex items-center gap-2 flex-wrap">
            <Badge variant="outline" className="text-xs font-medium py-0 px-2 bg-blue-50 text-blue-700 border-blue-100">
              {pr.review_state.replace(/_/g, " ")}
            </Badge>
            <Badge variant="outline" className="text-xs font-medium py-0 px-2">
              {sizeLabel} ({pr.additions + pr.deletions} lines)
            </Badge>
            {pr.analysis?.risk_label && (
              <Badge variant="outline" className="text-xs font-medium py-0 px-2">
                {pr.analysis.risk_label} risk
              </Badge>
            )}
            {pr.draft && (
              <Badge variant="outline" className="text-xs font-medium py-0 px-2 text-neutral-400">
                Draft
              </Badge>
            )}
          </div>
        </div>

        {/* Stats row */}
        <div className="grid grid-cols-3 gap-3 mb-4">
          {[
            { label: "Files changed", value: pr.changed_files },
            { label: "Additions", value: `+${pr.additions}` },
            { label: "Deletions", value: `-${pr.deletions}` },
          ].map(({ label, value }) => (
            <div key={label} className="bg-white rounded-xl border border-neutral-200 px-4 py-3">
              <p className="text-xs text-neutral-400 mb-1">{label}</p>
              <p className="text-lg font-semibold text-neutral-900">{value}</p>
            </div>
          ))}
        </div>

        {/* AI analysis */}
        {pr.analysis ? (
          <div className="bg-white rounded-xl border border-neutral-200 px-6 py-5">
            <h2 className="text-sm font-semibold text-neutral-700 mb-3">AI Analysis</h2>
            {pr.analysis.summary && (
              <p className="text-sm text-neutral-600 leading-relaxed mb-4">{pr.analysis.summary}</p>
            )}
            {pr.analysis.impacted_areas?.length > 0 && (
              <div className="mb-3">
                <p className="text-xs text-neutral-400 uppercase tracking-wide mb-2">Impacted areas</p>
                <div className="flex flex-wrap gap-1.5">
                  {pr.analysis.impacted_areas.map((area) => (
                    <Badge key={area} variant="outline" className="text-xs font-normal border-neutral-200 text-neutral-500">
                      {area}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : (
          <div className="bg-white rounded-xl border border-neutral-200 px-6 py-5">
            <h2 className="text-sm font-semibold text-neutral-700 mb-2">AI Analysis</h2>
            <p className="text-sm text-neutral-400">Analysis not yet available for this pull request.</p>
          </div>
        )}
      </main>
    </div>
  );
}

function formatWait(hours: number): string {
  if (hours < 1) return "just now";
  if (hours < 24) return `${Math.floor(hours)}h`;
  return `${Math.floor(hours / 24)}d`;
}

function computeSizeLabel(lines: number): string {
  if (lines <= 100) return "small";
  if (lines <= 400) return "medium";
  return "large";
}
