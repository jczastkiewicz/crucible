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

Next, in order:

1. Keywords with no `keyword.Expand` template and no engine reader are lines with no trait behind them, as Morph,
   Vanishing and Myriad were. No coverage command exists (ADR-0038 names one). A text search for heads quoted nowhere
   outside `keyword/defined.go`, by card count: Choose a Background 32, Doctor's companion 27, Banding 24, Assist 16
   (Java's AI never assists, `PlayerControllerAi.java:395-410`). Banding is a combat model: attacking bands
   (`AttackingBand.java`, `Combat.java:267-292`), blocking one member blocks the band (`Combat.java:402-404`), and the
   banding side assigns the other's combat damage (`Combat.java:764-781`, `:836-841`); Java's AI only joins banding
   creatures onto an attacker's band (`AiAttackController.java:504-579`). Spree needs no reader: `charmModeCost` adds
   the chosen modes' `ModeCost$`. Some are read under another spelling (Reach, 449, is); verify each against the engine
   before porting.
2. Blitz with a third field (`ValidAfterStack`, `SpellAbility.java:2648`): Riveteers Provocateur needs only this. Henzie
   and The Caldaia grant Blitz with `AffectedZone$ Stack`, which Java offers from the hand through a stack copy of the
   card (`GameActionUtil.java:250-279`). Not a Java defect: `StaticAbilityContinuous.java:756` substitutes
   `CardManaCost`.
3. The keyword-cast row's leftovers (Warp with X, `CastSa` and `warped`, a copy of a bestowed spell, Summitfest's hand
   and library zones).

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
