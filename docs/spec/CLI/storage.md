# CLI Storage

## Source of Truth

Git is the source of truth for repository state. The CLI should ask git for refs, commits, trees, blob IDs, staged content, and diffs instead of duplicating branch state in Isoprism storage.

Core operations include:

```bash
git rev-parse --show-toplevel
git rev-parse --git-dir
git rev-parse --verify <ref>
git symbolic-ref --quiet refs/remotes/origin/HEAD
git remote show origin
git ls-tree -r -z <ref>
git ls-files -s -z
git cat-file blob <blob-sha>
git show <ref>:<path>
git diff --name-status --find-renames -z <from> <to>
git diff --numstat -z <from> <to>
git diff --patch <from> <to>
git write-tree
```

## Cache Directory

The default cache directory is:

```text
.isoprism/
```

The user can override it with `--cache-dir`. The cache should be safe to delete and rebuild. It should speed up repeated parsing by using git blob identity.

Current layout:

```text
.isoprism/
  objects/
    nodes/
      <node-sha256>.json
    index/
      blob_to_nodes/
        <git-blob-sha>.json
  annotations/
    <base-sha>..<head-sha>/
```

There is intentionally no `.isoprism/refs/` directory. Git already owns ref state.

## Node Objects

Each cached node object represents one parsed semantic component from one file blob.

```json
{
  "schema_version": "isoprism-node-v1",
  "type": "function",
  "full_name": "api/cmd/isoprism:main.run",
  "filepath": "api/cmd/isoprism/main.go",
  "git_blob_sha": "abc123",
  "line_start": 24,
  "line_end": 82,
  "inputs": [],
  "outputs": [],
  "fields": [],
  "language": "go",
  "kind": "function",
  "body_hash": "...",
  "body": "...",
  "doc_comment": "",
  "is_test": false,
  "is_entrypoint": false,
  "outgoing_links": []
}
```

Node IDs are deterministic SHA-256 values over schema version, node kind, full name, file path, and git blob SHA. A changed file blob creates new node IDs. If the product later needs stable logical identity across versions, add a separate logical ID rather than weakening the content-addressed object model.

## Blob Index

`objects/index/blob_to_nodes/<git-blob-sha>.json` maps semantic names in a blob to cached node IDs. On cache hit, the daemon can load node objects without reparsing the file.

## Graph Construction

For a graph request, the daemon:

1. resolves the target git ref or tree
2. lists supported source files and blob SHAs
3. skips semantic parsing for files larger than 512 KiB
4. loads cached nodes for unchanged blobs
5. parses cache misses with tree-sitter
6. builds a resolver index from source content
7. extracts call edges
8. adds semantic type edges such as `owns_method` and `uses_type`
9. returns bounded graph payloads to the UI

The 512 KiB file cap is a temporary safety guard. Oversized files may still appear as changed files in review payloads, but the daemon must not parse them into semantic nodes or resolver content.

## Review Context Storage

Local review contexts may come from git diffs, staged changes, unstaged changes, annotations, or future `gh` CLI PR data. They should be represented as runtime/session data first. Persist only durable annotations or cacheable parse results under `.isoprism/`.

Do not store GitHub tokens, Supabase tokens, or cloud account state in `.isoprism/`.
