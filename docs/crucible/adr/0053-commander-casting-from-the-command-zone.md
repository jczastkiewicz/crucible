# ADR-0053 — A Commander Is Cast From the Command Zone Through Java's Commander Effect Card

- **Status:** Proposed
- **Date:** 2026-11-02
- **Deciders:** Crucible session (B14, Liesa)

## Context

ADR-0051 ports the commander designation only and lists the Commander Effect, tax and cast counts as not ported.
`commanderCasts()` (`castrecord.go`) therefore returns 0 for every player. 21 corpus files read a commander cast
(`CommanderCastFromCommandZone`, `wasCastFromCommand`: Liesa, the storm cycle, Study Hall, Opal Palace, Commander's
Insignia, ...) and 146 name `IsCommander`; a commander is never moved to, nor cast from, the Command zone.

Java does all of it through `Player.createCommanderEffect` (`Player.java:3254-3300`), called by `addCommander`
(`:2851`): a `DetachedCardEffect` owned by the player in their Command zone, holding two traits. One is a `Moved`
replacement (`ValidCard$ Card.IsCommander+YouOwn`, `Destination$ Hand,Library`, no `Origin$`, `Secondary$ True`,
`Optional$ True`, `OptionalDecider$ You`) overridden by `ChangeZone | Defined$ ReplacedCard | Destination$ Command`. The
other is a `MayPlay$ True` static on `Card.IsCommander+YouOwn` in the Command zone. `CommanderMoveReplacement$` is read
by nothing in `forge-game`.

The cast count is `Player.commanderCast` (`:190`, `incCommanderCast` `:2908`), incremented as the spell is cast when its
owner casts the commander from the Command zone (`MagicStack.java:385-387`). The tax is two generic mana per count,
added as the first step of `CostAdjustment.adjust` for a spell cast from the Command zone, before any RaiseCost or
ReduceCost static (`CostAdjustment.java:56-61`; Liesa's ReduceCost reduces it).

Two rules sit outside the effect. The CR 903.9a graveyard/exile return is a state-based action
(`GameAction.stateBasedAction_Commander`, `:1853-1866`, flagged on the move at `:499-500, 847-848`) that runs only under
the Commander, Brawl and Planeswalker variants (`:1561-1570`). A merged permanent with a commander component suppresses
the replacement until it is split (`Player.setCommanderReplacementSuppressed`, `:2873-2879`;
`GameAction.java:313-317, 492-505`).

Go's `Moved` replacements today: `movedDestination` (`replacement.go:336-345`) applies the first matching line, a CR 616
simplification with no choice among several; it is reached only for moves off the battlefield (`zonemove.go:56-61`) and
to the graveyard; its override may move to Exile, Hand or Library only, and a line with any other param, `Optional$`
included, is not applied.

## Decision Drivers

- Effect cards already live in the Command zone (`effectCardDef`, ADR-0048), and `MayPlay$` statics there already grant
  casts (`castOptions`).
- The rule should sit where Java has it, so that when the `Moved` path gains the CR 616.1 choice, this replacement takes
  part in it with no further change.
- PORT-2: scripts are compiled once at load, never interpreted per cast.
- GO-2 / GO-9: no package state; the effect is a `CardID` in the game's arena.

## Considered Options

1. Build the effect card as Java does: a Command-zone card whose two traits are compiled once from the Java lines, and
   extend the `Moved` path to carry them.
2. Hard-code the rule in the engine: a check in the zone-move code and a Command-zone case in `castOptions`. Rejected: a
   second mechanism beside `Moved` and `MayPlay$`, with its own ordering against other replacements, removed again once
   the `Moved` path gets the CR 616.1 choice.
3. Leave the commander casts unrecorded. Rejected: You're in Command already makes commanders in a constructed game
   (ADR-0051); once such a creature would go to the hand or library, Java puts it in the Command zone and lets it be
   cast from there with tax. Go would diverge on every such game, and Liesa's two statics stay inert.

## Decision

Option 1. `addCommander` (`commander.go`) creates the owner's Commander Effect once: a card in the Command zone whose
definition is a synthetic card (the two lines above) run through `compile.Compile` once when the DB loads and held
immutable by it, not per game. The Oathbreaker, Tiny Leaders and Planeswalker variants of those lines are not ported (no
variant model exists).

The `Moved` path is extended for it: `movedDestination` is reached for every move to the hand or library, from any
origin; an override may move to the Command zone; `Optional$` with `OptionalDecider$ You` asks the effect's controller
through the existing optional-replacement confirmation. The first-match rule stays, logged in `quirks.md`.

`Player.commanderCasts` is a count per commander `CardID`, read by key and summed, never iterated for order.
`recordSpellCast` increments it when the caster is the owner and cast a commander from the Command zone, before the
SpellCast triggers (`castspell.go` already calls it first); `commanderCasts()` and `Count$CommanderCastFromCommandZone`
read it. The tax is the first cost-adjustment step for such a cast, two generic per cast the activator recorded for that
commander (Java reads `activator.getCommanderCast(host)`), before `costStatics` applies RaiseCost and ReduceCost. A
commander entering the Command zone is placed first (`Zone.java:106-114`).

Not ported: commander damage and the 21-damage loss, color identity, deck setup and companion/partner rules, the
Commander variants and with them the 903.9a state-based action (Java runs it only under a variant, so a variant-less
game agrees), and a copy inheriting the designation. The merged-commander suppression is not ported: the replacement
applies to the move itself, a deviation logged in `quirks.md`. A fixture or test makes a commander through
`addCommander`; the fixture `|IsCommander` option gets applied through it.

## Consequences

**Good:** a commander can be cast from the Command zone with tax, returns there instead of the hand or library, and the
21 readers answer from real state; the rule lives in the same `Moved` and `MayPlay$` paths Java uses. **Bad:** the
`Moved` path now runs for every move to the hand or library, including a draw, so its cost rises for every such move.
Several `Moved` replacements on one move still resolve first-match, not by the affected player's choice. Every
`addCommander` adds a card to the Command zone, so tests and fixtures that count Command-zone cards see one more once a
commander exists. The gauntlet's decks are 60-card constructed with no commander setup, so in batch runs only You're in
Command reaches this code. **Neutral:** the compiled effect definition is shared by all games, like every card
definition.

## Related

ADR-0006, ADR-0048, ADR-0051, `porting/port-log/game-state.md`, `porting/port-log/quirks.md`
