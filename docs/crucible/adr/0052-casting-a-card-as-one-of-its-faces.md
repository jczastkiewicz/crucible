# ADR-0052 — Casting a Card as One of Its Faces

- **Status:** Accepted
- **Date:** 2026-10-31
- **Deciders:** Crucible session (R18b)

## Context

About 530 corpus cards offer a second spell or land on another face: Adventure 151, Split 128 (30 are Rooms), Modal 101,
Prepare 72, Disturb 32, Aftermath 27, Omen 16, Vengeance 1 (`AlternateMode:` and `K:Aftermath`/`K:Disturb`). The engine
cast face 0 only. `CastSpell` was `castFromHand(DoorLeft)`; only a Room could pick a door (`CastRoomDoor`), and `Play`
refused every other layout (`playCastGap`). The smoke probe counted them as playing on their front face.

Java builds one `SpellAbility` per face (`Card.getAllPossibleAbilities`, `Card.java:7524-7575`), tags it with a
`CardStateName`, and `chooseSpellAbilityToPlay` picks among them (`PlayerController.java:303`);
`setSplitStateToPlayAbility` (`Card.java:7484-7504`) switches the card to that state while the spell is on the stack.
Adventure exiles a resolved spell and tracks it with the exiling source (`CardFactoryUtil.java:4075-4101`,
`Card.isOnAdventure`); Omen shuffles on any departure from the stack (`:4102-4116`); a Modal back face stays the
permanent's state.

## Decision Drivers

- The Room cast already swaps `Def` for a one-face view while the spell is on the stack (`castAsDoor`, `doorView`), and
  `Action.Door` is already a face index
- Everything downstream (cost, type, timing, targets, abilities) reads `Def.Faces[0]`; a view makes it read the face
- No new decision method on `PlayerController`: the action already names the face

## Considered Options

1. A face parameter threaded through `castFromHand`, `payCastCost` and every reader
2. A face view in `Def` while the spell is on the stack, as Rooms do (`faceDef` holds the printed card)
3. One synthesised `SpellAbility` per face, as Java

## Decision

Option 2. `CastFace` and `PlayLandFace` swap `Def` for `doorView(printed, face)` before the ordinary cast, and undo the
swap if the cast is declined. `faceDef` holds the printed card; `leaveRoom` restores it when the card leaves the stack
or the battlefield, so a Modal back face is a permanent in its back state until it leaves. Adventure (`onAdventure`),
Omen and Aftermath are handled where the spell leaves the stack, the way Rebound and Buyback are. Rooms keep `roomDef`.

## Consequences

**Good:** no reader changes; the controller names the face through the existing `Action.Door`; copying a face spell
copies the face. **Bad:** the printed card is hidden behind `Def` while cast as a face, so code that wants the printed
card calls `PrintedDef`. **Neutral:** Fuse (17 cards), Disturb and the `Play` effect's face choice stay open
(`game-state.md`).
