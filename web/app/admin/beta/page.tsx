import { redirect } from "next/navigation";

// LegacyBetaAdminPage renders the legacy beta admin page for pilot administration.
export default function LegacyBetaAdminPage() {
  redirect("/admin");
}
