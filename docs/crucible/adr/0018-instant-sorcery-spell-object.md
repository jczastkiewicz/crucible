# ADR-0018 — Instant/Sorcery Spells as Stack Objects, With Stack-Item Identity

- **Status:** Accepted
- **Date:** 2026-09-25
- **Deciders:** `mc@archlab.pl`

## Context

`CastSpell` (`castspell.go:33`) casts only two shapes: a non-Aura permanent (`castableAsPermanent`) and an Aura
(`castAura`). Neither leaves anything on the stack to resolve into a script effect — `permanentEffect`/`attachEffect`
(`castspell.go:173-205`) both just `Move` the source card straight to the battlefield. An Instant or Sorcery has no cast
path at all: `CastSpell` returns `false` for one, the same as any other declined-by-the-rules case.

This blocks two of M6's largest remaining APIs by corpus lines, already researched and deferred (PORT-8: no Forge bug
found, this is a genuine engine gap):

- `Play` (330 lines) — `docs/crucible/porting/port-log/game-state/effects-play-copyspellability.md:23-41`
- `CopySpellAbility` (255 lines) — same file, lines 43-58

It also blocks 29 of the ~57 real remaining gaps in `PlayerController`
(`docs/crucible/00-master-implementation-plan-in-progress.md` item 29's own "Not ported yet" table, `PlayerController`'s
110-vs-46 row): `chooseSpellAbilityToPlay`, `chooseTargetsFor`, `chooseNewTargetsFor`, `chooseModeForAbility`,
cost-payment hooks and more all exist only to serve casting a real spell, which this port cannot do yet.

The research doc's own step 4 (`effects-play-copyspellability.md:75-76`) is explicit: landing this changes
`ResolveStack`'s documented contract ("no `PlayerController` method lets a player respond to anything on the stack",
`stack.go:52`) and `Ability`'s documented contract ("Nil for an ability naming no `ValidTgts$`", `ability.go:69` —
`Ability` gains a new identity field here). Both are cross-cutting contracts (ADRP-1) — hence this ADR, before any
implementing commit (ADRP-4).

**Explicitly out of scope**, deferred to a later ADR: interactive priority (players responding to what is already on the
stack). The research doc's own table (`effects-play-copyspellability.md:14-18`) shows neither `Play` nor
`CopySpellAbility`'s dominant shape needs it — `Play` casts during resolution with timing bypassed, and
`CopySpellAbility`'s dominant `Defined$ TriggeredSpellAbility` shape sits above the spell in an all-pass `ResolveStack`.
Bundling priority into this decision risks an unlandable ADR (ADRP-5's one-page target) for a need neither target API
has yet.

## Decision Drivers

- PORT-2: compile once, no runtime re-interpretation — a spell's effect is still `Ability{API, Params, ...}` pushed
  through the existing `Registry`, not a new interpretation path.
- GO-9: identity by ID, never pointer — a stack item needs an ID as much as a `Card` does, for the same reason.
- GO-7: a bad card fails its own game, not the batch — the fizzle check and Stack→Graveyard move must not panic.
- Unblocks `Play`, `CopySpellAbility`, `Discover`'s rejected branch (`discovereffect.go`),
  `ChangeZoneEffect.java:1556`'s cast, `ReplaceGraveyard$`, flashback, and `Game.MayPlayFromExile` consumers (Airbend,
  Heist grants) — six real callers already blocked on exactly this, not a speculative generalization (the "no premature
  abstraction" rule cuts the other way once six callers exist).

## Considered Options

1. **New `StackItem` type wrapping `Ability`.** Rejected: duplicates every field `Ability` already carries (`API`,
   `Source`, `Controller`, `Targets`, ...) for the sole benefit of the identity field, and forces every existing
   `Registry.Resolve` caller to unwrap it. `Ability` is already the stack's element type (`stack.go:16`); splitting it
   in two means two things to keep in sync.
2. **Reuse `Ability`, add a `StackItemID` field set by `PushAbility`.** `PushAbility` (`stack.go:23`) is already the
   single place every stack push goes through (its own doc comment: "a caller pushing one ability at a time... calls
   this directly"). Adding a monotonic counter there costs one field and one increment, and every existing caller
   (triggers, the two permanent-cast paths) gets an ID for free without changing its own call shape.
3. **No identity; carry the triggering `Ability` by value on the `SpellCast` event instead.** Rejected for
   `CopySpellAbility`'s `Defined$ TriggeredSpellAbility` shape: a by-value copy cannot be the same object `Game.Clone`
   (`cloneeffect.go`) later mutates a copy of, and `ChangeTargets`/targeting-a-spell need to name a _specific_ stack
   entry among possibly several, which a value carried on one event cannot do once more than one spell is ever cast in a
   turn.

Option 2 chosen.

## Decision

1. **`Ability` gains `ID StackItemID`** (a `uint32` arena-style handle, `GO-9`), set by `PushAbility` from a new
   `Game.nextStackItemID` counter — the same shape `CardID` already has, scoped to `*Game` the same way. Never reused
   within a game, so a copy or a resolved-and-gone item is never confused with a later, unrelated one.
2. **`CastSpell` gains a third branch**, for a card that is neither `castableAsPermanent` nor an Aura and carries a real
   `A:SP$` ability line (Instant/Sorcery's own spell ability, `Def.Faces[0].Abilities`): choose modes if modal, resolve
   targets via the existing `resolveTargets` (`targeting.go:54` — its own doc comment already names this as the future
   caller), pay or decline the cost, `PushAbility` the spell's own `Ability{API, Params, Amounts, Targets, ID}`, fire
   `SpellCast` and `checkSpellCastTriggers`/`checkBecomesTargetTriggers` — the same four steps `castAura` already runs,
   reordered per CR 601.2c (choose modes and targets before paying).
3. **`ResolveStack` gains a fizzle check and a post-resolution move.** Before dispatch: CR 608.2b — if the popped
   ability names a target (`Ability.Target`/`Targets`) no longer legal, skip `Registry.Resolve` for it (the same
   "declined by the rules" contract `resolveTargets` already uses for CR 603.3c) but still run the post-resolution move
   below, since CR 608.2b's own fizzled spell still leaves the stack into the graveyard. After dispatch (fizzled or
   resolved): if the ability's `Source` card is still in the `Stack` zone — `permanentEffect`/ `attachEffect` already
   moved their own source away, so this is a no-op for both existing shapes — move it to its owner's graveyard through
   the existing replacement pipeline (`checkMovedReplacement`), covering `ReplaceGraveyard$` for free since that already
   hooks the same `Move` path every other zone change uses.
4. **`ResolveStack`'s doc comment is corrected**, not the mechanism: "no `PlayerController` method lets a player respond
   to anything on the stack" still holds after this ADR — casting an Instant/Sorcery does not add a response window,
   only a second thing that can be _on_ the stack. Interactive priority stays a documented gap for a later ADR (see
   Context).

## Consequences

**Good:** `Play` and `CopySpellAbility` unblock immediately once this lands — both were fully researched and only
waiting on exactly this (`effects-play-copyspellability.md`). `CopySpellAbility`'s `Defined$ TriggeredSpellAbility`
shape (164 of 255 lines) reads `ID` off the `SpellCast` event's ability, once the event carries it. Six other blocked
callers (Context) unblock too, without a second design pass. The stack-item identity is cheap: one field, one counter,
no new type, no change to any existing `Ability` caller's signature.

**Bad:** `Ability` grows a field every existing literal `Ability{...}` construction site does not set (defaults to zero,
which `PushAbility` overwrites) — a small, one-time textual diff, not a semantic one. `Game.Clone` (`cloneeffect.go`)
needs to decide whether a cloned permanent's own stack-item history (if any) carries the original `ID` or gets a fresh
one; this ADR does not resolve that — it is `CopySpellAbility`'s own implementing PR's job, scoped to the one shape that
needs it (Layer 1 permanent-spell copies, `effects-play-copyspellability.md:55`).

**Neutral:** the fizzle check and post-resolution move both run unconditionally in `ResolveStack`'s loop, on every
ability, not just spells — an activated or triggered ability's `Source` is never in the `Stack` zone when this runs
(nothing moves a permanent's or a triggered ability's own source there), so the move is a zone-check no-op for every
non-spell caller, at the cost of one `Card.Zone` read per resolution.

## Related

ADR-0008 (dispatch, superseded in placement only by ADR-0017), ADR-0013 (event schema — `SpellCast` gains no new field;
`ID` is read off the pushed `Ability`, not the event), `effects-play-copyspellability.md`,
[04-adr-process](../guidelines/04-adr-process.md)
