# ADR-0055 — The Commander Variant Is a Flag on the Game: Commander Damage, Its Loss and the 903.9a Return

- **Status:** Accepted
- **Date:** 2026-11-04
- **Deciders:** Crucible session (Commander format batch)

## Context

ADR-0051 and ADR-0053 port the commander designation, the Commander Effect card, tax and cast counts, and leave out
commander damage, the 21-damage loss, the CR 903.9a graveyard/exile return and deck setup, because no variant model
exists. Java keeps the variant on the match: `GameRules.appliedVariants`, an `EnumSet<GameType>` (`GameRules.java:19`,
`addAppliedVariant` `:116`, `hasAppliedVariant` `:120`, `hasCommander` `:124-128`). The rules sites that read it:

| Rule                                                                                                                                                                                             | Java                                                     | Gate                                         |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------- | -------------------------------------------- |
| Commander damage: combat damage a commander deals to a player, after prevention, keyed by the real commander (the card, its meld partner, else the first merged component with the flag)         | `Player.java:690-698`, `Card.java:7446-7476`             | none, except Oathbreaker, TinyLeaders, Brawl |
| The 21-damage loss, after the empty-library, life and poison checks, through `loseConditionMet`                                                                                                  | `Player.java:2077-2083`                                  | Commander                                    |
| CR 903.9a: a real commander moved since the last check, now in a graveyard or exile, goes to the Command zone if its owner agrees (`confirmAction`), only when nothing else happened in the pass | `GameAction.java:499-500, 847-848, 1561-1570, 1853-1866` | Commander, Brawl, Planeswalker               |
| Starting life 40 (+20 over 20)                                                                                                                                                                   | `RegisteredPlayer.java:135-157`, `Game.java:375-378`     | Commander                                    |
| The deck's Commander section goes to the Command zone and is designated, after the library is built, before the opening shuffle                                                                  | `Match.java:344-353`, `Player.java:3016-3025`            | any commander variant                        |
| `AlterAttribute` Commander applies the Commander variant when the game has none                                                                                                                  | `AlterAttributeEffect.java:119-124`                      | none                                         |

`resetCommanderStats` clears the damage on a restart (`RestartGameEffect.java:71`). A two-player Commander game has no
free mulligan (`MulliganService.java:37`). Color identity is not enforced in play (`CostAddMana.java:87` is commented
out).

## Decision Drivers

- Batch runs of Commander decks are a product use; a commander that dies today stays in the graveyard and nobody loses
  to commander damage, so every such run measures the wrong game.
- Each rule should sit where Java has it (damage at the damage site, the loss with the other losses, 903.9a with the
  SBA).
- GO-2: the variant is per game, not package state. Covergate's 90% floor: a gate no test reaches is dead code.

## Considered Options

1. A `Variant` bit set on `Game`, Commander the only value; each site checks it as Java does.
2. All five commander-like `GameType` values now. Rejected: no deck, fixture or card reaches Brawl, Oathbreaker,
   TinyLeaders or Planeswalker (their Command-zone replacements and life differ, `Player.java:3281-3295`), so their
   branches would be untested code that guesses at variants nothing plays.
3. Infer the variant from the decks (a Commander section means Commander). Rejected: Java takes the variant from the
   match and ignores the section without one; a deck file must not change the rules for both seats.

## Decision

Option 1. `engine.Variant` is a bit set with `VariantCommander`; `Game.HasVariant` reads it. `Game.SetupVariant` is the
setup call (before the opening hands): it applies the variant and sets every seat's starting life and life to the
variant's (40). `AlterAttribute` Commander applies the variant without touching life, as Java does. A Subgame shares the
main game's variants and, with no `StartingLife$`, starts each seat at its own starting life (`SubgameEffect.java:38`,
`Game.java:375-378`). Each player carries `StartingLife` (`Player.startingLife`), which
`StartingLife`/`YourStartingLife` read, a Subgame's `StartingLife$` sets and RestartGame restores.

`Game.SeatCommander` puts a card in the owner's Command zone and designates it (`addCommander`), so the Commander Effect
follows it as in Java. `Player.commanderDamage` is an ordered list of (real commander, amount), added to in
`dealPlayerDamage` for combat damage after prevention and replacement, copied with the game and cleared by RestartGame.
The loss is a fourth case after poison in the SBA, reason `CommanderDamage`. `Game.Move` marks a real commander on every
move (Java's flag at `:847-848`, which `moveMerged`'s per-component moves also reach); the SBA, under the variant, after
the legend rule and only when the pass did nothing else, walks each player's graveyard then exile and asks the owner
through `ConfirmEffect`, clearing the mark either way.

`sim.Plan.Variant` (CLI `-variant commander`, recorded in the run manifest, the commanders in the deck digest) seats
each deck's Commander section after the library and before the sticker sheets; without the variant the section is
ignored.

Not ported: Brawl, Oathbreaker, TinyLeaders, Planeswalker; partner and companion; deck legality and color identity;
`initCommanderColor`'s color choice; the merged-commander replacement suppression (`quirks.md`).

## Consequences

**Good:** a Commander game starts at 40, a dead or exiled commander can come back to the Command zone, and 21 combat
damage from one commander ends the game, so Commander batch runs play the real format. You're in Command now gets the
same rules, as in Java. **Bad:** the SBA runs after the whole legend rule rather than inside Java's per-player loop, so
when one player's legend rule acts and another's commander is waiting, the return lands one pass later (same result
before priority, a quirks row). Java's own gate, offering the return only in a pass where no other state-based action
acted, already differs from CR 704.3, which performs every applicable state-based action at once; a mark set on the
move, not "since the last check" (CR 903.9a), keeps the offer open for that later pass. Whether a commander returns
depends on the controller's `ConfirmEffect` answer; the probe controller's yes budget decides it until M7. **Neutral:**
variant-less games are unchanged except that commander damage is counted, which nothing reads without the variant.

## Related

ADR-0051, ADR-0053, ADR-0014, `porting/port-log/game-state.md`, `porting/port-log/quirks.md`
