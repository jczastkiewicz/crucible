# ADR-0050 — Specialize Is a Persistent Face Swap on the Card

- **Status:** Accepted
- **Date:** 2026-10-28
- **Deciders:** Crucible session (B12)

## Context

Specialize (Alchemy) lets a card in hand or graveyard turn into one of five coloured faces for good. The corpus has 19
`K:Specialize:<cost>` cards (`forge-gui/res/cardsfolder`), 47 `Mode$ Specializes` trigger lines (one per card face, 47
files) and no scripted `Mode$ Specialize`: the ability is only synthesised. The loader already reads the five faces
(`carddb.FaceSpecializeWhite..Green` = indices 2-6, `SPECIALIZE:<COLOUR>` blocks). The engine refuses
`SetState Mode$ Specialize` (`setstateeffect.go`).

Java: `K:Specialize` expands to
`AB$ SetState | Cost$ <cost> ChooseColor<1> Discard<1/Card.ChosenColor; Card.AssociatedWithChosenColor> | Mode$ Specialize | SorcerySpeed$ True`
(`CardFactoryUtil.java:3507-3519`); `AssociatedWithChosenColor` is the basic land type of the colour
(`CardStateProperty.java:63-78`). `SetStateEffect` allows `Specialize` in any zone, maps the chosen colour to state
`SpecializeW..G` (`Card.java:771-781`), sets `specialized` (`Card.java:238`), re-registers triggers and fires
`Specializes` for the card (`SetStateEffect.java:172-215`, `TriggerSpecializes.java:46-64`, only `ValidCard$` is
tested). The chosen colour is transient (`host.setChosenColors(null)` after the change). Nothing resets the face on a
zone change; the flag is carried into the new object (`GameAction.java:262-263`, `CardCopyService.java:83`), so the face
survives hand, graveyard, exile and battlefield.

The engine already swaps the whole `Def` for a transformed card (`frontDef`, `setstateeffect.go`), but `turnFrontFaceUp`
undoes that when the card leaves the battlefield (CR 711.8).

## Decision Drivers

- GO-9, PORT-2: identity by `CardID`; the face is a compiled `*compile.Card`, nothing parsed at run time.
- The swap must outlive zone changes, which `frontDef` does not.
- Fail closed (GO-7) for every interaction not modelled.

## Considered Options

1. A new `Card.specializedFrom *compile.Card` (the unspecialised definition; nil means not specialised). `Def` becomes a
   single-face definition built from the chosen face, as `transform` builds the back face.
2. Reuse `frontDef`. Rejected: the leave-battlefield path restores the front face, which is wrong for Specialize.
3. A Layer 1 `copyEffect` that never ends. Rejected: copy effects end on leaving the battlefield (`endCopiesOnLeave`),
   and a specialised card is not a copy, so Clone and copy-of-copy rules would see one.
4. A per-card face index consulted through an accessor. Rejected: `Def` is a field read at hundreds of sites; an
   accessor is a rewrite of the engine for one mechanic.

## Decision

Option 1.

**State.** `specializedFrom` is set by the change and never cleared by a move; `Def` keeps the face. A token or copy of
a specialised permanent copies `Def` as it does today. A card with `specializedFrom != nil` refuses a second Specialize
(Java `canSpecialize`, `Card.java:6811`).

**The change.** `SetState Mode$ Specialize` takes the colour from the source's chosen colour (`Memory.ChosenColors`,
cleared afterwards), builds `Def` from `Faces[2+i]` (name, `SplitSpecialize`), takes a new timestamp as `transform`
does, and fires `Specializes` with `ValidCard$` matching. The effect works in any zone. A colour whose face is empty, or
a card with no Specialize faces, is an error.

**Cost.** `ChooseColor<1>` is already a cost part; `Discard<1/Card.ChosenColor;Card.AssociatedWithChosenColor>` needs
the `AssociatedWithChosenColor` valid property (the basic land type of the chosen colour; false with no colour). The
keyword expands in `internal/keyword` like Mutate. Activation is sorcery speed from the hand; Karlach and its kin add
the graveyard through `AdditionalActivationZone$` (the Java site that defaults the hand zone is traced when porting).

**Triggers.** Triggers are scanned from `Def`, so the new face's `Specializes` and graveyard triggers (Karlach) work
with no registration step.

**Fail closed.** Specialize on a card face down, under a copy effect, or transformed errors; a specialised card
transforming, flipping or turning face down errors rather than guess (no corpus card does it).

## Consequences

**Good:** every id that points at the card stays valid, the layer pass and zone-change code are untouched, and a card
specialised in hand keeps its face in the graveyard, as the Karlach graveyard trigger needs.

**Bad:** `Def` is replaced by a synthesised definition per specialisation, one allocation per event. Code that wants the
printed front face of a specialised card must read `specializedFrom`, and a reader that forgets will see the face.

**Neutral:** Forge's `SetStateEffect.java:31` reads `sa.getParam("Mode")` without a null guard in `getStackDescription`;
the logged defect (#12377) is unrelated to this decision and unchanged.

## Related

ADR-0047, ADR-0048, `docs/crucible/00-master-implementation-plan-open.md` row B12,
[04-adr-process](../guidelines/04-adr-process.md)
