import Link from "next/link";
import { QueueItem } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";

interface Props {
  item: QueueItem;
  teamSlug: string;
}

export function QueueItemRow({ item, teamSlug }: Props) {
  const waitLabel = formatWait(item.waiting_hours);
  const sizeLabel = item.analysis?.size_label ?? computeSizeLabel(item.additions + item.deletions);
  const riskLabel = item.analysis?.risk_label;

  return (
    <Link
      href={`/orgs/${teamSlug}/pr/${item.id}`}
      className="group flex items-start gap-4 px-5 py-4 rounded-xl bg-white border border-neutral-100 hover:border-neutral-200 hover:shadow-sm transition-all"
    >
      {/* Author avatar */}
      <Avatar className="h-8 w-8 mt-0.5 shrink-0">
        <AvatarImage src={item.author_avatar_url} alt={item.author_github_login} />
        <AvatarFallback className="text-xs bg-neutral-100 text-neutral-600">
          {item.author_github_login.slice(0, 2).toUpperCase()}
        </AvatarFallback>
      </Avatar>

      {/* Main content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-3 mb-2">
          <div className="min-w-0">
            <p className="text-sm font-medium text-neutral-900 truncate leading-snug group-hover:text-neutral-700">
              {item.title}
            </p>
            <p className="text-xs text-neutral-400 mt-0.5">
              <span className="font-medium text-neutral-500">{item.repo_full_name}</span>
              {" · "}#{item.number}
              {" · "}
              {item.author_github_login}
            </p>
          </div>

          {/* Urgency indicator */}
          <div className="shrink-0 text-right">
            <p className="text-xs text-neutral-400">{waitLabel}</p>
          </div>
        </div>

        {/* Badges row */}
        <div className="flex items-center gap-1.5 flex-wrap">
          <ReviewStateBadge state={item.review_state} />
          <SizeBadge size={sizeLabel} />
          {riskLabel && <RiskBadge risk={riskLabel} />}
          {item.analysis?.impacted_areas?.slice(0, 3).map((area) => (
            <Badge
              key={area}
              variant="outline"
              className="text-xs font-normal border-neutral-200 text-neutral-500 py-0 px-2"
            >
              {area}
            </Badge>
          ))}
        </div>
      </div>
    </Link>
  );
}

function ReviewStateBadge({ state }: { state: QueueItem["review_state"] }) {
  const map: Record<QueueItem["review_state"], { label: string; className: string }> = {
    needs_review: {
      label: "Needs review",
      className: "bg-blue-50 text-blue-700 border-blue-100",
    },
    needs_author: {
      label: "Needs author",
      className: "bg-amber-50 text-amber-700 border-amber-100",
    },
    stalled: {
      label: "Stalled",
      className: "bg-red-50 text-red-700 border-red-100",
    },
    draft: {
      label: "Draft",
      className: "bg-neutral-50 text-neutral-500 border-neutral-200",
    },
  };
  const { label, className } = map[state] ?? map.needs_review;
  return (
    <Badge variant="outline" className={`text-xs font-medium py-0 px-2 ${className}`}>
      {label}
    </Badge>
  );
}

function SizeBadge({ size }: { size: string }) {
  const map: Record<string, { label: string; className: string }> = {
    small: { label: "S", className: "bg-green-50 text-green-700 border-green-100" },
    medium: { label: "M", className: "bg-yellow-50 text-yellow-700 border-yellow-100" },
    large: { label: "L", className: "bg-orange-50 text-orange-700 border-orange-100" },
  };
  const { label, className } = map[size] ?? map.medium;
  return (
    <Badge variant="outline" className={`text-xs font-medium py-0 px-2 ${className}`}>
      {label}
    </Badge>
  );
}

function RiskBadge({ risk }: { risk: string }) {
  const map: Record<string, { label: string; className: string }> = {
    low: { label: "Low risk", className: "bg-green-50 text-green-700 border-green-100" },
    medium: { label: "Med risk", className: "bg-yellow-50 text-yellow-700 border-yellow-100" },
    high: { label: "High risk", className: "bg-red-50 text-red-700 border-red-100" },
  };
  const { label, className } = map[risk] ?? { label: risk, className: "" };
  return (
    <Badge variant="outline" className={`text-xs font-medium py-0 px-2 ${className}`}>
      {label}
    </Badge>
  );
}

function formatWait(hours: number): string {
  if (hours < 1) return "just now";
  if (hours < 24) return `${Math.floor(hours)}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

function computeSizeLabel(linesChanged: number): string {
  if (linesChanged <= 100) return "small";
  if (linesChanged <= 400) return "medium";
  return "large";
}
