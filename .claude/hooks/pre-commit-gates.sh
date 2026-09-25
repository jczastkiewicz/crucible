#!/usr/bin/env bash
# PreToolUse(Bash): block `git commit` until every CI gate passes.
# Exit 2 blocks the tool call and hands stderr back to Claude.
cmd=$(jq -r '.tool_input.command // ""')
printf '%s' "$cmd" | grep -Eq '(^|[^[:alnum:]_-])git[[:space:]].*commit([[:space:]]|$)' || exit 0
# Gates whose inputs are unchanged since HEAD are skipped (gates.sh header); CI
# still runs every gate on push.
out=$(GATES_AUTO_SKIP=1 "$CLAUDE_PROJECT_DIR/crucible/scripts/gates.sh" full 2>&1) && exit 0
printf 'Commit blocked: CI gates failed. Fix, then commit again.\n%s\n' "$out" >&2
exit 2
