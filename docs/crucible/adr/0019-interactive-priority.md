# ADR-0019 — Interactive Priority (CR 117)

- **Status:** Accepted
- **Date:** 2026-09-25
- **Deciders:** `mc@archlab.pl`

## Context

`ResolveStack` (`stack.go:47-86`) plays out CR 117 for the one case this port has ever reached: no `PlayerController`
method lets a player respond to anything on the stack, so every priority pass is a pass in succession and the top item
always resolves next, with nothing new arriving on top of it in the meantime (`stack.go:55-63`). ADR-0018 built real
Instant/Sorcery casting on top of that same assumption and explicitly deferred interactive priority to a later ADR
(`0018-instant-sorcery-spell-object.md:31-36`) rather than risk an unlandable ADR bundling both.

This blocks:

- `Play` (330 corpus lines) and `CopySpellAbility` (255 lines) — ADR-0018 unblocked their dominant shapes, but the
  research doc's own table (`effects-play-copyspellability.md:14-18`) is explicit that a real response window is the
  next thing both need past their dominant shape.
- Most of the ~57 real remaining `PlayerController` gaps: `chooseSpellAbilityToPlay`/`playChosenSpellAbility` (the ask
  loop itself), `chooseTargetsFor`/`chooseNewTargetsFor` for a response, `payManaCost` as a response, and everything
  else in `00-master-implementation-plan-in-progress.md` item 29's casting-infra bucket that exists only to serve a real
  priority window.
- `MustBlock`'s own deferred question (`effects-batches.md:696`): whether to trust, re-prompt, or error on a
  controller's answer that turns out to violate a legality check discovered after the fact. This ADR decides that
  question generally (Decision, point 5) — `MustBlock`'s own ADR applies it rather than re-deciding it.

The general CR 608.2b fizzle check (ADR-0018 Decision point 3, narrowed to an Aura's own `Target`) stays out of scope
here too: responses remove the premise ADR-0018 scoped it under ("no way to make a chosen target illegal between casting
and resolving yet"), but a general per-entity re-check (Java's `SpellAbility.canTarget(entity, fizzleCheck=true)`) is
its own unit, not bundled into this ADR (Consequences).

## Decision Drivers

- GO-2: no package-level mutable state — priority state (`pPlayerPriority`, `pFirstPriority` in Java) lives on `*Game`,
  scoped per game, the same as every other turn-structure field `turn.go` already carries.
- GO-8: no `any` — a controller's priority answer is a typed action, not a `map[string]string` or an interface grab bag.
- TEST-1: existing module tests and all 300+ scenario fixtures, `expect.events` included, must stay byte-identical with
  the default (empty-queue) answer — priority is additive, not a behavior change for every game that never queues a
  response.
- PORT-2: no new runtime interpretation — a response is still `Ability{API, Params, ...}` pushed through the existing
  `Registry`, the same as every other cast path.

## Considered Options

1. **One `PlayerController` method returning a typed pass-or-act result, asked in a loop.** Mirrors Java's
   `chooseSpellAbilityToPlay`/`playChosenSpellAbility` pair (`PlayerController.java:278-279`) collapsed into one round
   trip: ask, get back either "pass" or a chosen action, apply it if not a pass, ask the same player again until they
   pass. Matches this port's existing one-question-at-a-time `PlayerController` shape (every other decision method is
   already this shape) and needs no new concept beyond a zero-value-means-pass action struct.
2. **Two methods, split ask/act like Java.** Rejected: Java's split exists because `chooseSpellAbilityToPlay` also
   drives the human GUI's legal-action highlighting, a concern this port has none of (PORT-6, no UI). A single method
   removes a round trip with no loss — `ScriptedController`'s queue already answers "what, if anything, to do" in one
   shot for every other decision point.
3. **Model priority as its own state machine object, separate from `ResolveStack`.** Rejected: Java's `pPlayerPriority`/
   `pFirstPriority`/`givePriorityToPlayer` are three fields on `PhaseHandler`, not a separate type — replicating that as
   fields on `*Game` alongside the stack (`stack.go`, `turn.go`) keeps the same locality every other turn-structure
   field already has (GO-2's own reasoning), and a separate object would need its own way back into `*Game` for the
   stack, SBAs, and triggers it has to drive.

Option 1 chosen.

## Decision

1. **`PlayerController` gains one method**, `TakeAction(g *Game, pid PlayerID) Action` — the "priority" ask, named for
   the CR 117 concept, not Java's method name. `Action` is a typed struct (GO-8): a zero value means pass. `Action`'s
   fields carry exactly what a response can be today — cast a spell (Instant/Sorcery only, `castInstantOrSorcery`'s own
   path) or activate an ability (`ActivateAbility`'s own path) — reusing those two functions' existing signatures rather
   than inventing a third cast path. `ScriptedController.QueueAction`/`TakeAction` follow the existing
   `QueueX`/`ChooseX` pattern (`control.go:499-...`): an empty queue answers pass, the same default every other
   `ScriptedController` method already has.
2. **A priority loop, `Game.passPriority`**, replaces `ResolveStack`'s current "everyone already passed" assumption:
   - Priority starts with the active player at the top of every step/phase that grants it (Decision, point 4) and resets
     to the active player every time the stack resolves (CR 117.3b) — one field, `Game.priorityPlayer`, the same role
     Java's `pFirstPriority` and `pPlayerPriority` both play, kept as one field here: this port has no human GUI to
     drive separately from the pass-counter anchor, so the two Java roles collapse into one without losing the coupling
     Java's `resetPriority()` already ties them to (`MagicStack.java:582`, `PhaseHandler.java:142-144`).
   - Loop: check state-based actions and push waiting triggers in APNAP order (`pushTriggeredAbilities`, already ported)
     — before every ask, not once per phase, same as `checkStateBasedEffects` runs before every
     `chooseSpellAbilityToPlay` call (`PhaseHandler.java:1050-1056`). Then ask `priorityPlayer.TakeAction`. A non-pass
     answer applies it (push or activate), sets `priorityPlayer` back to the acting player (CR 117.3c — the caster keeps
     priority), and loops again without advancing. A pass advances `priorityPlayer` to the next player in turn order;
     once priority has gone around back to whoever it started this round with, either the stack is empty (step ends,
     return to `AdvancePhase`'s caller) or non-empty (`ResolveStack` pops and resolves exactly one item, then the loop
     restarts with priority reset to the active player).
   - No nested loops: a response only pushes and returns to the outer loop — the loop itself is the only place
     `PushAbility`/`ActivateAbility` get called from a priority ask. `CastSpell`'s own public entry point (main-phase,
     sorcery-speed cast) stays a caller of the loop, not a recursive call into it, the same relationship it already has
     to `ResolveStack` today.
3. **`ResolveStack` narrows to "pop and resolve exactly one item"**, called once per full pass-around by `passPriority`,
   instead of looping to empty itself. Existing direct callers (module tests, `actions.go`'s `resolvestack` verb) keep
   working unchanged for the no-response case: looping `ResolveStack` to empty by hand is exactly what those callers
   already do, and stays a valid, narrower way to drive the stack without going through a full priority round — useful
   for a test that wants to resolve without a controller ever being offered a response.
4. **Which steps grant priority** (`Game.givesPriority(phase PhaseType) bool`, ported from `onPhaseBegin`'s own
   per-phase sets, `PhaseHandler.java:240-451`): not `Untap` (CR 502.4). `Cleanup` grants it only when the SBA check or
   a trigger check after it finds something to do (CR 514.3a) — `beginPhase`'s existing Cleanup body (`turn.go`'s own
   header already names Cleanup as one of the four steps with a body) gains that repeat-check, not a blanket "no
   priority in Cleanup" rule. Combat steps with no attackers/no damage to assign (`COMBAT_DECLARE_ATTACKERS` with zero
   attackers, a first-strike-damage step with no first strikers) do not grant it either — this port's own Combat is thin
   enough (`00-master-implementation-plan-in-progress.md` item 29) that this rule is stated now and wired in once each
   of those steps has a real body to guard. Every other step/phase grants it by default.
5. **A controller's chosen action that fails a legality check is an error, not a silent skip or a re-prompt.** The
   oracle (`ScriptedController`, or any future real controller) is never offered an action it cannot legally take — CR
   307.1's own timing check (`Player.canCastSorcery`, `Player.java:2512-2515`) only ever gates what appears in the
   legal-action set a controller chooses from, never re-validates after the fact in Java. This port has no such
   legal-action enumeration yet (`TakeAction` just asks "what, if anything" and trusts the answer, the same stance every
   other `PlayerController` method already documents — `Ability`'s own doc comment, "NOT re-checked ... trust the
   controller's answer"). Trusting a queued action that violates sorcery timing or any other structural legality check
   would let a fixture reach a state CR forbids with nothing catching it, so `passPriority` checks timing legality
   itself before applying a non-pass answer and returns an error if it fails (GO-7: a bad card, or here a bad queued
   action, fails its own game, not the batch) — the one exception to "trust the controller" this port's decision methods
   otherwise hold to, made here because nothing else in this loop can catch it before state changes. This is the general
   rule `MustBlock`'s own ADR (Context) applies rather than re-decides: a controller's answer is trusted up to the
   legality checks this port can cheaply run inline; a check that would require re-deriving the full legal-action set
   (Java's own approach) is deferred, not silently skipped.
6. **No new event.** A pass is not telemetry-worthy on its own (ADR-0013's own volume argument — a pass happens far more
   often than every event this port already emits combined) and every existing `expect.events` fixture file must stay
   byte-identical for the empty-queue default (Decision Drivers). `AbilityActivated`/`AbilityResolved` already cover a
   response's own push/resolve.

**Explicitly out of scope:** mana abilities (CR 605, which do not use the stack and do not pass priority the normal
way); split second; a real `AIController` (M7); `MagicStack.undoStack`; `Play` and `CopySpellAbility` themselves (this
ADR only removes their remaining blocker); the general CR 608.2b fizzle check past an Aura's own `Target`
(Consequences).

## Consequences

**Good:** `Play`, `CopySpellAbility`, and the casting-infra bucket of `PlayerController`'s remaining gaps all unblock.
`ResolveStack`'s existing callers (module tests, `actions.go`'s `resolvestack` verb) keep their current meaning — pop
and resolve one item — rather than needing to learn a new "resolve everything, offering priority along the way" shape; a
caller that wants the full priority round calls `passPriority` instead, additive rather than a breaking rename.

**Bad:** the general CR 608.2b fizzle check becomes load-bearing rather than a documented but currently-unreachable gap
— a response resolving above a targeted spell can now actually remove its target before this port has a general re-check
for it (only the Aura shape has one, ADR-0018 Decision point 3). Named as the next real unit once this lands, not solved
here: Java's own mechanism is a per-entity `canTarget(entity, fizzleCheck=true)` (`SpellAbility.java:1591`), not a
recompute-and-intersect of the candidate list — the shape that already broke `TestRemoveFromGameSpellOnStack` once
(ADR-0018 Decision point 3) and must not be reused for the general case either.

**Neutral:** `Game` gains one field (`priorityPlayer PlayerID`), no new type. `PlayerController` gains one method,
implemented by `ScriptedController` and any future controller the same way every other method already is.

## Related

ADR-0018 (instant/sorcery spell object — this ADR removes the priority gap it deferred), ADR-0013 (event schema — no new
event kind), ADR-0017 (`Registry` dispatch — a response still resolves through it, unchanged),
`effects-play-copyspellability.md`, `effects-batches.md:696` (`MustBlock`'s own deferred question, answered generally
here), [04-adr-process](../guidelines/04-adr-process.md)
