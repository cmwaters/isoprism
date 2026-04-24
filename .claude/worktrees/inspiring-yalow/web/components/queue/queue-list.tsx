import { QueueItem } from "@/lib/types";
import { QueueItemRow } from "./queue-item-row";

interface Props {
  items: QueueItem[];
  teamSlug: string;
}

export function QueueList({ items, teamSlug }: Props) {
  if (!items || items.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <div className="h-12 w-12 rounded-full bg-neutral-100 flex items-center justify-center mb-4">
          <svg className="h-5 w-5 text-neutral-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <p className="text-sm font-medium text-neutral-600">All caught up</p>
        <p className="text-xs text-neutral-400 mt-1">No open pull requests need attention.</p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {items.map((item) => (
        <QueueItemRow key={item.id} item={item} teamSlug={teamSlug} />
      ))}
    </div>
  );
}
