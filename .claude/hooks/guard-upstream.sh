#!/usr/bin/env bash
# PreToolUse(Write|Edit|NotebookEdit): REV-1. Every line touched outside
# Crucible's own paths is a future rebase conflict, so such an edit needs the
# user's explicit approval. Paths already logged in upstream-patches.md pass.
root=$CLAUDE_PROJECT_DIR
f=$(jq -r '.tool_input.file_path // .tool_input.notebook_path // ""')
[ -n "$f" ] || exit 0
case "$f" in /*) ;; *) f="$root/$f" ;; esac
case "$f" in "$root"/*) rel=${f#"$root"/} ;; *) exit 0 ;; esac

case "$rel" in
crucible/* | docs/crucible/* | .claude/* | CLAUDE.md) exit 0 ;;
esac
grep -qF "\`$rel\`" "$root/docs/crucible/porting/upstream-patches.md" 2>/dev/null && exit 0

jq -n --arg r "REV-1: $rel is upstream Forge. Edit only for a PORT-8 fix, and log it in docs/crucible/porting/upstream-patches.md in the same commit." \
  '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "ask", permissionDecisionReason: $r}}'
