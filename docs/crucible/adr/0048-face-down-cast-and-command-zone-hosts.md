# ADR-0048 — A Spell Cast Face Down Is the Card Turned Face Down Before the Cast; Mask's Replacements Are a Creature Flag

- **Status:** Accepted
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

**Cast.** `castOpts.faceDown` makes `castSpell` turn the card face down in its zone before anything else
(`turnFaceDownForCast`): its definition becomes `faceDownDef(false)` (colorless 2/2, no name, no text, no mana cost, so
mana value 0), as Java does (`Card.java:7476-7478`, `CostAdjustment.java:50-52`; CR 708.2, 708.2a, 708.4: the spell is
turned face down before it goes on the stack, so every choice, cost and effect sees only those characteristics). A cast
that does not happen turns it face up again. The {3} a Morph cast pays is a cost paid, not a mana cost the spell has.
Option 1's flag on the stack object is rejected: every cost, kicker and restriction read between the announcement and
the stack would see the face-up card. **Leaving the stack.** A face-down spell that goes anywhere but the battlefield
(countered, fizzled) is revealed to every player by its owner (CR 708.9). A spell cannot be turned face up (CR 708.7).
**Resolution.** A face-down creature spell enters the battlefield with the definition it already has; no ETB trigger
exists on it (CR 708.3). A copy of a face-down spell copies that definition. **Turning up.** A creature cast by Mask has
no turn-up special action unless the card itself has Morph; only the three replacements flip it (Gatherer ruling,
Illusionary Mask). Its enters-the-battlefield abilities neither trigger on entry nor when it turns up (CR 708.3). The
effect keeps applying after the Mask leaves the battlefield (same ruling) and ends when the creature turns up or leaves.
**Replacements.** Java makes an Effect card in Command with three `Updated` replacements (`AssignDealDamage`,
`DealtDamage`, `Tap`) remembered on the creature. The card has no other observable behaviour, so `Card.illuMask` stands
for it: set when the cast stands, cleared by `turnFaceUp`, read by `maskTurnUp` at the three events (combat and effect
damage sources, damage to a creature, the Taps check). Reason: the engine has no `AssignDealDamage` or `DealtDamage`
replacement events and Mask is the only corpus user of both. **Morph, Megamorph, Disguise.** The keyword's turn-face-up
cost is a separate follow-up that reuses `turnFaceUp`; this ADR only fixes the cast path and the Mask's replacements.
Disguise's face-down form also has ward {2} (CR 702.168), so `faceDownDef` needs a variant for it in that follow-up.

## Consequences

**Good:** one cast path serves Mask now and 231 keyword cards later; a countered face-down spell is revealed as CR 708.9
requires. A copy of a face-down spell copies its face-down definition (CR 707.2: copiable values are modified by
face-down status; CR 707.10: a copy is put on the stack, not cast).

**Bad:** the card's own definition is swapped while it sits in hand, so every reader between the announcement and the
stack must tolerate a face-down card in hand; `castSpell` undoes it on every failed cast.

**Neutral:** the Morph stack text stays unreproduced (the port has no spell text).

## Related

CR 708 ([ancestral.vision](https://ancestral.vision/additional-rules/face-down-spells-and-permanents.html)),
[Illusionary Mask rulings](https://gatherer.wizards.com/ME3/en-us/197/illusionary-mask), ADR-0009, ADR-0017,
[game-state.md](../porting/port-log/game-state.md), open plan row B7
