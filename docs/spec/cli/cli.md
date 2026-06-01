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
- `staged` compares the git index against the detected default branch.
- `unstaged` compares the working tree against the detected default branch.
- In two-ref form, `staged` may be used as the second ref to compare the first ref against the git index.
- In two-ref form, `worktree`, `working-tree`, or `unstaged` may be used as the second ref to compare the first ref against the current working tree.

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

## `gh` Pull Request Integration

`isoprism serve` uses `gh` from the daemon process when the GitHub CLI is installed and authenticated for the current checkout. React never shells out to `gh`.

The daemon uses:

```bash
gh pr list --state open --limit 50 --json ...
gh pr view <number> --json ...
```

Open PRs are exposed as local review items in the browser side panel. Selecting a PR fetches its head ref into a hidden local ref under `refs/isoprism/pr/<number>/head`, compares the PR merge base against that hidden head ref, and returns the same PR graph data shape used by the cloud product.

The Review section also includes local cards when relevant:

- `Uncommitted changes`: compares `HEAD -> worktree`.
- `Worktree as PR`: compares the default-branch merge base against `worktree`, matching the shape of a GitHub PR opened from the current checkout.

The integration must not check out the PR branch or mutate the user’s working tree. If `gh` is unavailable, unauthenticated, or not in a GitHub-backed checkout, the local review cards and repo/program graph still work.
