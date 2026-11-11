#!/usr/bin/env bash
# The P4 trigger-mode measure (m5-rules-coverage.md): every corpus `T:Mode$` value,
# and whether a card with that mode is in some scenario's setup.state. Only a
# card the fixture loads counts; a Mode$ line quoted in a comment does not.
# Usage: trigger-mode-coverage.sh [-missing]
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
python3 - "$root" "${1:-}" <<'PY'
import collections, glob, os, re, sys
root, flag = sys.argv[1], sys.argv[2]
modes = collections.defaultdict(list)
by_name = {}
for path in glob.glob(f"{root}/forge-gui/res/cardsfolder/**/*.txt", recursive=True):
    text = open(path, encoding="utf-8", errors="replace").read()
    names = [n.strip() for n in re.findall(r"^Name:(.*)$", text, re.M)]
    found = set(re.findall(r"^T:Mode\$ ?([A-Za-z]+)", text, re.M))
    for n in names:
        by_name[n] = found
    for m in found:
        modes[m].append(names[0] if names else os.path.basename(path))
covered = collections.defaultdict(set)
for setup in glob.glob(f"{root}/crucible/testdata/scenarios/*/setup.state"):
    case = os.path.basename(os.path.dirname(setup))
    for line in open(setup, encoding="utf-8"):
        if "=" not in line:
            continue
        for entry in line.split("=", 1)[1].strip().split(";"):
            for m in by_name.get(entry.split("|", 1)[0].strip(), ()):
                covered[m].add(case)
missing = sorted((m for m in modes if m not in covered), key=lambda m: (-len(modes[m]), m))
print(f"{len(modes)} trigger modes, {len(modes) - len(missing)} with a fixture card, {len(missing)} without")
if flag == "-missing":
    for m in missing:
        print(f"{m}\t{len(modes[m])}\t{modes[m][0]}")
PY
