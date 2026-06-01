# CLI Commands

## `isoprism diff`

Generate a semantic diff for the current repository.

```bash
isoprism diff
isoprism diff <ref>
isoprism diff <ref1> <ref2>
isoprism diff staged
isoprism diff unstaged
```

Resolution rules:

- `isoprism diff` compares the default branch to `HEAD`.
- `isoprism diff <ref>` compares `<ref>` to `HEAD`.
- `isoprism diff <ref1> <ref2>` compares the two named refs.
- `staged` compares the git index against `HEAD`.
- `unstaged` compares working-tree changes against `HEAD`.

Supported flags:

```bash
--output <path>       write static HTML to a path
--open               open generated HTML in the browser
--no-open            do not open generated HTML
--json               print the ReviewGraphPayload JSON
--cache-dir <path>   override .isoprism cache location
--rebuild-cache      remove cached parse objects before generating
--share              future cloud upload flow; not implemented
```

## `isoprism serve`

Start the local daemon and browser viewer.

```bash
isoprism serve
isoprism serve --port 3717
isoprism serve --host 127.0.0.1
isoprism serve --no-web
isoprism serve --cache-dir <path>
```

Default behavior:

1. detect the repository root
2. choose the default `.isoprism/` cache
3. bind to loopback
4. serve the embedded viewer
5. open `http://127.0.0.1:3717/local`

The daemon must not bind to a public interface unless the user explicitly passes a non-loopback host.

## Frontend Development Mode

For Isoprism development only:

```bash
isoprism serve --web-dir ../web --web-port 3000
```

This mode starts or targets the local Next.js web app and uses the daemon as the local API. Installed users should not need Node.js or the source checkout.

## `isoprism annotate`

Annotations are local human/agent context attached to the current diff range.

```bash
isoprism annotate diff --reason-for-change "..." --expected-outcome "..."
isoprism annotate node <node-sha256> --description "..." --reasoning "..."
isoprism annotate test --node <node-sha256> --description "..."
```

Annotations live under `.isoprism/annotations/<base-sha>..<head-sha>/` and may be included in future review payloads.

## Default Branch Detection

The CLI detects the default branch in this order:

1. `git symbolic-ref --quiet refs/remotes/origin/HEAD`
2. `git remote show origin`
3. local `main`
4. local `master`
5. fail loudly and ask for explicit refs

## Future `gh` Integration

Future CLI work should use `gh` from the daemon process, not from React. `gh` can provide PR metadata and issue context for local `reviewItems` while keeping the API local-specific and the UI product-neutral.
