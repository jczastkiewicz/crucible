#!/usr/bin/env bash
# SessionStart (Claude Code on the web only): install the golangci-lint the
# commit hook's gates need. The container image ships one built with an
# older Go than crucible/go.mod targets, which fails every commit
# (crucible/scripts/ensure-golangci.sh has the details). Synchronous, so the
# first commit never races it; a no-op once the right binary exists.
set -euo pipefail
[ "${CLAUDE_CODE_REMOTE:-}" = "true" ] || exit 0
"$CLAUDE_PROJECT_DIR/crucible/scripts/ensure-golangci.sh" >/dev/null
