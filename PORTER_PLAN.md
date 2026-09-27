# Porter plan: UnlockDoor (Duskmourn Rooms)

Batch slug `unlockdoor`. Scratch file; deleted in the final commit.

## Corpus facts

- Effect lines (`AB$`/`DB$ UnlockDoor`): 4. `Mode$ Unlock` x2 (Ghostly Keybearer targeted, Ghostly Dancers `Choices$`),
  `Mode$ LockOrUnlock` x2 (Marina Vendrell, Keys to the House, targeted). Default `Mode$ ThisDoor`: 0 script lines --
  only Java's synthesized `ST$ UnlockDoor` special action (CardFactoryUtil.java:124) and cast-ETB unlock
  (GameAction.java:571).
- Trigger `Mode$ UnlockDoor ... ThisDoor$ True`: 30 lines. `Mode$ FullyUnlock`: 17 lines. 30 Room cards.

## Steps (commit after each)

1. Engine state: `Card.doors` bitmask + printed split def (`roomDef`), room view Def, enter/leave battlefield paths,
   `NewCard` into battlefield, Clone. Unlock/lock primitives, UnlockDoor + FullyUnlock trigger modes.
2. `unlockDoorEffect`: `Unlock`, `LockOrUnlock`; reject default/`ThisDoor` mode. New controller decision
   `ChooseRoomDoor`. `FullyUnlocked` valid property.
3. Cast a Room half: `ActionCast.AbilityIndex` 0 = left, 1 = right; stack view; permanentEffect unlocks cast half.
4. Special action `ActionUnlockDoor`: sorcery timing, pay locked door's cost, no stack.
5. Fixture: `UnlockedRoom:` load + dump, `queue` verb for the door choice, scenario.
6. Docs (`effects-unlockdoor.md`, index row, counts), gates full, delete this file.
