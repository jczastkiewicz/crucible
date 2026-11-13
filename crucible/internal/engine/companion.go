// Companion (CR 702.139): Player.assignCompanion (Player.java:3223-3308),
// which Match.prepareAllZones runs for each player with a sideboard, after
// the sideboard is seated and before the library is shuffled
// (Match.java:352-356). The player may reveal a sideboard card whose
// companion condition their starting deck meets; it goes to the Command
// zone with its Companion Effect card (compile.CompanionEffect), whose
// granted special action puts it into its owner's hand for {3}.

package engine

//enginelint:allow card game zone control id effecteffect effecthelpers valid

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// minimumDeckSize is the game type's main-deck minimum
// (DeckFormat.java:45, :78): 60 for Constructed, 99 under the Commander
// variant. DeckSizePlus20 (Yorion) reads it.
func (g *Game) minimumDeckSize() int {
	if g.HasVariant(VariantCommander) {
		return 99
	}
	return 60
}

// AssignCompanion is Player.assignCompanion for pid: the sideboard cards
// whose Companion condition pid's library meets are offered through
// controller's ChooseEntitiesForEffect (Java's optional
// chooseSingleEntityForEffect), and the one chosen moves to pid's Command
// zone with its Companion Effect. A condition the engine cannot read is an
// error, never a met condition (GO-7).
func (g *Game) AssignCompanion(pid PlayerID, controller PlayerController) error {
	side := g.Zone(Sideboard, pid).Cards()
	if len(side) == 0 {
		return nil
	}
	library := g.Zone(Library, pid).Cards()
	var legal []EntityID
	for _, id := range side {
		for _, line := range g.Card(id).KeywordLines() {
			k := keyword.Parse(line)
			if k.Name != "Companion" {
				continue
			}
			ok, err := g.companionConditionMet(pid, id, library, k.Details)
			if err != nil {
				return err
			}
			if ok {
				legal = append(legal, CardEntity(id))
			}
		}
	}
	if len(legal) == 0 {
		return nil
	}
	first, _ := legal[0].AsCard()
	chosen := controller.ChooseEntitiesForEffect(g, pid, first, legal, 0, 1)
	if err := checkChoice(chosen, legal, 0, 1); err != nil {
		return fmt.Errorf("engine: companion: %w", err)
	}
	if len(chosen) == 0 {
		return nil
	}
	companion, _ := chosen[0].AsCard()
	g.moveByEffect(controller, companion, Command, 0, NoPlayer, false)
	def := *g.db.CompanionEffect()
	name := possessive(g.Card(companion).Name()) + " Companion Effect"
	def.Name, def.Faces[0].Name = name, name
	eff := g.Card(g.NewCard(&def, pid, Command))
	eff.IsEffect = true
	eff.effectLife = effectLifetime{duration: effectPermanent, player: pid, host: companion}
	return nil
}

// possessive is LangEnglish.getPossesive (LangEnglish.java:21-26) for a
// card name.
func possessive(name string) string {
	if strings.HasSuffix(name, "s") {
		return name + "'"
	}
	return name + "'s"
}

// companionConditionMet is assignCompanion's test of one Companion keyword,
// "Companion:<restriction>:<text>" or "Companion:Special:<rule>:<text>"
// (Companion.java:12-27), against pid's library.
func (g *Game) companionConditionMet(pid PlayerID, companion CardID, library []CardID, details string) (bool, error) {
	fields := strings.Split(details, ":")
	if len(fields) < 2 {
		return false, fmt.Errorf("engine: companion %q: no condition", details)
	}
	if fields[0] != "Special" {
		// deckMatchesDeckRestriction (:3301-3308): every library card is
		// valid for the restriction, with the companion as its source.
		spec := valid.Parse(fields[0])
		for _, id := range library {
			if !Matches(g, g.Card(id), spec, pid, companion) {
				return false, nil
			}
		}
		return true, nil
	}
	switch fields[1] {
	case "UniqueNames":
		names := map[string]bool{}
		for _, id := range library {
			c := g.Card(id)
			if c.Type().Has(cardtype.Land) {
				continue
			}
			if names[c.Name()] {
				return false, nil
			}
			names[c.Name()] = true
		}
		return true, nil
	case "UniqueManaSymbols":
		for _, id := range library {
			if !uniqueManaSymbols(g.Card(id).Def) {
				return false, nil
			}
		}
		return true, nil
	case "DeckSizePlus20":
		return len(library) >= g.minimumDeckSize()+20, nil
	case "SharesCardType":
		// The printed core types every nonland card shares
		// (getPaperCard().getRules().getType(): a split card's two halves
		// combined).
		shared := []cardtype.CoreType(nil)
		first := true
		for _, id := range library {
			c := g.Card(id)
			if c.Type().Has(cardtype.Land) {
				continue
			}
			types := printedType(c.Def).CoreTypes()
			if first {
				shared, first = types, false
				continue
			}
			shared = slices.DeleteFunc(shared, func(t cardtype.CoreType) bool { return !slices.Contains(types, t) })
		}
		return first || len(shared) > 0, nil
	}
	return false, fmt.Errorf("engine: companion %q: unknown special rule", details)
}

// printedType is CardRules.getType: the front face's type line, a split
// card's two halves combined.
func printedType(def *compile.Card) cardtype.Line {
	t := def.Faces[0].Type
	if def.SplitType == carddb.SplitSplit {
		t = t.Union(def.Faces[carddb.FaceAlternate].Type)
	}
	return t
}

// uniqueManaSymbols is one card's half of Player.allCardsUniqueManaSymbols
// (Player.java:3199-3221): no shard twice in its mana cost (a split card's
// two halves read together), and no two halves with the same generic amount
// (or both free).
func uniqueManaSymbols(def *compile.Card) bool {
	costs := []mana.Cost{def.Faces[0].ManaCost}
	if def.SplitType == carddb.SplitSplit {
		costs = append(costs, def.Faces[carddb.FaceAlternate].ManaCost)
	}
	var shards []mana.Shard
	var generics []int
	for _, mc := range costs {
		for _, s := range mc.Shards() {
			if slices.Contains(shards, s) {
				return false
			}
			shards = append(shards, s)
		}
		if mc.Generic() > 0 || mc.CMC() == 0 {
			if slices.Contains(generics, mc.Generic()) {
				return false
			}
			generics = append(generics, mc.Generic())
		}
	}
	return true
}
