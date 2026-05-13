#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/codex-worktree-cleanup.sh [idea-name]

Removes a Codex-friendly Isoprism worktree and deletes its local idea branch.

When Codex Desktop provides CODEX_WORKTREE_PATH, the script validates cleanup
safety and lets Codex Desktop remove the worktree itself.

Environment overrides:
  CODEX_SOURCE_TREE_PATH  Source workspace path provided by Codex Desktop.
  CODEX_WORKTREE_PATH     Worktree path provided by Codex Desktop.
  CODEX_WORKTREE_PARENT   Parent directory for worktrees.
                         Default: $HOME/.codex/worktrees
  CODEX_WORKTREE_SLUG     Directory slug under CODEX_WORKTREE_PARENT.
                         Default: derived from CODEX_THREAD_ID and idea name
  CODEX_WORKTREE_BRANCH   Branch name to delete.
                         Default: idea/<idea-name>
  CODEX_FORCE_CLEANUP     Set to 1 to allow git worktree remove --force.
                         The script still prints the dirty status first.

Example:
  scripts/codex-worktree-cleanup.sh
  scripts/codex-worktree-cleanup.sh graph-context
USAGE
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

codex_managed=0
if [ -n "${CODEX_WORKTREE_PATH:-}" ]; then
  codex_managed=1
fi

if [ "$#" -lt 1 ] && [ "$codex_managed" != "1" ]; then
  usage
  exit 1
fi

idea_name="${1:-${CODEX_WORKTREE_IDEA:-$(basename "${CODEX_WORKTREE_PATH:-isoprism-worktree}")}}"
repo_root="${CODEX_SOURCE_TREE_PATH:-$(git rev-parse --show-toplevel)}"
repo_name="$(basename "$repo_root")"

safe_name="$(
  printf '%s' "$idea_name" |
    tr '[:upper:]' '[:lower:]' |
    tr -cs 'a-z0-9._-' '-'
)"
safe_name="${safe_name%-}"
safe_name="${safe_name#-}"

if [ -z "$safe_name" ]; then
  echo "Could not derive a safe name from: $idea_name" >&2
  exit 1
fi

thread_slug=""
if [ -n "${CODEX_THREAD_ID:-}" ]; then
  thread_slug="${CODEX_THREAD_ID%%-*}"
fi

default_slug="$safe_name"
if [ -n "$thread_slug" ]; then
  default_slug="$thread_slug-$safe_name"
fi

worktree_slug="${CODEX_WORKTREE_SLUG:-$default_slug}"
worktree_parent="${CODEX_WORKTREE_PARENT:-$HOME/.codex/worktrees}"
branch="${CODEX_WORKTREE_BRANCH:-idea/$safe_name}"
worktree_path="${CODEX_WORKTREE_PATH:-$worktree_parent/$worktree_slug/$repo_name}"

cd "$repo_root"

if [ -d "$worktree_path" ]; then
  dirty_status="$(git -C "$worktree_path" status --porcelain)"

  if [ -n "$dirty_status" ]; then
    echo "Worktree has local changes:"
    git -C "$worktree_path" status --short
    echo

    if [ "${CODEX_FORCE_CLEANUP:-0}" != "1" ]; then
      echo "Refusing to remove dirty worktree."
      echo "Commit, stash, discard changes, or rerun with CODEX_FORCE_CLEANUP=1."
      exit 1
    fi
  fi

  if [ "$codex_managed" = "0" ]; then
    if [ "${CODEX_FORCE_CLEANUP:-0}" = "1" ]; then
      git worktree remove --force "$worktree_path"
    else
      git worktree remove "$worktree_path"
    fi
  fi
fi

if [ "$codex_managed" = "0" ] && git show-ref --verify --quiet "refs/heads/$branch"; then
  git branch -d "$branch"
fi

if [ "$codex_managed" = "0" ]; then
  git worktree prune
fi

echo "Cleanup checked:"
echo "  Path:   $worktree_path"
echo "  Branch: $branch"
