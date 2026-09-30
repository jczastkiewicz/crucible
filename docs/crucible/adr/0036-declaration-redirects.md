# ADR-0036 — Declaration Redirects: Who Declares Is Rules State, Not a Controller Parameter

- **Status:** Accepted
- **Date:** 2026-09-30
- **Deciders:** Crucible session (M5 rules kernel)

## Context

`DeclaresAttackers$` / `DeclaresBlockers$` (Odric, Master Tactician's `S:` line; Invasion Plans, Melee, Berserker's
Frenzy, Brutal Hordechief, Master Warcraft as Effect SVars — 6 cards) hand a combat declaration to another player. Java:

- `StaticAbilityContinuous.java:555-565` puts `getDefinedPlayers(...).getFirst()` into the affected player's
  timestamp-keyed map (`Player.java:4045-4066`, newest timestamp wins); `StaticEffect.java:204-205` removes the entry
  when the effect ends.
- `PhaseHandler.java:535` asks `whoDeclares.getController()` for attackers; `:662-671` does the same for blockers, and
  passes `whoDeclaresBlockers` as the `DeclareBlocker` replacement's `Player` (Camouflage's `Defined$ ReplacedPlayer`).

Crucible skips both params silently (`continuous.go` `applyOneContinuousRules`), so a card naming them resolves as if it
had none, and `Defined$ ReplacedPlayer` always resolves to the defender (ADR-0035 Context 2). Every `PlayerController`
decision method already takes the acting player as `decider`, and ADR-0030 fixed the principle: a redirect changes whose
brain answers, never who acts.

## Decision Drivers

- ADR-0030: `PlayerID` stays the acting player throughout.
- `applyContinuousRules` rebuilds every player's `RulesMod` from scratch each pass, so an effect's end needs no removal
  code (GO-2: no second store to keep in sync).
- PORT-8: a param that never reaches its effect is a bug, not a skip.

## Considered Options

State:

1. **Two `PlayerID` fields on `RulesEffect`**, set from the first defined player, newest `Timestamp` wins.
2. A timestamp-keyed map per player with add and remove, as Java. Rejected: the rebuild pass already re-derives the
   value, so a map needs an explicit removal hook that Crucible does not otherwise have.
3. Reuse ADR-0030's `controlledBy` grants. Rejected: those are one-shot scheduled grants that redirect every decision
   (CR 800.4b); a declaration redirect is a continuous static that covers one declaration.

Surface:

1. **Queries `Game.AttackDeclarer(pid)` / `Game.BlockDeclarer(pid)`**, returning `pid` when nothing redirects — the same
   shape as `Game.ControllingPlayer`, for a harness that routes a different brain per seat.
2. A `declarer` parameter on `PlayerController.DeclareCombatAttackers` / `DeclareCombatBlockers`. Rejected: every
   controller and fixture changes signature for a redirect no scripted test needs; `decider` already names the seat.

## Decision

State option 1, surface option 1. `rulesEffect` reads both params through `Defined$`-style player resolution (`You`,
`AttackingPlayer`, ...); `DeclareCombatAttackers` and `DeclareCombatBlockers` keep asking with `decider` unchanged. The
declarer is read by Camouflage: `ReplacedPlayer` resolves to `BlockDeclarer(defender)`, `ReplacedDefendingPlayer` stays
the defender. `Defined$ AttackingPlayer` (the combat's attacking player, an error outside combat, GO-7) is added to
resolve Odric's line.

## Consequences

**Good:** six cards stop resolving as if the param were absent; Camouflage's declarer is right; no interface break.
**Bad:** a harness that wants the redirect must call the query itself — the engine does not reroute for it. **Neutral:**
attack targets, exert and enlist stay with the declarer's seat, as one declaration.

## Related

ADR-0030, ADR-0035, ADR-0024, [`game-state.md`](../porting/port-log/game-state.md) "Not ported yet"
