# RestartGame and ExiledWithSource (ADR-0034)

Karn Liberated's ultimate (`karn_liberated.txt:7`) and the per-card "exiled with" state its
`SubAbility$ ReturnFromExile` reads. Design: `docs/crucible/adr/0034-restartgame-driver-restart-signal.md`.

## ExiledWithSource lands

`Card.exiledWith` (`card.go`) is Java's `Card.exiledWith` (`Card.java:326`): which host object exiled the card.

| Piece                       | Where              | Java                                                                                        |
| --------------------------- | ------------------ | ------------------------------------------------------------------------------------------- |
| Mark cleared on zone entry  | `put`/`putFront`   | `Card.cleanupExiledWith` on every move but to the stack (`GameAction.java:576-579`)         |
| Mark set after exile move   | `markExiledWith`   | `SpellAbilityEffect.handleExiledWith` (`SpellAbilityEffect.java:1087-1116`)                 |
| Host object stamp           | `hostObjectStamp`  | `equalsWithGameTimestamp` against the ability's host object (`CardProperty.java:397-411`)   |
| `ExiledWithSource` property | `valid.go`         | `CardProperty.java:397-411`, exact name only                                                |
| Fixture `ExiledWith:<id>`   | `internal/fixture` | `GameState.java:407-410` (dump), `:771-781`/`:1398` (load)                                  |
| `Spell` valid base          | `baseMatches`      | `Card.isSpell` (`Card.java:5500-5502`): instant, sorcery, or an Aura off the battlefield    |
| Last battlefield stamp      | `battlefieldStamp` | the old object an ability keeps after its host moves (LKI copy keeps its `gameTimestamp`)   |
| Setting sites               | six effects        | `ChangeZone`, `ChangeZoneAll`, `Dig`, `DigUntil`, `Heist`, `Airbend` -- Java's own callers  |
| Reject lists lifted         | `playeffect.go`    | `Play`'s `Valid$ Card.ExiledWithSource`; `Clone`'s `cloneUnportedProperties` entry went too |

**Identity is the host object, not the `CardID`.** ADR-0034 proposed a plain `CardID` match because this port's IDs are
stable across zone moves. That over-matches: Java compares `equalsWithGameTimestamp`, so a host that left the
battlefield and came back is a new object that exiled nothing. The mark stores the host's `zoneStamp` at exile time; the
property compares it against the stamp of the host object an ability of the host sees (`hostObjectStamp`, `game.go`):

| Host is in                          | Object an ability sees                       | Reason                                                           |
| ----------------------------------- | -------------------------------------------- | ---------------------------------------------------------------- |
| Battlefield, Stack, Command         | current object (`zoneStamp`)                 | Java's host object is the live one                               |
| elsewhere, left the battlefield     | last battlefield object (`battlefieldStamp`) | Java's ability keeps the old host object, not the moved copy     |
| elsewhere, never on the battlefield | current object, not listed                   | Java adds to `exiledCards` only for a host in play/stack/Command |

The second row is what makes Karn work: the restart shuffles Karn into its library before `ReturnFromExile` resolves,
and Java's `sa.getHostCard()` is still the battlefield-era Karn object whose `exiledCards` list and timestamp match.
`battlefieldStamp` is set as a card leaves the battlefield (`Move`, `MoveToLibraryTop`) because the `Game.lki` snapshot
is taken after `put` has already restamped the card, so its `zoneStamp` is the new zone's. Divergence: an `Ability`
carries no host object of its own, so an ability a moved card's new object activates from its graveyard or hand reads
the old battlefield object too; no real `ExiledWithSource` line is on such an ability (corpus grep of `ActivationZone$`/
`TriggerZones$` Graveyard/Hand/Exile over the 186 `ExiledWithSource` cards: one hit, Altar of the Wretched, whose
graveyard ability does not read it).

`listed` is Java's `exilingSource.addExiledCard(movedCard)` guard (`SpellAbilityEffect.java:1100-1104`): only a host in
play, on the stack or in the Command zone lists the card, and the property requires `source.hasExiledCard(card)` too.
`Game.SetExiledWith` (fixture loading) always lists, as `GameState`'s own `addExiledCard` does.

**Rejected, before acting.**

| Shape                             | Where                  | Reason                                                                      |
| --------------------------------- | ---------------------- | --------------------------------------------------------------------------- |
| `ExiledWithEffectSource$` (3)     | `ChangeZone`           | marks the effect card's own source instead (`SpellAbilityEffect.java:1092`) |
| `ExiledWithSourceLKI` (15)        | `Play` (`playSpecGap`) | reads the exile zone's cards-added-this-turn LKI list, not ported           |
| `ExiledWithEffectSource` property | `Play` (`playSpecGap`) | effect-card source comparison, not ported                                   |

Not set: cost exiles (`CostExile`, `CostExileFromStack`, `CostBeholdExile`, `CostForage`, `CostCollectEvidence` all call
`handleExiledWith`) and the casting-time sites (`PlaySpellAbility.java:534-535`, `CostAdjustment.java:274-275`, craft
`Card.java:1571-1572`). This port's exile costs are self-exile only (`exile.go`, `exilefromgrave.go`), so the mark Java
sets there names the card itself; a later `ExiledWithSource` check by that same card reads false here. Casting-time
sites have no port to hang the mark on (no Adventure/craft casting).
