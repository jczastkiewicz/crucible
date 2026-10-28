# ADR-0048 — A Spell Cast Face Down Is a Flag on the Stack Object; Face-Down Triggers Live on a Command-Zone Host

- **Status:** Proposed
- **Date:** 2026-10-28
- **Deciders:** Crucible session (B7)

## Context

Illusionary Mask (`Play | CastFaceDown$ True | ReplaceIlluMask`) is the one refused card that needs a face-down cast.
The same path is what Morph, Megamorph and Disguise need: 231 corpus cards carry one of those keywords. Today they reach
the battlefield face up only; the engine has `facedown.go` (`IsFaceDown`, `turnFaceDown`, `setFaceDownState`, manifest
and cloak) for permanents but no spell that is face down on the stack.

Java: the face-down ability is the Morph one built by `CardFactoryUtil.abilityCastFaceDown` (`:80-120`, cost {3});
`PlayEffect.java:321-324, 458-459, 538-575` casts it, `WithoutManaCost` strips the {3}; `MagicStack.java:263` keeps the
card face down on resolution when the spell was cast face down; the permanent enters as a 2/2 through
`setOriginalStateAsFaceDown` and `moveToPlay`. Stack text keeps "Morph - Creature 2/2" and the {3} description (a PORT-7
quirk). `ReplaceIlluMask` adds an Effect card in the Command zone that remembers the creature and holds three `Updated`
replacements (`AssignDealDamage`, `DealtDamage`, `Tap`), each turning it face up (`SetState | TurnFaceUp`). The effect
leaves when the creature leaves the battlefield or is countered.

## Decision Drivers

- GO-9: identity by `CardID`; the stack object is not a pointer-linked copy of the card.
- PORT-2: no script parsing at run time; the face-down characteristics come from `faceDownDef`.
- The Command-zone Effect host already exists (`effecteffect.go`); a second mechanism would fork it.
- 231 keyword cards benefit, so the cast path is shared, not Mask-specific.

## Considered Options

1. A `faceDown bool` on the stack object (`StackItem`/`castRecord`), set at cast, read at resolution; face-down
   characteristics applied by the existing `setFaceDownState`.
2. Move the card to the stack already face down by flipping the card's own `faceUpDef` in the hand.
3. A separate "morph spell" object type.

## Decision

Option 1.

**Cast.** `castspell.go` gains `castFaceDown`: the spell's characteristics are `faceDownDef(false)` (colorless 2/2, no
name, no abilities, cost {3} replaced by the alternative the cast supplies), not the card's. The flag lives on the stack
object so a copy of the spell, a counter, or a fizzle never touches the card in the hand. **Resolution.** A face-down
creature spell enters through `moveToPlay` with `setFaceDownState`, as manifest does (`changezoneeffect.go:654-667`).
`MagicStack.java:263` is the reference: face down is kept only when the flag is set. **Hosts.** The Mask's Effect card
is an `effecteffect` host in Command whose three replacements are compiled from the script's
`Triggers$`/`ReplacementEffects$` and bound to the remembered `CardID`; it is removed by the same "remembered card left"
check other Effect hosts use (`EffectEffect.java` `ExileOnMoved`). **Morph, Megamorph, Disguise.** The keyword's
turn-face-up cost is a separate follow-up that reuses `turnFaceUp`; this ADR only fixes the cast path and the host.

## Consequences

**Good:** one cast path serves Mask now and 231 keyword cards later; copies and counters behave per CR 708. **Bad:** the
stack object grows a field every cast path must carry (copy, split, adventure). **Neutral:** the {3} description on the
stack stays wrong-looking on purpose (PORT-7).

## Related

ADR-0009, ADR-0017, [game-state.md](../porting/port-log/game-state.md), open plan row B7
