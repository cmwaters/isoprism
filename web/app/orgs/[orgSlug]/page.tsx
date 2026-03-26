import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import { apiFetch } from "@/lib/api";
import { QueueResponse } from "@/lib/types";
import { QueueList } from "@/components/queue/queue-list";
import { AppHeader } from "@/components/layout/app-header";
import { RefreshButton } from "@/components/queue/refresh-button";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ orgSlug: string }>;
}

export default async function ActivityPage({ params }: Props) {
  const { orgSlug } = await params;

  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();

  if (!user) redirect("/login");

  const { data: session } = await supabase.auth.getSession();
  const token = session.session?.access_token;
  if (!token) redirect("/login");

  let queue: QueueResponse = { items: [], total: 0 };
  try {
    queue = await apiFetch<QueueResponse>(
      `/api/v1/orgs/${orgSlug}/queue`,
      token
    );
  } catch (err) {
    console.error("Failed to load queue:", err);
  }

  return (
    <div className="min-h-screen bg-neutral-50">
      <AppHeader orgSlug={orgSlug} activeTab="queue" />
      <main className="max-w-3xl mx-auto px-6 py-10">
        <div className="mb-8 flex items-start justify-between">
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-neutral-900">
              Queue
            </h1>
            <p className="text-sm text-neutral-500 mt-1">
              {queue.total === 0
                ? "All clear — no open pull requests."
                : `${queue.total} open pull request${queue.total === 1 ? "" : "s"}, ranked by urgency.`}
            </p>
          </div>
          <RefreshButton />
        </div>
        <QueueList items={queue.items} teamSlug={orgSlug} />
      </main>
    </div>
  );
}
