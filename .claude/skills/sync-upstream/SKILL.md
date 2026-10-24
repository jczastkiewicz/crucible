---
name: sync-upstream
description:
  Merge Card-Forge/forge's master into this fork, following docs/crucible/runbooks/upstream-sync.md, then verify the Go
  port still agrees with what moved. Use when asked to sync, update from upstream, or pull in the latest Forge changes.
---

# Sync with Card-Forge/forge

Procedure: `docs/crucible/runbooks/upstream-sync.md` (ADR-0015). Read it first, every time: merge commands, the gate
table, golden regeneration, `javacycles -expect 88`, `tools/metrics`, conflicts, the "Never" list. This skill adds only
what the runbook does not cover.

## Merge

| Situation | Do |
| --- | --- |
| Landing the sync PR | Merge commit. Never squash, never rebase (ADR-0001). Only irreversible mistake in the procedure |
| Conflict in `forge-gui/res/cardsfolder/` or upstream file | `git checkout --theirs <path>`, never a hand-merge |
| Conflict in a Crucible-owned root config (REV-1 list) | Crucible's side plus what upstream added |
| `gh` command | `--repo jczastkiewicz/crucible`: `gh` defaults to upstream |
| Branch diverged from its remote | Push to a new branch name. Never force-push: Claude Code's git guard blocks it, and routing around a harness denial is not allowed |

## Verify the port, not only the gates

Run the uncached suite once after the merge. `gates.sh full` is tuned for one small diff and caches tests:

```bash
cd crucible && go test -race -count=1 ./...
```

Two failure shapes, two responses:

| Failure | Response |
| --- | --- |
| Corpus golden moved, no "unknown"/"unexpected" wording | New data. Regenerate with that test's `-update`, read the diff (TEST-6). Beyond the runbook's table also `TestCorpusParses` (`internal/carddb`), `TestCorpusVocabulary`, `TestCorpusCosts`, `TestCorpusDecklists`, `TestCorpusAST`. Trust `go test` output over any list |
| Parser rejects a script, or `apiscan` reports a param nothing reads | Forge card-script bug (PORT-8). Fix locally, log in `upstream-patches.md` "Pending upstream fixes", separate branch `upstream-pr/<slug>` off `upstream/master` with only that fix. Never in the sync PR |

`apiscan -check -api` never checks `S:` static-ability params: `tools/apiscan/scan.go` `readEffectParams` walks only
`ability/effects/*.java`. Reason: Sanctum Lurker's `Affected$` (should be `ValidCard$`) passed every gate. A sync that
adds or touches an `S:` line in a mode Crucible ports: check its params against `forge-game/.../staticability/` by hand.

## Review every upstream commit against the port

Mandatory, every sync, before the PR: gates only see data that moved, not a rule that changed under a port. Walk **all**
commits, not only the Java diff.

```bash
git log --reverse --format='%h %s' <old-sha>..<new-sha>        # every commit, read the whole list
git diff <old-sha> <new-sha> --stat -- forge-game forge-core forge-gui/res/lists forge-gui/res/cardsfolder/upcoming
git diff <old-sha> <new-sha> --name-only | rg -v 'cardsfolder|forge-gui/src|forge-ai|forge-gui-|\.(png|jpg)$'
```

Classify each commit, then act. A commit is "ignored" only after the file list shows nothing under `forge-game`,
`forge-core`, `forge-gui/res` (lists, editions, tokens, rules) or a card script Crucible has a note about.

| Commit touches | Check | Do |
| --- | --- | --- |
| `forge-game`/`forge-core` Java | grep the class and method in `crucible/internal/engine/*.go` and `docs/crucible/porting/port-log/`; read the diff | Table below |
| An effect Crucible ported | Does the Go still match? Params added, removed, loop or guard changed | Re-port the changed behavior with a test; update the `quirks.md` row |
| A Forge bug Crucible reported (`forge-java-defects.md`, `card-script-defects.md`, `upstream-patches.md`) | Did upstream fix it, and how? | Make Go match upstream's fix, remove the Go refusal or workaround, delete the defect row and "Pending" row (DOC-16), keep the PR number in the retired list |
| Card scripts | `rg` the touched card names in `docs/crucible/porting/`, `crucible/internal` tests and fixtures | A test or note naming a changed card needs updating; a script fix of a param Crucible rejects lifts that refusal |
| A new `ApiType`, trigger mode, static mode, replacement event, keyword or valid-property | `git diff` shows it in `ApiType.java`, `TriggerType`, `StaticAbilityMode`, `ReplacementType`, `CardProperty`/`CardPropertyTest` | Row in `game-state.md` "Not ported yet" naming Java site and corpus line count; port it if the user asked |
| `forge-gui/res/lists`, editions, `TypeLists.txt` | Parsed by Crucible loaders | Golden moves; read the diff |
| Only `forge-gui/src`, `forge-ai`, UI, Android, build | Nothing Crucible reads | Ignore, but say so in the PR body |

Then read each Java diff that touches something ported:

| Change | Do |
| --- | --- |
| Ported rule got a new escape hatch (static mode, param, SBA or combat exemption) | Port it in the same commit, with a test. Template: `ignoreLegendRule` in `staticability.go` |
| Unported rule refactored | Row in `game-state.md` "Not ported yet" if missing. Precedent: Shadow-blocking moved to `Mode$ CanBlockIfShadow`; tracking update, not new work |
| Upstream regression or new Forge bug found while reading | PORT-8: stop, name file and line, row in `forge-java-defects.md`, fix on `upstream-pr/<slug>` |

Put the result in the sync PR body: one line per commit group (ported-impact / tracking row added / ignored and why).
