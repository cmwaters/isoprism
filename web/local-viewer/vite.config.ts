import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const webRoot = path.resolve(__dirname, "..");
const repoRoot = path.resolve(webRoot, "..");

export default defineConfig({
  root: __dirname,
  base: "/local/",
  plugins: [react()],
  resolve: {
    alias: {
      "@": webRoot,
      "next/link": path.resolve(__dirname, "next-link.tsx"),
    },
  },
  define: {
    "process.env.NEXT_PUBLIC_API_URL": "undefined",
    "process.env.NEXT_PUBLIC_ISOPRISM_LOCAL_API_URL": "undefined",
    "process.env.NEXT_PUBLIC_VERCEL_GIT_COMMIT_SHA": JSON.stringify("local"),
  },
  build: {
    outDir: path.resolve(repoRoot, "api/internal/localgraph/viewer"),
    emptyOutDir: true,
  },
});
