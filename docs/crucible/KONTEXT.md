# Kontext — Session Handoff

- **Status:** Active
- **Parent:** [README.md](README.md)

Where the open-plan loop stands, what waits on the owner, and the working lessons that are not rules yet. Reason: a new
session starts here instead of re-deriving the state from git log. Counts, rows and decisions live in the linked files;
this file only points at them (DOC-11).

---

## State

| Item                     | Where                                                                          | State                                                                         |
| ------------------------ | ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| Smoke counts             | [smoke-probe.md](porting/port-log/smoke-probe.md)                              | Every judged corpus card plays, 0 refused; re-run with `landporter reconcile` |
| Open work                | [00-master-implementation-plan-open.md](00-master-implementation-plan-open.md) | B rows left: `B1`, `B6`; then R rows                                          |
| Engine gaps              | [game-state.md](porting/port-log/game-state.md) "Not ported yet"               | The M6 backlog: silent misses, not refusals                                   |
| Java oddities reproduced | [quirks.md](porting/port-log/quirks.md)                                        | One row per quirk or deviation                                                |
| Java defects             | [forge-java-defects.md](porting/forge-java-defects.md)                         | `Not filed` rows wait for R3                                                  |

## Waiting on the owner

| Row                                                  | Blocked by                                                                    | Reason Claude stops                            |
| ---------------------------------------------------- | ----------------------------------------------------------------------------- | ---------------------------------------------- |
| `B1` Charm `Defined$ You`                            | Publishing branch `upstream-pr/charm-defined-you`                             | Publishing upstream is the owner's (R3)        |
| `B6` Dakkon `PlayLandVariant`                        | A Vanguard variant model: avatar in the Command zone, life and hand modifiers | A new game-variant model needs an ADR (ADRP-4) |
| `GameAction.java:170-172` Animate outliving CR 400.7 | An issue text, not a PR                                                       | Latent, and Takklemaggot depends on it         |

Unconfirmed PORT-8 candidate, JUnit probe first: `Cost.add` merges a plain mana part into the cost's mana and takes
`maxWaterbend` from the incoming part only (`Cost.java:1019`), so a tax merged by `CostAdjustment.adjust` (`:90`,
`:103`) may drop a Waterbend cost's tap (Hama's grant under Thalia). Go keeps the tap.

Unconfirmed PORT-8 candidate, JUnit probe first: a static-granted `Blitz:CardManaCost:<valid>` (Henzie, The Caldaia,
Riveteers Provocateur) reaches `new Cost("CardManaCost")`: `KeywordWithCost` special-cases only the literal `ManaCost`
and `StaticAbilityContinuous` substitutes no `CardManaCost` (only `PumpEffect.java:458` does). Go offers no such cast.

Next: Bestow (43 cards, task row in `game-state.md`). Java keeps the bestowed state into play: a move to the battlefield
keeps the same `Card` object (`GameAction.java:171-172`, `copied = c`), so `animateBestow`'s timestamped type and
`Enchant:Creature` change stays until `unattachFromEntity` reverts it (`Card.java:4003-4004`). Two Go checks decide
whether it needs an ADR: whether a removable type and keyword record survives `Move` from the stack (an Animate made off
the battlefield does, `quirks.md`), and whether an Aura's enchant restriction is read from `K:Enchant` or the
`SP$ Attach` line. Model the cast on `castMutate`, the revert on `Game.Unattach`.

## Working lessons

| Lesson                                                                                                                              | Reason                                                                                                                                               |
| ----------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Confirm every PORT-8 candidate with a throwaway JUnit test on `upstream/master` before porting or fixing                            | Reading the Java alone was wrong for Averna, Exotic Pets, Myr Battlesphere and Takklemaggot: Java was right each time                                |
| In an `AITest` probe, resolve a trigger's ability directly (`Trigger.ensureAbility`, `setTriggeringObject`, `AbilityUtils.resolve`) | A trigger caused by `AbilityUtils.resolve` did not reach the stack through `playUntilStackClear`, and the probe reported a defect that was not there |
| Prefix the pre-push gate chain with `set -o pipefail`                                                                               | `cmd \| tail && next` passes `tail`'s exit code, so a red lint or test run still reaches `git push`                                                  |
| Commit nothing while the background gate run is going                                                                               | `git push origin master` pushes whatever `master` is at the end, including commits the race run never saw                                            |
| Re-pad a table row to its old cell widths after editing a cell                                                                      | Claude-only tables are not reformatted by Prettier, and MD060 fails a misaligned row                                                                 |
| Avoid a bare `X$` token next to a backticked `Y$` in one table cell                                                                 | markdownlint reads the `$...$` pair as math and reports MD038                                                                                        |
| `go test ./... \| grep -v '^ok'` under `set -o pipefail` exits 1 when every package passes                                          | `grep -v` selects no line; read the output, not the status                                                                                           |
| On "no space left on device" in the commit hook, run `go clean -cache`                                                              | The Go build cache grows to the whole disk allowance (28 GB this session)                                                                            |
| Mutation-check each new Go test: revert the fix, see it fail                                                                        | Two tests in this loop passed for the wrong reason until checked                                                                                     |
