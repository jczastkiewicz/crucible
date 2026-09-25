#!/usr/bin/env bash
# PreToolUse(Bash): block `git commit` until every CI gate passes, run in the
# checkout being committed (a porter's worktree, not the main one).
# Exit 2 blocks the tool call and hands stderr back to Claude.
. "$(dirname "$0")/lib.sh"
in=$(cat)
cmd=$(jq -r '.tool_input.command // ""' <<<"$in")
cwd=$(jq -r '.cwd // ""' <<<"$in")
dir=$(commit_dir "$cmd" "${cwd:-${CLAUDE_PROJECT_DIR:-.}}")
[ -n "$dir" ] || exit 0
root=$(repo_root "$dir")
# Gates whose inputs are unchanged since HEAD are skipped (gates.sh header); CI
# still runs every gate on push.
out=$(GATES_AUTO_SKIP=1 "$root/crucible/scripts/gates.sh" full 2>&1) && exit 0
printf 'Commit blocked: CI gates failed in %s. Fix, then commit again.\n%s\n' "$root" "$out" >&2
exit 2
