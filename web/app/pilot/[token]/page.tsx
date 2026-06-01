import { redirect } from "next/navigation";

// PilotInvitePage renders the pilot invite page for pilot forms.
export default async function PilotInvitePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  redirect(`/login?pilot=${encodeURIComponent(token)}`);
}
