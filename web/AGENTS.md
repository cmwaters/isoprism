<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

# Isoprism Development Flow

- Use `main` for all development and production changes for now; never create feature branches or open PRs.
- Keep `preview` only as a synced mirror of `main` while any external tooling still expects it.
- Run local web against the deployed production API:
  - `NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev`
  - Open `http://localhost:3000`.
- The deployed Railway API uses the single production GitHub App. Keep its frontend config aligned with:
  - `FRONTEND_URL=https://isoprism.com`
  - `FRONTEND_URLS=https://isoprism.com,http://localhost:3000`
- Commit and push verified web and API changes to `main`; Vercel deploys web changes and Railway deploys API changes.
- Keep documentation updated with code changes; docs should remain a reliable source of truth.
