"use client";

import { useParams } from "next/navigation";
import { SettingsView } from "@/components/settings/settings-view";

export default function SettingsPage() {
  const params = useParams<{ owner: string }>();
  const account = decodeURIComponent(params.owner);

  return <SettingsView account={account} />;
}
