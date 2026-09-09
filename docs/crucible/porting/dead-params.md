# Dead Params

- **Status:** Verified and acted on. Seven renames are upstream in
  [#11846](https://github.com/Card-Forge/forge/pull/11846); the nine no-op deletions are ready; Dead Ringers is the one
  entry still open
- Every candidate was checked against the effect's Java, the key's git history, sibling cards using the candidate key,
  and the card's printed Oracle text from Scryfall

Parameters that card scripts write and **no Java code reads**. Forge does not warn: an unread key sits in the param map
and the effect proceeds without it, so the card silently does less than its script says.

## How they were found

`tools/apiscan` reads every param name Forge reads, from every call site in `forge-game`, `forge-ai` and
`forge-gui/player` — **1,276 keys** across six shapes:

| Shape                                                | Example                                         |
| ---------------------------------------------------- | ----------------------------------------------- |
| The accessors on an ability                          | `sa.getParam("NumDmg")`                         |
| The raw map, before an ability object exists         | `mapParams.containsKey("Layer")`                |
| A helper taking the key as an argument               | `getDefinedPlayersOrTargeted(sa, "TokenOwner")` |
| A key bound to a variable first                      | `final String key = "ResultSubAbilities"`       |
| Triggers and replacements matching against the event | `matchesValidParam("ValidExplorer", …)`         |
| A list literal with no call site                     | `additionalAbilityKeys`                         |

Every param use in the corpus — **262,477** of them — is then checked against that set. **Eighteen uses of thirteen keys
across 17 cards** match nothing. `grep -rn '"<key>"' --include=*.java .` over the whole repository returns **0** for all
thirteen.

The first pass reported 326 unknown keys. All but these were the scan's own blind spots, one per shape above; a
vocabulary gate is only worth as much as the completeness of what it compares against.

Counting them needs the same care. A plain `grep -F 'Secret$'` reports two cards, because `KeepSecret$` on
`ominous_lockbox` ends with the searched string. Keys are only ever preceded by a line start, a pipe or a colon, so the
count below anchors on that:

```console
$ grep -rhoE '(^|[|:] )Secret\$' forge-gui/res/cardsfolder/ | wc -l
1
```

## How each candidate was verified

Four independent checks per key, because a plausible rename is not evidence:

| Check                | What it settles                                                          |
| -------------------- | ------------------------------------------------------------------------ |
| Effect source        | Which keys the effect actually reads, and the default when one is absent |
| `git log -S` on Java | Whether the key was ever read, and which commit stopped reading it       |
| Sibling cards        | What every other card in the same situation writes                       |
| Printed Oracle text  | What the card is supposed to do, independent of any Forge script         |

## Verdicts

| Parameter                 | Uses | Verdict                                  | Fix               | State    |
| ------------------------- | ---: | ---------------------------------------- | ----------------- | -------- |
| `ValidConniver`           |    1 | **Wrong behaviour** — restriction absent | → `ValidCard`     | #11846   |
| `PeekNum`                 |    1 | **Wrong behaviour** — 1 card, not X      | → `PeekAmount`    | #11846   |
| `Secret`                  |    1 | **Wrong behaviour** — vote is public     | → `Secretly`      | #11846   |
| `ValidTgtDesc`            |    2 | Cosmetic — raw valid string shown        | → `ValidTgtsDesc` | #11846   |
| `ValidTgtsDes`            |    2 | Cosmetic — raw valid string shown        | → `ValidTgtsDesc` | #11846   |
| `ConditionPresentCompare` |    1 | **Wrong behaviour** — `GE1`, not `EQ2`   | two candidates    | **open** |
| `TokenController`         |    3 | No-op — identical to the default         | delete            | ready    |
| `RememberRandomChoice`    |    2 | No-op — already unconditional            | delete            | ready    |
| `OverwriteSpells`         |    1 | Leftover — reader deleted 2023           | delete            | ready    |
| `AISearchGoal`            |    1 | Leftover — reader deleted 2019           | delete            | ready    |
| `AlternativeMessage`      |    1 | Leftover — reader deleted 2023           | delete            | ready    |
| `SpeTgtPrompt`            |    1 | Invented — never existed in Java         | delete            | ready    |
| `TrigDescReminderDefined` |    1 | Invented — never existed in Java         | delete            | ready    |

Four behavioural defects, two cosmetic keys over four cards, seven dead keys with no observable effect. Faerie Dragon
writes `RememberRandomChoice$` twice, which is why 18 uses fall on 17 cards.

`#11846` is the shipped rename; `ready` is verified and waiting on a deletion commit; `open` means the key is certainly
dead but the correct fix has not been decided.

---

## Wrong behaviour

### `ValidConniver` — Leader, Super-Genius

```text
R:Event$ Connive | ActiveZones$ Battlefield | ValidConniver$ Creature.YouCtrl | ReplaceWith$ DBDraw | ...
```

Printed text: _If a creature **you control** would connive, instead you draw a card, then that creature connives._

`ReplaceConnive.canReplace` reads **`ValidCard`** and nothing else
(`forge-game/src/main/java/forge/game/replacement/ReplaceConnive.java:15`):

```java
if (!matchesValidParam("ValidCard", runParams.get(AbilityKey.Affected))) {
```

`CardTraitBase.matchesValidParam` (`CardTraitBase.java:259-265`) returns `!hasParam("Invert" + param)` when the param is
absent — i.e. **true**. The restriction is not merely loosened, it is gone: the replacement applies to every connive in
the game, including an opponent's, and its controller draws the card each time.

The only `Event$ Connive` replacement in the corpus, so there is no sibling to compare against; the Java and the printed
text settle it on their own.

**Decision:** rename to `ValidCard`.

### `PeekNum` — Mindblaze

```text
SVar:DBReveal:DB$ PeekAndReveal | PeekNum$ X | NoPeek$ True | ValidTgts$ Player | RememberRevealed$ True | ...
SVar:X:TargetedPlayer$CardsInLibrary
```

Printed text: _Target player **reveals their library**. If that library contains exactly the chosen number of cards with
the chosen name, Mindblaze deals 8 damage to that player._

`PeekAndRevealEffect.resolve` (`PeekAndRevealEffect.java:57`):

```java
String peekAmount = sa.getParamOrDefault("PeekAmount", "1");
```

So the reveal is the **top card only**. `DBDamage` then counts `Card.NamedCard` among `Remembered` and compares against
the chosen number, so Mindblaze can only ever deal damage when the chosen number is 1 and the top card matches. `X` —
the whole library — is computed and discarded.

Sibling evidence is decisive. Commit `6ba40743621` (2022-05-03,
`refactor more DigEffect + NoMove$ cards to PeekAndReveal`) converted 18 cards in one pass; every other card in that
commit got `PeekAmount$`, Mindblaze alone got `PeekNum$`. The source key in `DigEffect` is `DigNum`
(`DigEffect.java:41`), which is where the wrong name came from.

**Decision:** rename to `PeekAmount`.

### `Secret` — Mob Verdict

```text
A:SP$ Vote | Defined$ Player | Secret$ True | VotePlayer$ Other | StoreVoteNum$ True | ...
```

Printed text: _Secret council — Each player **secretly** votes for another player, then those votes are revealed._

`VoteEffect.java:59` reads `Secretly`. Without it the votes are cast openly, so every player after the first votes with
full knowledge — the opposite of what a secret council is for.

All four other "Secret council" cards write `Secretly$ True`. Cirdan, the Shipwright is a structural twin:

```text
SVar:TrigVote:DB$ Vote | Defined$ Player | Secretly$ True | VotePlayer$ Player | StoreVoteNum$ True | ...
```

**Decision:** rename to `Secretly`.

### `ConditionPresentCompare` — Dead Ringers (open)

```text
A:SP$ Destroy | TargetMin$ 2 | TargetMax$ 2 | NoRegen$ True | ValidTgts$ Creature.nonBlack |
  ConditionNoDifferentColors$ Targeted | ConditionDefined$ Targeted | ConditionPresent$ Card |
  ConditionPresentCompare$ EQ2 | ...
```

Printed text: _Destroy two target nonblack creatures unless either one is a color the other isn't._ Gatherer ruling:
_Both of the target creatures must be exactly the same color or combination of colors. Both being colorless is also
okay. If they differ in any way, you can still cast the spell, but it does not do anything on resolution._

The condition vocabulary is `ConditionDefined` / `ConditionPresent` / `ConditionCompare`
(`SpellAbilityCondition.java:164-173`). `presentCompare` defaults to `"GE1"` (`SpellAbilityVariables.java:102`), so the
written `EQ2` is discarded and the check becomes "at least one target remains".

The divergence is narrow but the intent is documented. Commit `ca362664b7c` (2024-03-27, _MagicStack: fix fizzle
removing too much Targets (#4873)_) rewrote this exact line, replacing a `RememberOriginalTargets` scheme with the
`Targeted` conditions, and added the `EQ2` on purpose — the whole commit is about what happens when one target becomes
illegal. Today, with `GE1`, the spell destroys the surviving target; the author wrote it to do nothing.

Whether `EQ2` is itself rules-correct is a separate question — CR 608.2b's default is "do as much as possible", and no
ruling covers the one-legal-target case. The defect to report is the typo; the rules question belongs to whoever reviews
it.

**Decision: open.** Excluded from [#11846](https://github.com/Card-Forge/forge/pull/11846) because the typo is certain
but the fix is not. The two candidates behave oppositely in the only case that distinguishes them — one target removed
in response:

| Fix                       | Result with one legal target left | Argument                          |
| ------------------------- | --------------------------------- | --------------------------------- |
| → `ConditionCompare$ EQ2` | Spell does nothing                | What `ca362664b7c` wrote it to do |
| Delete the param          | Destroys the surviving target     | CR 608.2b, do as much as possible |

Report as an issue, not a pull request: picking between them is a rules call, and a PR that picks one hides that a
choice was made.

---

## Cosmetic

### `ValidTgtDesc` and `ValidTgtsDes`

Beorn's Hospitality and Generous Revival write the first; Galion, Elvenking's Butler and Shuttle Crew write the second.

```text
SVar:TrigPutCounter:DB$ PutCounter | ValidTgts$ Creature.YouCtrl | ValidTgtDesc$ creature you control | ...
SVar:TrigDealDamage:DB$ DealDamage | ValidTgts$ Creature.tapped+OppCtrl | ValidTgtsDes$ tapped creature an opponent controls | NumDmg$ 4
```

The key is `ValidTgtsDesc` — one missing `s`, one missing `c`. `TargetRestrictions` falls back to a generated
description and then builds the prompt from it (`TargetRestrictions.java:136-150`):

```java
this.validTgtsDesc = Lang.getInstance().buildValidDesc(Arrays.asList(this.validTgts), maxTargets != "1");
...
this.uiPrompt = "Select target " + validTgtsDesc;
```

`Lang.formatValidDesc` (`Lang.java:233-242`) lowercases only bare card types and five common words. A valid string
carrying properties is left untouched, so the player is shown the raw script token:

| Card                | Valid string              | Prompt shown                            |
| ------------------- | ------------------------- | --------------------------------------- |
| Beorn's Hospitality | `Creature.YouCtrl`        | `Select target Creature.YouCtrl`        |
| Generous Revival    | `Creature.YouOwn+cmcLE3`  | `Select target Creature.YouOwn+cmcLE3`  |
| Galion              | `Creature.Other+YouCtrl`  | `Select target Creature.Other+YouCtrl`  |
| Shuttle Crew        | `Creature.tapped+OppCtrl` | `Select target Creature.tapped+OppCtrl` |

No rules effect — each card's `TriggerDescription` already carries the correct printed sentence — but the target prompt
is unreadable. Leader, Super-Genius spells the key correctly on its own trigger, in the same file as `ValidConniver`.

**Decision:** rename all four to `ValidTgtsDesc`.

---

## No observable effect

### `TokenController` — Orochi Hatchery, Spawning Pit, Tomb of Urami

```text
A:AB$ Token | Cost$ 5 T | TokenAmount$ Y | TokenController$ You | TokenScript$ g_1_1_snake | ...
```

`TokenEffect` takes the creating player from `getDefinedPlayersOrTargeted(sa, "TokenOwner")` (`TokenEffect.java:44`),
which resolves to `sa.getParamOrDefault("TokenOwner", "You")` (`SpellAbilityEffect.java:341`). None of the three
abilities targets, so the fallback branch is the one taken and the result is `You` — exactly what the script asks for.

`git log -S TokenController -- '*.java'` is empty: the key was never read, in any revision. It predates the 2013 module
re-org. 2,198 corpus lines write `TokenOwner$ You`.

**Decision:** delete. Renaming to `TokenOwner` is equally correct and equally pointless; deleting is the smaller diff.

### `RememberRandomChoice` — Faerie Dragon

```text
SVar:Damage3:DB$ DealDamage | NumDmg$ 3 | Random$ True | CardChoices$ Creature | PlayerChoices$ Player |
  RememberRandomChoice$ True | SubAbility$ DBCleanup | ...
```

`DamageDealEffect.java:163-169` already remembers unconditionally:

```java
if (sa.hasParam("Random")) { // only for Whimsy and Faerie Dragon
    for (int i = 0; i < n; i++) {
        GameEntity random = Aggregates.random(choices);
        tgts.add(random);
        choices.remove(random);
        hostCard.addRemembered(random); // remember random choices for log
    }
}
```

Which is why the chain ends in a `Cleanup` that clears `Remembered`. The param asks for behaviour that is not optional.

**Decision:** delete, both occurrences.

### `OverwriteSpells` — Dance of the Dead

```text
SVar:DBAnimate:DB$ Animate | Defined$ Self | OverwriteSpells$ True | Keywords$ Enchant:... | ...
```

Read until commit `25900ee10cd` (_Aura Spells have internal Attach Spell for multiple Enchant Keywords (#6996)_), which
deleted the mechanism from `AnimateEffectBase`:

```java
-        boolean clearSpells = sa.hasParam("OverwriteSpells");
-        if (clearSpells) {
-            removedAbilities.addAll(Lists.newArrayList(c.getSpells()));
-        }
```

Aura spells now carry an internal Attach spell, so there is nothing left to clear. Dance of the Dead kept the param.

**Decision:** delete.

### `AISearchGoal` — Natural Order

```text
A:SP$ ChangeZone | Cost$ 2 G G Sac<1/Creature.Green/green creature> | ... | AILogic$ SacAndUpgrade+SacWorst |
  AISearchGoal$ Creature.Green
```

Read by `ChangeZoneAi` until commit `0ba88f3ce5c` (_Improved SacAndUpgrade AI, now it works with Eldritch Evolution and
requires fewer parameters in scripts_):

```java
-        String definedGoal = sa.hasParam("AISearchGoal") ? sa.getParam("AISearchGoal") : "Creature";
```

That commit touched the only two cards using the key. Eldritch Evolution has since had it removed; Natural Order has
not.

**Decision:** delete.

### `AlternativeMessage` — Invasion of Arcavios

```text
SVar:TrigSearch:DB$ ChangeZone | Hidden$ True | Origin$ Library | Destination$ Hand | ShuffleNonMandatory$ True |
  OriginAlternative$ Graveyard,Sideboard | AlternativeMessage$ Would you like to search your library with this ability? ...
```

Commit `da0db2282c1` (_WHO: the_five_doctors.txt and support (ChangeZoneEffect refactors) (#4228)_) deleted the reader
and stripped the param from roughly 60 cards in the same diff:

```java
-                sb.append(sa.getParam("AlternativeMessage")).append(" ");
```

The prompt is now assembled from localisation keys (`ChangeZoneEffect.java:945-948`):

```java
sb.append(Localizer.getInstance().getMessage("lblSearchLibrary")).append(" ");
sb.append(altFetchList.size()).append(" ").append(Localizer.getInstance().getMessage("lblCardMatchSearchingTypeInAlternateZones"));
```

62 corpus cards use `OriginAlternative`. Invasion of Arcavios is the one the sweep missed.

**Decision:** delete.

### `SpeTgtPrompt` — Explosive Getaway

```text
A:SP$ ChangeZone | ValidTgts$ Artifact,Creature | TgtPrompt$ Choose target artifact or creature |
  Origin$ Battlefield | Destination$ Exile | TargetMin$ 0 | TargetMax$ 1 |
  SpeTgtPrompt$ Select target creature you control | ...
```

Printed text: _Exile **up to one** target artifact or creature._

The card has one target, `TgtPrompt` is present and correct, and the dead param's text names a different set of objects
than the card can target. One occurrence in 33,689 scripts, zero in Java in any revision — invented by the card's author
in `c20dfe85e45` and never supported.

**Decision:** delete. Not a misspelling of `TgtPrompt`: there is no second target for it to describe.

### `TrigDescReminderDefined` — Nihiloor

```text
SVar:DBImmediateTrigger:DB$ ImmediateTrigger | RememberObjects$ ChosenCard & Player.IsRemembered |
  ... | TrigDescReminderDefined$ Player.IsRemembered | TriggerDescription$ When you do, gain control of ...
```

`ImmediateTriggerEffect` reads `TriggerDescription`, `SpellDescription`, `TriggerAmount`, `RememberObjects`,
`RememberEach`, `RememberSVarAmount`. One occurrence in the corpus, zero in Java in any revision.

Added in `cbdd5031842` (MID update) alongside `ChoiceTitleAppendDefined$`, which _was_ subsequently supported and is now
written `ChoiceTitleAppend$ Defined …` on the line above. The reminder key never got the same treatment. Nihiloor's
`TriggerDescription` already carries the printed sentence.

**Decision:** delete.

---

## What happens to this list

Twelve of the thirteen keys are settled. Seven renames are in [#11846](https://github.com/Card-Forge/forge/pull/11846);
the six no-op keys behind the nine remaining uses go upstream as deletions. `ConditionPresentCompare` stays open until
someone rules on it.

`ConditionPresentCompare` is the one entry on `parity-matrix.md`'s deliberate-exclusion table, because a gate that fails
on every run is not a gate. The row names the reason and the condition that deletes it, which is the difference between
an exclusion and a silence: the key still fails `tools/apiscan -check`, and the table is where someone decided to ship
anyway. The other twelve are fixed rather than excluded, so the list is one row and meant to stay one row.

## Fix manifest

Every edit, by file. Paths are relative to `forge-gui/res/cardsfolder`.

| File                                                    | Edit                                                    |
| ------------------------------------------------------- | ------------------------------------------------------- |
| `l/leader_super_genius.txt`                             | `ValidConniver$` → `ValidCard$`                         |
| `m/mindblaze.txt`                                       | `PeekNum$` → `PeekAmount$`                              |
| `m/mob_verdict.txt`                                     | `Secret$` → `Secretly$`                                 |
| `d/dead_ringers.txt`                                    | `ConditionPresentCompare$` → `ConditionCompare$`        |
| `b/beorns_hospitality.txt`                              | `ValidTgtDesc$` → `ValidTgtsDesc$`                      |
| `upcoming/generous_revival.txt`                         | `ValidTgtDesc$` → `ValidTgtsDesc$`                      |
| `g/galion_elvenkings_butler.txt`                        | `ValidTgtsDes$` → `ValidTgtsDesc$`                      |
| `upcoming/shuttle_crew.txt`                             | `ValidTgtsDes$` → `ValidTgtsDesc$`                      |
| `o/orochi_hatchery.txt`                                 | drop `TokenController$ You`                             |
| `s/spawning_pit.txt`                                    | drop `TokenController$ You`                             |
| `t/tomb_of_urami.txt`                                   | drop `TokenController$ You`                             |
| `f/faerie_dragon.txt`                                   | drop `RememberRandomChoice$ True`, twice                |
| `d/dance_of_the_dead.txt`                               | drop `OverwriteSpells$ True`                            |
| `n/natural_order.txt`                                   | drop `AISearchGoal$ Creature.Green`                     |
| `i/invasion_of_arcavios_invocation_of_the_founders.txt` | drop `AlternativeMessage$ …`                            |
| `e/explosive_getaway.txt`                               | drop `SpeTgtPrompt$ Select target creature you control` |
| `n/nihiloor.txt`                                        | drop `TrigDescReminderDefined$ Player.IsRemembered`     |

Each deletion takes the key, its value, and one `|` separator, leaving the rest of the line untouched.

The seven renames shipped as one commit on `fix/unread-param-typos`, opened as
[#11846](https://github.com/Card-Forge/forge/pull/11846). `d/dead_ringers.txt` is deliberately not in it. The nine
deletions are a separate commit, because a reviewer approving seven behavioural changes should not also have to read
nine that change nothing.

## Known limit of the scan

The per-API vocabulary is not yet usable. Because the scan reads all of `forge-game` for shared keys, and the effects
live inside it, every key ends up "shared" and the per-API sets come out empty. The gate that exists is therefore "some
Java code reads this key", not "the effect this card names reads this key" — the weaker of the two claims, and the
stronger one needs the base classes separated from the effects.

## Related

- [card-script-defects.md](card-script-defects.md) — the other gate findings, and the ten already merged upstream
- [parity-matrix.md](parity-matrix.md) — the exclusion table these fixes keep empty
- `PORT-8` in [CLAUDE.md](../../../CLAUDE.md) — report upstream, never work around
