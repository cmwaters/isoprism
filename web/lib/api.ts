/**
 * Thin wrapper around the Go API. All requests attach the Supabase session token.
 */

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "https://api.isoprism.com";

const LOCAL_API_URL = process.env.NEXT_PUBLIC_ISOPRISM_LOCAL_API_URL;

// apiURL chooses the hosted or local API base URL for web requests.
function apiURL() {
  if (typeof window !== "undefined" && window.location.pathname.startsWith("/local")) {
    return LOCAL_API_URL ?? window.location.origin ?? API_URL;
  }
  return API_URL;
}

// apiBaseURL returns the API base URL for the current auth mode.
export function apiBaseURL(token: string): string {
  if (token === "local" && typeof window !== "undefined") {
    return window.location.origin;
  }
  return apiURL();
}

// apiFetch performs an authenticated API request and returns typed JSON.
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
