import { redirect } from "next/navigation";

export default async function BetaInvitePage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  redirect(`/login?beta=${encodeURIComponent(token)}`);
}
