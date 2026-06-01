/**
 * Thin wrapper around the Go API. All requests attach the Supabase session token.
 */

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "https://api.isoprism.com";

const LOCAL_API_URL = process.env.NEXT_PUBLIC_ISOPRISM_LOCAL_API_URL;

function apiURL() {
  if (typeof window !== "undefined" && window.location.pathname.startsWith("/local")) {
    return LOCAL_API_URL ?? window.location.origin ?? API_URL;
  }
  return API_URL;
}

export function apiBaseURL(token: string): string {
  if (token === "local" && typeof window !== "undefined") {
    return window.location.origin;
  }
  return apiURL();
}

export async function apiFetch<T>(
  path: string,
  token: string,
  options?: RequestInit
): Promise<T> {
  const res = await fetch(`${apiBaseURL(token)}${path}`, {
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
