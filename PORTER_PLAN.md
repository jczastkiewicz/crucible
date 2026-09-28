# Porter plan: Meld (ADR-0032)

Scratch file for whoever resumes this worktree; deleted in the final commit.

## Assigned

`Meld` (7 corpus lines: Gisela/Bruna, Graf Rats/Midnight Scavengers, Hanweir Battlements/Garrison, Titania/Argoth,
Urza/Mightstone, Mishra/Dragon Engine, Vanille/Fang).

## Approach

- Secondary representation: Java-literal `PlayerZoneBattlefield.addToMelded` (PlayerZoneBattlefield.java:45-49):
  `Zone`/`ZoneOwner` = Battlefield/controller, `Melded=true`, but NOT in the zone's card set. Every battlefield walk (84
  `Zone(Battlefield...)` sites) excludes it for free. Differs from ADR wording (per-walk skip); documented in port-log.
- Primary: `MeldedWith CardID`; Def swapped to meld face via `frontDef` (same as `transform()`).
- Split back: hook in `Game.Move` and `Game.MoveToLibraryTop` leaving-battlefield branches; helper in `game.go`.
- `named<Name>` valid property (CardProperty.java:62-68) - needed by every corpus line's trigger/condition.
- `removeFromCombat` for both exiled cards; `Attacking$ True` via shared helper from `changecombatantseffect.go`.

## Order

1. Commit this plan.
2. Primitive: Card fields, unmeld hooks, `named` property + tests. Commit.
3. `meldeffect.go` + tests + registry. Commit.
4. Docs (port-log `effects-meld.md`, index row, counts). Gates full. Commit, delete this file.
