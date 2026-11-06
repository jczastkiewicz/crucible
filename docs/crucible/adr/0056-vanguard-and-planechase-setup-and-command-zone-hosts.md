# ADR-0056 — Vanguard and Planechase Setup; Avatars, Planes and Phenomena Are Command-Zone Hosts

- **Status:** Accepted
- **Date:** 2026-11-05
- **Deciders:** Crucible session (Vanguard and Planechase batch)

## Context

ADR-0029 ports the Planechase effects (planeswalk, the planar die, chaos) behind `planechaseActive`, set once a card
enters a `PlanarDeck`; ADR-0055 adds `Game.variants` with Commander. Neither variant has a game setup, and a card in the
Command zone that is not an effect card or a scheme is not a host (`isCommandTraitHost`, `continuouslayers.go`), so the
106 Vanguard avatars and 173 planes and phenomena sit there inert apart from their triggers. Corpus: avatars carry 35
`EffectZone$ Command` statics, 49 `TriggerZones$ Command` triggers and 40 `ActivationZone$ Command` abilities; planes 58
statics and 273 triggers. Java:

| Rule                                                                                                                                                                                                                                                                                                                                                                     | Java                                                                                |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| A static functions from the zones its `EffectZone$` names, else only in play; a trigger from its `TriggerZones$`                                                                                                                                                                                                                                                         | `StaticAbility.java:253-254, 338-352`, `Trigger.java:98-101`                        |
| An ability activates from its `ActivationZone$`                                                                                                                                                                                                                                                                                                                          | `SpellAbilityRestriction.java:99-100`                                               |
| `HandLifeModifier:+H/+L` (CardRules deltas) is added to starting hand and life; starting hand is also the max hand size                                                                                                                                                                                                                                                  | `CardRules.java:480-486`, `RegisteredPlayer.java:207-213`, `Game.java:375-381`      |
| The Avatar section goes to the Command zone, after the library, before the shuffle; no rule reads `hasAppliedVariant(Vanguard)`                                                                                                                                                                                                                                          | `Player.java:2978-2985`, `Match.java:345-353`                                       |
| The Planes section becomes the planar deck, shuffled before the library                                                                                                                                                                                                                                                                                                  | `Player.java:3006-3012`                                                             |
| After the mulligans the starting player turns the top plane face up (a phenomenon goes to the bottom first), no trigger                                                                                                                                                                                                                                                  | `GameAction.java:2377-2381`, `Player.java:2736-2750`                                |
| Each turn change gives the active plane to the new turn's player                                                                                                                                                                                                                                                                                                         | `PhaseHandler.java:858-862`                                                         |
| Each player gets a "Planar Dice" effect card: the planeswalk trigger and the special action `ST$ RollPlanarDice \| Cost$ X \| SorcerySpeed$ True \| Activator$ Player \| SpecialAction$ True \| ActivationZone$ Command`, X the times the special action was taken this turn (`PhaseHandler`), counted as it resolves; a roll an effect causes does not count (CR 901.9) | `Player.java:3306-3331`, `RollPlanarDiceEffect.java:28-29`, `PhaseHandler.java:527` |

## Decision Drivers

- Batch runs of Vanguard or Planechase decks play the real game only once the avatar, the planes and the die work.
- One host rule for every Command-zone card type, as Java has one zone check, not one per type.
- PORT-2: the Planar Dice card is compiled once per DB, like the Commander Effect (ADR-0053).

## Considered Options

1. Make avatars, planes and phenomena trait hosts and let each line's own zone param decide, as Java does; add the setup
   steps and the special action to the Planar Dice card.
2. A Vanguard- and plane-specific pass that applies their statics. Rejected: a second static path beside the layer
   system, with its own ordering against every other static.
3. Leave the setup to the fixture format. Rejected: `sim` builds games from decklists, and a decklist's Avatar and
   Planes sections would still be ignored.

## Decision

Option 1. `isCommandTraitHost` admits Vanguard, Plane and Phenomenon cards; their statics apply only with
`EffectZone$ Command` (the existing `layerHostZoneActive`), and an `ActivationZone$ Command` ability activates for the
card's controller, or for whoever `Activator$` admits. The planar deck is not the Command zone, so only the face-up
plane functions.

`compile.Card` carries `HandDelta`/`LifeDelta` from `HandLifeModifier`. `Player.StartingHandSize` replaces the
`startingHandSize` constant and is the opening hand, the mulligan draw and the base maximum hand size. `Game.SeatAvatar`
puts an avatar in its owner's Command zone and adds its deltas. `Game.SeatPlanarDeck` puts the planes in the planar deck
and shuffles it; `Game.InitPlanechase`, called after the mulligans, turns the starting player's first plane face up and
gives every player their Planar Dice card. `Variant` gains `VariantVanguard` and `VariantPlanechase` (GameRules); rules
code keeps reading `planechaseActive` (ADR-0029), which is what Java's effects test. The special action increments
`Game.planarDiceSpecialActions`, read by `Count$PlanarDiceSpecialActionThisTurn` and reset each turn. Under the
Planechase variant each turn change hands the active plane to the turn's player: its controller, and with it the Command
zone it sits in.

`sim -variant` takes a comma list (`commander`, `vanguard`, `planechase`) and seats each section under its variant.

Not ported: Archenemy setup, concurrent planes (ADR-0029), the planar die replacements, a planar deck of phenomena only
(Java loops forever, `Player.java:2736-2745`, a Forge defect).

## Consequences

**Good:** an avatar's statics, triggers and abilities work from the Command zone, the planes do, the first plane appears
and players roll the die, so Vanguard and Planechase decks play their format. **Bad:** `traitHosts` now returns these
cards, so every walk over it (triggers, replacements, statics) visits them; a line with no zone param that Java treats
as in-play-only must keep failing the zone check there, which the existing checks do. Whether the die is rolled is the
controller's choice; the probe controller decides until M7. **Neutral:** Commander-only and constructed games are
unchanged.

## Related

ADR-0029, ADR-0048, ADR-0053, ADR-0055, `porting/port-log/game-state.md`, `porting/port-log/quirks.md`
