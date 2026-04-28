<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

# Isoprism Development Flow

- Use only `main` and `preview`; never create feature branches or open PRs.
- Develop frontend-only changes on `preview`.
- Run local web against the deployed production API:
  - `NEXT_PUBLIC_API_URL=https://api.isoprism.com npm run dev`
  - Open `http://localhost:3000`.
- The deployed Railway API uses the single production GitHub App. Keep its frontend config aligned with:
  - `FRONTEND_URL=https://isoprism.com`
  - `FRONTEND_URLS=https://isoprism.com,http://localhost:3000`
- If a web change needs an API change, make the API change on `main`, push `main` so Railway deploys it, then continue frontend work on `preview`.
- Merge `preview` into `main` only when the frontend change is finalised and ready for production.
- Keep documentation updated with code changes; docs should remain a reliable source of truth.
