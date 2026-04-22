# Git Workflow

This is a single-person prototype. Use only `main` and `preview` branches:

- Commit directly to `main` — no feature branches, no PRs
- `preview` mirrors `main` (fast-forward only); sync it by merging `main` into `preview` and pushing both
- Never create new branches or open pull requests
- Don't worry about backwards compatibility or legacy code
