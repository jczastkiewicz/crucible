# ADR-0037 — The Battlefield Zone Is Keyed by Controller

- **Status:** Accepted
- **Date:** 2026-10-01
- **Deciders:** Crucible session (M5 rules kernel)

## Context

Java keeps a permanent in the battlefield zone of the player who controls it.
`GameAction.controllerChangeZoneCorrection` (`GameAction.java:994-1033`) moves it there, with `ChangesZone` triggers
suppressed, whenever `Card.getController()` disagrees with the zone's player. It runs after every static-ability pass
(`GameAction.java:1206`), when a one-shot control effect lands (`ControlGainEffect.java:224,258`) and when one ends
(`Game.java:935,963`).

Crucible keeps `Card.Controller()` as a fold over `ControlMod`/`tempControllers` and never moves the card: a stolen
permanent stays in `Zone(Battlefield, owner)` (`Card.ZoneOwner`). About 85 call sites read `g.Zone(Battlefield, pid)` as
"pid's permanents", so each is wrong for a stolen card:

| Site                           | Wrong result                                                 |
| ------------------------------ | ------------------------------------------------------------ |
| `attack.go:99`                 | the thief's creature is never offered as an attacker         |
| `block.go:93`                  | it cannot block for the thief; the owner may still block     |
| `turn.go:348`                  | the thief's untap step skips it; the owner's untaps it       |
| cost and `...All` effect scans | "permanents you control" misses it, owner's scan includes it |

`battlefieldControlledBy` (`amountheads.go:112`) works around it for one amount head. The fixture format lists a card
under its controller (`Owner:` marks a different owner, `load.go:280`), but `dump.go:118` writes it under the zone
owner, so the round trip is lossy. `changeControllerAt` (`gaincontroleffect.go:90`) already applies the sickness,
combat-removal and Ring-bearer consequences of a change, but only for one-shot effects; Layer 2 applies none.

## Decision Drivers

- PORT-7: Java's zone layout is what the oracle compares and what scripts' zone lookups assume.
- A bug class the compiler cannot catch (a new `Zone(Battlefield, pid)` site written without thinking about theft) must
  be removed by construction, not by review.
- GO-2/GO-12: zone lists stay the ordering authority.

## Considered Options

1. **Re-home on control change**, as Java: the zone key follows `Controller()`.
2. A controller-filtering helper (`battlefieldControlledBy`) replacing all ~85 sites. Rejected: 85 edits, every future
   site must remember, the fixture round trip stays lossy, and the layout differs from Java's.
3. Leave stolen permanents unsupported. Rejected: 35 of the 42 real Layer 2 statics (Control Magic) plus every one-shot
   `GainControl` line would keep giving wrong combat and untap results.

## Decision

Option 1. `Zone(Battlefield, pid)` is the permanents `pid` controls. `ControlMod` and `tempControllers` stay the source
of truth; the zone is the index derived from them.

- `Game.correctControllerZones` walks every battlefield card and, where `Controller()` differs from `ZoneOwner`, removes
  it from the old list and appends it to the controller's (Java's `newBattlefield.add`). It keeps `Timestamp` and
  `zoneStamp` (CR 400.7: same object), emits no `ZoneChanged` event and fires no `ChangesZone` trigger.
- It runs right after `applyContinuousControl` in `CheckStateBasedActions` (before the other appliers, which read
  `YouCtrl`), and at the end of `changeControllerAt`. It walks a snapshot, never inside a zone iteration. Stack-zone
  control (`ControlSpell`) is excluded, as in Java.
- The consequences of a controller change (summoning sickness, removal from combat, losing the Ring-bearer) move into
  the correction, so Layer 2 gets them too.
- `dump.go` needs no change: listing by zone key is then listing by controller, and `compareZoneCards` compares
  correctly.
- The `ChangesController` trigger stays in "Not ported yet".

## Consequences

**Good:** every `Zone(Battlefield, pid)` site is right by construction; the fixture round trip is exact; the layout
matches Java; `battlefieldControlledBy` becomes redundant. **Bad:** a card's position in a zone list now changes when
control does, which is observable in trigger and APNAP order (it matches Java). A site that means "permanents pid owns"
must walk every player's zone and filter on `Owner`. **Neutral:** `Card.Owner` and `ZoneOwner` diverge on the
battlefield exactly when control differs.

The implementing PR adds module tests (a stolen creature attacks, blocks and untaps for the thief; control returns when
the Aura leaves; list order) and a `layer2-*` scenario.

## Related

ADR-0009, ADR-0030, ADR-0021, [`game-state.md`](../porting/port-log/game-state.md) "Not ported yet"
