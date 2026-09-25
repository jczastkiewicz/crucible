---
name: sync-upstream
description:
  Merge Card-Forge/forge's master into this fork, following docs/crucible/runbooks/upstream-sync.md, then verify the Go
  port still agrees with what moved. Use when asked to sync, update from upstream, or pull in the latest Forge changes.
---

# Sync with Card-Forge/forge

The procedure is `docs/crucible/runbooks/upstream-sync.md` (ADR-0015) — read it first, every time; it is more specific
than "merge and open a PR" and this skill does not restate it. This skill adds what the runbook assumes you already
know: which gate to trust, and how to check the parts nothing gates.

## Do the merge

Runbook's own steps: `git fetch upstream`, branch as `sync/<date>-<short-sha>` off `master`, `git merge upstream/master`,
push, open a PR (`gh pr create`, or the GitHub tool if `gh` is unavailable — open it yourself rather than by API if the
runbook's workflow-trigger warning applies to your setup). **Merge with a merge commit when the PR lands. Never squash,
never rebase** — ADR-0001, and the one irreversible mistake the runbook calls out.

A merge conflict inside `forge-gui/res/cardsfolder/` (or any file upstream owns) is resolved by taking upstream's side
whole, never by hand-editing to keep both — `git checkout --theirs <path>`. A conflict in a file Crucible owns (the six
root configs REV-1 lists) keeps Crucible's side plus whatever upstream added.

## Verify the port, not just the gates

**`gates.sh full`'s auto-skip must see corpus changes, not just `crucible/` changes** (fixed in this repo, but if you
ever see `gates.sh` report "all green" on a sync that touched only `forge-gui/res/cardsfolder/`, distrust it and run
the line below instead). A sync's real gate is the uncached suite CI runs, once, right after the merge — not
`gates.sh full`'s cached version, which is tuned for a single small diff, not hundreds of upstream files:

```bash
cd crucible && go test -race -count=1 ./...
```

Read every failure. Two shapes, and they need different responses:

- **A corpus golden moved with no "unknown"/"unexpected" wording** (a count changed, a new row appeared) — new cards
  added data the parser already handles. Regenerate with that test's own `-update` flag and read the diff before
  committing it (TEST-6) — the runbook's own "When a corpus golden fails" section has the decision tree. The goldens a
  sync can move are not only the eight the runbook table lists: this repo has also seen `internal/carddb`
  (`TestCorpusParses`), `internal/carddb/vocab` (`TestCorpusVocabulary`), `internal/cost` (`TestCorpusCosts`),
  `internal/deck` (`TestCorpusDecklists`), and `internal/carddb/compile` (`TestCorpusAST`, when upstream rewrites an
  existing card's script rather than only adding new ones) move too. Trust the actual `go test` output over any fixed
  list, this one included.
- **A parser rejects a script, or a param apiscan says nothing reads** — a genuine Forge card-script bug (PORT-8), not
  data. Fix it locally (rename/remove the offending line), log it in
  `docs/crucible/porting/upstream-patches.md`'s "Pending upstream fixes" table, and prepare a branch off
  `upstream/master` (not this fork's `master`) with just that one-line
  fix, ready for a real PR against `Card-Forge/forge` — `git checkout -b upstream-pr/<slug> upstream/master`, apply the
  fix, commit, push to this fork, open the PR from there. Don't fold this into the sync PR; it is a separate, upstream
  facing change.

**`apiscan -check -api` has a blind spot: it never verifies `S:` static-ability params.**
`tools/apiscan/scan.go`'s `readEffectParams` only walks `forge-game/.../ability/effects/*.java`'s own `extends` chain —
every static-ability mode's implementing class lives in `forge-game/.../staticability/` instead, so a static-ability
param written under the wrong name (Sanctum Lurker's `Affected$` instead of `ValidCard$` was exactly this) passes every
gate clean. If the sync's diff adds or touches an `S:` line on a card whose mode Crucible has ported (or is about to),
cross-check its params against the Java class by hand.

**Diff `forge-game/src/main/java` and `forge-core/src/main/java` against the port, not just against the corpus** — the
runbook's own step 3 says to skim these; this is how. For a sync from `<old-sha>` to `<new-sha>`:

```bash
git diff <old-sha> <new-sha> --stat -- forge-game/src/main/java forge-core/src/main/java forge-gui/res/lists
```

For every file that touches something already ported (grep its class/method name across
`crucible/internal/engine/*.go` and `docs/crucible/porting/port-log/`), read the actual diff. Two things to look for:

- **A rule Crucible already ports got a new escape hatch.** A new static-ability mode, a new param, a new exemption on
  an SBA or a combat-legality check the Go engine already implements. Port the small addition
  (`ignoreLegendRule`/`ignorePlaneswalkerZeroLoyaltyRule` in `staticability.go` are the template: a `ValidCard` match
  walked over every battlefield permanent) in the same commit, with a test — don't leave the Go port silently
  disagreeing with the oracle on a rule it already claims to implement.
- **A rule Crucible has never ported got refactored, not just renamed.** Log it in `game-state.md`'s "Not ported yet"
  table if it isn't already — CombatUtil's hardcoded Shadow-blocking moving into `Mode$ CanBlockIfShadow` is the
  precedent: Crucible had never ported Shadow-blocking either way, so this is a naming/tracking update, not new work
  to rush through mid-sync.

## Everything else

`javacycles -expect 82`, `tools/metrics`, and differential-parity handling are exactly as the runbook describes — read
it, don't guess.

If a rebase or history rewrite ever leaves a Claude-driven branch diverged from its own remote copy, push to a new
branch name rather than force-pushing: Claude Code's own git-destructive guard blocks a force-push outright, and
routing around a harness denial via another tool or a later turn is against the rules that guard follows, not just this
repo's.
