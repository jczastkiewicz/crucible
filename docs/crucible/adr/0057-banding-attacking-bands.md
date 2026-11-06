# ADR-0057 — Banding: Attacking Bands on `Combat`, a Band Decision After the Attack

- **Status:** Accepted
- **Date:** 2026-11-05
- **Deciders:** Crucible session (B-row pack, Banding oracle), accepted by `jc@archlab.pl`

## Context

Banding (CR 702.22) changes three parts of combat:

1. Who attacks together. Any number of banding creatures plus at most one creature without banding form a band. "Bands
   with other <type>" bands only with creatures of that type.
2. Who is blocked. A creature blocking one member of a band blocks the whole band (CR 702.22h). An effect that blocks
   one member blocks the whole band too (CR 702.22i).
3. Who divides damage. The player whose side has banding divides the other side's combat damage (CR 702.22j-k).

The corpus has 24 `K:Banding` cards and one `K:Bands with other` (Old Fogey). Four statics grant Banding (Banding
Sliver, Dire Wolves, Fortified Area, Cooperation) and five grant `Bands with other:Creature.Legendary`. The engine reads
none of it: the keyword line has no trait, and `Combat` has no band.

Java models each attacker's band as an `AttackingBand` (`AttackingBand.java:12-65`; `isValidBand` at `:29-55`). Every
attacker belongs to one band, a singleton by default (`Combat.java:267-292`). A block is keyed on the band
(`Combat.java:402-404`, `:457-464`). Damage division switches the deciding player:

| Damage                      | Normally decided by  | With banding                                                     | Java                  |
| --------------------------- | -------------------- | ---------------------------------------------------------------- | --------------------- |
| A blocked attacker's damage | Attacking player     | The blocker's controller, when the blockers form a valid band    | `Combat.java:836-841` |
| A blocker's damage          | Blocker's controller | Attacking player, when the attackers it blocks form a valid band | `Combat.java:764-781` |

The second row presupposes CR 510.1d: a blocker on several attackers divides its damage. The engine dealt its full
damage to each until `36282c9e7bb`, which routes the division through `AssignCombatDamage`.

`PlayerController` has no band method. Bands come back inside `declareAttackers`' `Combat`
(`PlayerController.java:173`). The human UI builds them (`InputAttack.java:186-249`). The AI only joins a banding
creature onto an attacker's existing band (`AiAttackController.java:504-579`), never starting one itself.

## Decision Drivers

- ADR-0024: a declaration is validated, and an illegal one is an `*IllegalDeclarationError`, never repaired.
- ADR-0010 L3: a `ReplayController` must answer every Go decision from Java's recorded ones.
- GO-12: iteration order is load-bearing, so no bare map holds the bands.
- The cost must fall on the 25 banding cards, not on every combat. 678 test lines declare attackers, and three
  controllers implement `DeclareCombatAttackers`.

## Considered Options

1. **Widen `DeclareCombatAttackers` to return bands.** Rejected: it changes every controller and every combat test for a
   mechanic on 25 cards.
2. **A new decision `DeclareBands`, asked after the attack only when a declared attacker has Banding or Bands with
   other.** Chosen.
3. **Leave Banding inert, as now.** Rejected: Java's AI does band (`reinforceWithBanding`), so AI-vs-AI parity (L4)
   differs whenever a banding deck attacks.
4. **Bands as a map from attacker to band id.** Rejected by GO-12.

## Decision

Option 2.

**State.** `Combat.bands [][]CardID` is ordered by declaration, and each declared attacker sits in exactly one band. No
`DeclareBands` call leaves every attacker a singleton. Removing an attacker from combat removes it from its band, and an
emptied band is dropped. Java leaves an empty band in its map (`Combat.java:280`, `:640`). Nothing reads it except
`getAttackingBands`, so it is not reproduced. A PORT-8 report waits on a test that shows it.

**Declaration.** `PlayerController.DeclareBands(g, decider, attackers) [][]CardID` is asked once, only when an attacker
can band. It comes after the attack is checked and before anything taps, exerts or pays, so an illegal band changes
nothing (ADR-0024). An attacker a cost then removes leaves its band. Each returned band is checked with
`isValidBand(band, false)`:

- at most one member without banding;
- for Bands with other, every member matches the filter;
- every member attacks the same target, as Java keys bands by defender (`Combat.java:267-292`).

An invalid band, or an attacker in two bands, is an `*IllegalDeclarationError` (ADR-0024). Attackers the answer leaves
out stay singletons. The scripted, smoke and default controllers return nil.

**Blocking.** Blockers are validated against the attacker the player named, including the per-blocker attacker count
(`canBlockMoreCreatures`). Only then is each block expanded to one `Block` per member of that attacker's band (CR
702.22h, `Combat.java:402-404`). The expansion therefore never trips a blocker's one-attacker limit. An effect that
blocks one member (`BecomesBlocked`) blocks every member (CR 702.22i, `Combat.java:398-399`); Java runs
`AttackerBlocked` for the target only, reproduced, a `quirks.md` and a `forge-java-defects.md` row.

**Damage.** There is one question, `AssignCombatDamage`, and only the decider changes:

- A blocked attacker whose live blockers form a valid band (`isValidBand(blockers, true)`, at least one with banding)
  has its damage divided by the controller of the first blocker in block order. This reproduces `Combat.java:839-840`'s
  `get(0)`.
- A blocker whose attackers form a valid band has its damage divided by the attacking player.

When the banding side decides, Java lets it divide freely, not in lethal-first order (the last argument at
`Combat.java:784`). The Go question already trusts the answer's division.

**Replay.** Java records one `declareAttackers` answer carrying the bands. `ReplayController` answers
`DeclareCombatAttackers` with its attackers and `DeclareBands` with its bands. That is two Go questions from one record,
and the option-set assertion runs on the first.

## Consequences

**Good:** Combats without a banding attacker run exactly as today, with no new question and no new state. Damage needs
no second controller method. The replay maps one Java record onto two Go decisions without guessing.

**Bad:** `PlayerController` gains a method that every adapter must implement. The M7 AI must port `reinforceWithBanding`
to answer it. Until then the default controller declares no bands, so a banding deck attacks without them in
simulations.

**Neutral:** The CR 510.1d division (`36282c9e7bb`) already sends a blocker's multi-attacker damage through
`AssignCombatDamage`. Banding only changes who answers.

## Related

ADR-0010, ADR-0024, ADR-0036, ADR-0043
