/**
 * Thin wrapper around the Go API. All requests attach the Supabase session token.
 */

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "https://api.isoprism.com";

declare global {
  interface Window {
    __ISOPRISM_API_URL__?: string;
  }
}

function apiURL() {
  if (typeof window !== "undefined" && window.location.pathname.startsWith("/local")) {
    return "";
  }
  if (typeof window !== "undefined" && window.__ISOPRISM_API_URL__) {
    return window.__ISOPRISM_API_URL__;
  }
  return API_URL;
}

export async function apiFetch<T>(
  path: string,
  token: string,
  options?: RequestInit
): Promise<T> {
  const res = await fetch(`${apiURL()}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...options?.headers,
    },
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`API error ${res.status}: ${text}`);
  }

  return res.json() as Promise<T>;
}
