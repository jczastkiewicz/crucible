# Porter plan: Subgame (CR 720)

Scratch file for whoever resumes this worktree; deleted in the final commit.

## Assigned

- `Subgame` (`SubgameEffect.java`), 3 real corpus lines: Shahrazad, Enter the Dungeon, The Countdown Is at One. No ADR.

## Approach

1. `subgameeffect.go`: build a fresh `*Game` (`NewGame`, same `db`, the main game's own `rand` pointer - Java's
   `MyRandom` is one stream across both games), one seat per main-game player still in the game, same seat order.
2. Zones (`prepareAllZonesSubgame`): Library copied (`NewCard(def)`, tokens/copied spells skipped), outside-game cards
   (Hand, Battlefield, Graveyard, Exile, Stack, Sideboard, Ante, Merged) copied to the subgame Sideboard, Attraction /
   Contraption decks copied, each shuffled in Java's order.
3. Start (`GameAction.startGame`): coin flip + `ChooseStartingPlayer`, draw 7 (no second shuffle), `PerformMulligans`,
   `StartTurn`, `Run` with the main game's registry and controller, a turn cap that errors.
4. Outcome: `Player.Won` in the subgame -> `RememberPlayers$ Win|NotWin` on the host; shuffle main libraries and variant
   decks (`player.shuffle(sa)` loop).
5. Rejected before acting: `StartingLife$` non-integer, `RememberPlayers$` other than Win/NotWin, Condition params,
   SchemeDeck/PlanarDeck in the main game (game-start variant actions unported), a Companion among outside cards.
   Rejected after the subgame: a card that left the subgame Sideboard (CR 720.4a cross-game mapping, not built).
6. Tests (`subgame_test.go`), docs `effects-subgame.md`, index row, counts, gates, commit.
