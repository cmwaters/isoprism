import Link from "next/link";

interface Props {
  orgSlug: string;
  activeTab?: "queue" | "settings";
}

export function AppHeader({ orgSlug, activeTab = "queue" }: Props) {
  return (
    <header className="border-b border-neutral-100 bg-white">
      <div className="max-w-3xl mx-auto px-6 h-14 flex items-center justify-between">
        <div className="flex items-center gap-6">
          <Link href={`/orgs/${orgSlug}`} className="flex items-center gap-2">
            <div className="h-5 w-5 rounded-full bg-neutral-900" />
            <span className="text-sm font-semibold tracking-tight">Aperture</span>
          </Link>
          <nav className="flex items-center gap-1">
            <Link
              href={`/orgs/${orgSlug}`}
              className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                activeTab === "queue"
                  ? "text-neutral-900 bg-neutral-100"
                  : "text-neutral-500 hover:text-neutral-700 hover:bg-neutral-50"
              }`}
            >
              Queue
            </Link>
            <Link
              href={`/orgs/${orgSlug}/settings`}
              className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                activeTab === "settings"
                  ? "text-neutral-900 bg-neutral-100"
                  : "text-neutral-500 hover:text-neutral-700 hover:bg-neutral-50"
              }`}
            >
              Settings
            </Link>
          </nav>
        </div>
      </div>
    </header>
  );
}
