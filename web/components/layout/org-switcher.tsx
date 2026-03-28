"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { Organization } from "@/lib/types";

interface Props {
  currentOrgSlug: string;
}

export function OrgSwitcher({ currentOrgSlug }: Props) {
  const [currentOrg, setCurrentOrg] = useState<Organization | null>(null);

  useEffect(() => {
    async function init() {
      const supabase = createClient();
      const { data: { session } } = await supabase.auth.getSession();
      if (!session) return;
      try {
        const { orgs } = await apiFetch<{ orgs: Organization[] }>("/api/v1/me/orgs", session.access_token);
        setCurrentOrg(orgs?.find((o) => o.slug === currentOrgSlug) ?? null);
      } catch {}
    }
    init();
  }, [currentOrgSlug]);

  const name = currentOrg?.name ?? currentOrgSlug;
  const letter = name[0]?.toUpperCase() ?? "?";

  return (
    <Link
      href={`/orgs/${currentOrgSlug}/settings`}
      className="flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-neutral-100 transition-colors"
    >
      {currentOrg?.avatar_url ? (
        <img src={currentOrg.avatar_url} alt={name} className="h-6 w-6 rounded-full object-cover" />
      ) : (
        <div className="h-6 w-6 rounded-full bg-neutral-200 flex items-center justify-center">
          <span className="text-xs font-semibold text-neutral-600">{letter}</span>
        </div>
      )}
      <span className="text-sm font-medium text-neutral-900 truncate">{name}</span>
    </Link>
  );
}
