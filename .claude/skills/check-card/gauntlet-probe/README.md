# Gauntlet probes (R20c)

One L2 probe scenario per gauntlet card: the card played on a fixed board, Go's post-state compared with Java's
(ADR-0060). A `same` probe is promoted to `crucible/testdata/scenarios/gauntlet-<card>`, so the CI L2 gate keeps it.

Run from the repo root, `S` a scratch dir:

```bash
python3 -I resolve.py cards.txt > paths.tsv          # card names -> script paths
python3 -I sweep.py paths.tsv; python3 -I vsweep.py paths.tsv   # names Go never mentions
python3 -I gen.py bN.spec $S/probe/bN                # spec -> probe scenarios
cd crucible && go run ./tools/l2diff emit -scenarios $S/probe/bN -out $S/l2/bN
L2_NO_BUILD=1 oracle-java/l2-run.sh $S/l2/bN/manifest.tsv $S/l2/bN/java   # drop L2_NO_BUILD after Java edits
go run ./tools/l2diff diff -dir $S/l2/bN             # report.txt, diffs/
python3 -I ../.claude/skills/check-card/gauntlet-probe/promote.py $S/probe/bN $S/l2/bN testdata/scenarios [skip probe names]
```

Spec block (`b1.spec` is the A-D batch): `== Card Name [#variant]`, then `cost R 2` (lands to tap; generic paid by the
first color, queued `paygeneric`), `set key=value` / `set +key=value` (setup), `nocast` / `play`, `queue ...` (put
before the cast), other lines are actions after it (default `resolvestack`). Base board: ai has Grizzly Bears 50,
Ornithopter 51, Swamp 52, Pacifism 53 on the Bears; hands, graveyards and six-Island libraries (ids 80-85, 90-95).
Go-side gotchas: a planeswalker needs `|Counters:LOYALTY=n`; mill and graveyard exile ask `cardorder`; `payphyrexian`
takes true/false; `tapformana` cannot tap for {C}. Java maps only the `ScenarioAnswers.KINDS` queue kinds.
