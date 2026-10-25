#!/usr/bin/env bash
# Append one row to docs/crucible/porting/port-log/quirks.md, padded to the
# table's column widths (the table is the aligned style; prettier rejects a
# short row). Usage: addquirk.sh "<quirk>" "<java citation>" "<cards>"
# Cells must not contain a literal pipe.
set -euo pipefail
[ "$#" -eq 3 ] || { echo "usage: $0 quirk citation cards" >&2; exit 2; }
root="$(git rev-parse --show-toplevel)"
python3 - "$root/docs/crucible/porting/port-log/quirks.md" "$@" <<'PY'
import sys
path, *cells = sys.argv[1:]
if any("|" in c for c in cells):
    sys.exit("addquirk: a cell contains a pipe")
lines = open(path).read().split("\n")
rows = [i for i, l in enumerate(lines) if l.startswith("|")]
cols = [len(c) for c in lines[rows[-1]].split("|")[1:-1]]
if len(cols) != len(cells):
    sys.exit(f"addquirk: table has {len(cols)} columns, got {len(cells)} cells")
row = "|" + "|".join((" " + c).ljust(w) if len(c) + 1 < w else " " + c + " " for c, w in zip(cells, cols)) + "|"
lines.insert(rows[-1] + 1, row)
open(path, "w").write("\n".join(lines))
PY
