import { createBrowserClient } from "@supabase/ssr";

// createClient creates the Supabase client for the current runtime.
export function createClient() {
  return createBrowserClient(
    process.env.NEXT_PUBLIC_SUPABASE_URL!,
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!
  );
}
