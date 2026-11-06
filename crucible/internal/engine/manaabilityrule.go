// CR 605.1a: which activated mana-producing lines are mana abilities.

package engine

//enginelint:allow zone

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
)

// isManaAbility is SpellAbility.isManaAbility (SpellAbility.java:394-440) for
// an ability line: an activated Mana or ManaReflected line that has no target,
// is no loyalty ability, and neither pays a cost nor resolves a sub-ability
// that moves a card to or from a library (CR 605.1a). Chromatic Sphere's draw
// and Charmed Pendant's mill make theirs ordinary activated abilities, which
// use the stack and cannot be activated while a cost is being paid. The test
// reads the script, not the board, as Java's does (CR 605.1a disregards
// replacement effects).
func isManaAbility(ab *compile.Ability) bool {
	if ab == nil || ab.Record != compile.Activated || (ab.Name != "Mana" && ab.Name != "ManaReflected") {
		return false
	}
	// usesTargeting and isPwAbility read the root only (SpellAbility.java:
	// 400-403, :814-817, :1779-1781).
	for _, key := range [...]string{"ValidTgts", "TgtPrompt", "Planeswalker"} {
		if _, ok := ab.Param(key); ok {
			return false
		}
	}
	if text, ok := ab.Param("Cost"); ok && costMovesLibraryCard(cost.Parse(text)) {
		return false
	}
	for tail := ab; tail != nil; tail = manaChainNext(tail) {
		if effectMovesLibraryCard(tail) {
			return false
		}
	}
	return true
}

// costMovesLibraryCard is LibraryMovementCostVisitor.movesCardToOrFromLibrary:
// a Mill, Draw or PutCardToLib part, or an exile from the library
// (ExileFromTop, the one CostExile whose zone is Library, Cost.java:469).
func costMovesLibraryCard(c cost.Cost) bool {
	for _, p := range c.Parts {
		switch p.Name {
		case "Mill", "Draw", "PutCardToLib", "ExileFromTop":
			return true
		}
	}
	return false
}

// effectMovesLibraryCard is SpellAbilityEffect.movesCardToOrFromLibrary for
// a's API, the overrides in forge-game's effects package: the APIs that always
// move one, ChangeZone and ChangeZoneAll naming Library as Origin$ or
// Destination$, Play naming it as ValidZone$, and Manifest or Cloak with no
// ChoiceZone$ or one naming it (ManifestBaseEffect.java:24).
func effectMovesLibraryCard(a *compile.Ability) bool {
	switch a.Name {
	case "Draw", "Mill", "Dig", "DigUntil", "Surveil", "Seek", "Connive", "Discover", "Explore", "Learn", "Heist":
		return true
	case "ChangeZone", "ChangeZoneAll":
		return paramNamesLibrary(a, "Origin") || paramNamesLibrary(a, "Destination")
	case "Play":
		return paramNamesLibrary(a, "ValidZone")
	case "Manifest", "Cloak":
		_, has := a.Param("ChoiceZone")
		return !has || paramNamesLibrary(a, "ChoiceZone")
	}
	return false
}

// paramNamesLibrary is SpellAbilityEffect.zoneParamIsLibrary: key's
// comma-separated zone list names Library.
func paramNamesLibrary(a *compile.Ability, key string) bool {
	v, ok := a.Param(key)
	if !ok {
		return false
	}
	for _, z := range strings.Split(v, ",") {
		if zone, ok := ZoneByName(strings.TrimSpace(z)); ok && zone == Library {
			return true
		}
	}
	return false
}

// manaChainNext is a's SubAbility$ link, nil for none.
func manaChainNext(a *compile.Ability) *compile.Ability {
	for _, s := range a.Subs {
		if strings.EqualFold(s.Key, "SubAbility") {
			return s.Ability
		}
	}
	return nil
}
