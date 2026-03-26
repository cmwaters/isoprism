"use client";

import { useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { createClient } from "@/lib/supabase/client";
import { apiFetch } from "@/lib/api";
import { Button } from "@/components/ui/button";

function JoinContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const supabase = createClient();

  const orgSlug = searchParams.get("org");
  const [status, setStatus] = useState<"idle" | "loading" | "sent" | "error">("idle");

  async function handleRequestAccess() {
    if (!orgSlug) return;
    setStatus("loading");

    const { data: sessionData } = await supabase.auth.getSession();
    const token = sessionData.session?.access_token;
    if (!token) {
      router.push("/login");
      return;
    }

    try {
      await apiFetch(`/api/v1/orgs/${orgSlug}/join-requests`, token, {
        method: "POST",
      });
      setStatus("sent");
    } catch {
      setStatus("error");
    }
  }

  if (!orgSlug) {
    router.push("/onboarding");
    return null;
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-neutral-50">
      <div className="w-full max-w-sm px-6">
        <div className="mb-12 flex items-center gap-2">
          <div className="h-6 w-6 rounded-full bg-neutral-900" />
          <span className="text-lg font-semibold tracking-tight">Aperture64</span>
        </div>

        {status === "sent" ? (
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-neutral-900 mb-2">
              Request sent
            </h1>
            <p className="text-sm text-neutral-500">
              Your request to join <span className="font-medium">{orgSlug}</span> has been sent to the org admin. You&apos;ll get access once they approve it.
            </p>
          </div>
        ) : (
          <div>
            <div className="mb-8">
              <h1 className="text-2xl font-semibold tracking-tight text-neutral-900 mb-2">
                Join {orgSlug}
              </h1>
              <p className="text-sm text-neutral-500">
                Your GitHub account is a member of <span className="font-medium">{orgSlug}</span>, but you need approval to access this workspace.
              </p>
            </div>

            {status === "error" && (
              <p className="text-sm text-red-500 mb-4">
                Something went wrong. Please try again.
              </p>
            )}

            <Button
              onClick={handleRequestAccess}
              disabled={status === "loading"}
              className="w-full h-11 bg-neutral-900 hover:bg-neutral-700 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-40"
            >
              {status === "loading" ? (
                <span className="flex items-center gap-2">
                  <span className="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
                  Sending…
                </span>
              ) : (
                "Request access"
              )}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

export default function JoinPage() {
  return (
    <Suspense>
      <JoinContent />
    </Suspense>
  );
}
