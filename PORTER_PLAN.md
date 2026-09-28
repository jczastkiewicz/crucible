# Porter plan: LosePerpetual (scoped ADR-0023 slice)

Scratch file for whoever resumes this branch. Deleted in the final commit.

## Assigned

- `LosePerpetual` (2 real lines: Racketeer Boss, Pass the Torch), plus the ADR-0023 slice it needs: `Triggers$` grants
  under `Duration$ Perpetual` on `Animate`/`AnimateAll`.

## Findings so far

- Java `SpellAbility.getTrigger()` (SpellAbility.java:1354-1359) walks `getParent()` to the root: a sub-ability sees the
  root's trigger. No PORT-8 bug there.
- Java `zonesCheck` (TriggerReplacementBase.java:61-65): no `TriggerZones$` = active in any zone. SpellCast trigger on
  the cast card itself fires from the Stack.
- Perpetual survives zone change: `GameAction.java:265-266` `copied.setPerpetual(c)` re-applies `PerpetualAbilities`
  with fresh trigger copies.

## Steps (commit after each)

1. compile: resolve `Triggers$` on `Animate`/`AnimateAll` into compiled trigger SubRefs; regenerate golden AST.
2. engine: per-card granted-trigger overlay (`Card` field, grant ID from `g.timestamp`), Clone copy, survives Move;
   `triggerFaces` accessor; migrate every `range face.Triggers` loop; stamp grant ID on pushed triggered abilities
   (carried down sub chains).
3. engine: `checkSpellCastTriggers` scans the cast card's own triggers on the Stack (no `TriggerZones$` or one naming
   Stack).
4. engine: Animate/AnimateAll `Triggers$` + `Duration$ Perpetual` grant; `LosePerpetual` effect;
   `ConditionDefined$ Remembered` in `isPresentMatches`.
5. Tests + scenarios (Racketeer Boss, Pass the Torch), docs, counts, gates full.
