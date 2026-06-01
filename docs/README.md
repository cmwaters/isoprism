# Isoprism Docs

This folder is the documentation source of truth for Isoprism.

## Structure

```text
docs/
  infrastructure.md
  spec/
    CLI/
    Cloud/
    Common/
  adr/
```

## Spec

`docs/spec/` contains the current product and technical contracts.

- `CLI/`: local-first command, daemon, storage, API, and UI behavior.
- `Cloud/`: hosted GitHub App product, storage, API, UI, and AI behavior.
- `Common/`: shared semantic graph and React UI contracts.

## ADR

`docs/adr/` contains implementation decisions and plans. ADRs explain why a direction was chosen; specs describe the current intended behavior.

## Infrastructure

`docs/infrastructure.md` covers both runtimes:

- CLI local daemon and embedded viewer
- cloud Vercel/Railway/Supabase/GitHub/Gemini deployment
