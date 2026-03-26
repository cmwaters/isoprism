"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

export default function RequestSentPage() {
  const router = useRouter();

  useEffect(() => {
    const timer = setTimeout(() => router.replace("/"), 5000);
    return () => clearTimeout(timer);
  }, [router]);

  return (
    <div className="min-h-screen flex items-center justify-center bg-neutral-50">
      <div className="w-full max-w-sm px-6 text-center">
        <div className="mb-6 flex justify-center">
          <div className="h-12 w-12 rounded-full bg-green-100 flex items-center justify-center">
            <svg className="h-6 w-6 text-green-600" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
        </div>
        <h1 className="text-xl font-semibold tracking-tight text-neutral-900 mb-2">
          Request sent
        </h1>
        <p className="text-sm text-neutral-500 mb-6">
          Your request to add the GitHub org has been sent to the org&apos;s administrators. You&apos;ll be notified once it&apos;s approved.
        </p>
        <button
          onClick={() => router.replace("/")}
          className="text-sm text-neutral-500 underline underline-offset-2 hover:text-neutral-700 transition-colors"
        >
          Back to app
        </button>
        <p className="text-xs text-neutral-400 mt-4">Redirecting automatically…</p>
      </div>
    </div>
  );
}
