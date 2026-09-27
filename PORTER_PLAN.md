# Porter plan: SetInMotion (Archenemy, CR 904)

Scratch file; deleted in the final commit.

| Step | Piece                                                                                                          | Files                                      |
| ---- | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| 1    | `setSchemeInMotion` game action (Player.setSchemeInMotion): replacement fail-closed, move top/given to Command | `setinmotioneffect.go`                     |
| 2    | `Mode$ SetInMotion` trigger walk (Battlefield + whole Command, TriggerZones$ gate, records Scheme)             | `trigger.go` (appended), `ability.go`      |
| 3    | `SetInMotion` effect: `RepeatNum$`, `Again$` (root triggered Scheme), `ConditionDefined$ Remembered`           | `setinmotioneffect.go`                     |
| 4    | Main1 turn-based action (PhaseHandler.java:278, gate = SchemeDeck non-empty) and scheme SBA (GameAction:1745)  | `turn.go`, `action.go` (if scope approved) |
| 5    | Tests, registry regen, docs `effects-setinmotion.md`, index row, counts                                        | tests, docs                                |

Commit after each step that builds and passes `gates.sh fast`.

Status: steps 1-5 committed; remaining: rules review, fix findings, delete this file.
