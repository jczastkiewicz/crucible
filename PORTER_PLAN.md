# Porter plan: ChooseSector

Scratch file, deleted in the final commit.

| Step | What                                                                                                  |
| ---- | ----------------------------------------------------------------------------------------------------- |
| 1    | `PlayerController.ChooseSector` + `ScriptedController` queue; `Memory.chosenSector` (clone copies it) |
| 2    | `choosesectoreffect.go`: host controller picks Alpha/Beta/Gamma, writes host `Memory.SetChosenSector` |
| 3    | Regenerate registry, enginelint, tests in `package engine_test`                                       |
| 4    | Docs: `effects-choosesector.md`, `game-state.md` index row, counts in `CLAUDE.md` and plan            |
| 5    | Gates full, commit, delete this file                                                                  |

Read side (`Creature.ChosenSector`/`DifferentSector`, SBA 704.5u sector assignment) stays unported, by assignment.
