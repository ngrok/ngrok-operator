#!/usr/bin/env bash
#
# create-dry-run-worktree.sh — Creates a disposable detached worktree for
# rehearsing release prep without touching the current checkout.

set -euo pipefail

BASE_REF="origin/main"
FETCH=true

while [[ $# -gt 0 ]]; do
  case $1 in
    --base-ref) BASE_REF="$2"; shift 2 ;;
    --no-fetch) FETCH=false; shift ;;
    -h|--help)
      cat <<EOF
Usage: create-dry-run-worktree.sh [OPTIONS]

Creates a detached git worktree for release dry-runs.

Options:
  --base-ref REF  Base ref for the worktree (default: origin/main)
  --no-fetch      Skip fetching from origin before creating the worktree
  -h, --help      Show this help
EOF
      exit 0 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

if [[ "$FETCH" == true ]]; then
  git fetch origin --quiet
fi

worktree_dir=$(mktemp -d "${TMPDIR:-/tmp}/ngrok-operator-release-dry-run.XXXXXX")
git worktree add --detach "$worktree_dir" "$BASE_REF" >/dev/null

cat <<EOF
Created detached dry-run worktree:
  $worktree_dir

Next steps:
  cd $worktree_dir
  claude
  # or, once Copilot CLI is available:
  gh copilot -- --agent release-agent -p "prepare a dry-run release"

Cleanup:
  git worktree remove "$worktree_dir"
EOF
