import { redirect } from "next/navigation";

export default async function PilotInvitePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  redirect(`/login?pilot=${encodeURIComponent(token)}`);
}
