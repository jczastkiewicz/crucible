package smoke

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// maxCostCopies caps how many copies of one card a cost or condition places.
const maxCostCopies = 4

// costNeed is a card a cast needs on the probe player's side: one that spec
// accepts, count of them, in zone.
type costNeed struct {
	spec  string
	zone  engine.ZoneType
	count int
}

// costNeeds lists what def's casting asks of its caster's own cards: the
// permanents a spell Cost$, a RaiseCost Cost$ on the card itself or an
// AlternateAdditionalCost keyword sacrifices, taps, returns or exiles; the
// cards it reveals or beholds; and the permanents an IsPresent$ on the spell
// line counts (Feast of Blood's two Vampires). The generic boards rarely hold
// a Goblin to sacrifice or a Pirate to reveal, so such a card was never cast.
func costNeeds(def *compile.Card) []costNeed {
	var out []costNeed
	addCost := func(text string) {
		for _, p := range cost.Parse(text).Parts {
			spec, _, _ := strings.Cut(p.Field(1), ";")
			if spec == "" || spec == "CARDNAME" || spec == "NICKNAME" || spec == "Card.Self" {
				continue
			}
			n, err := strconv.Atoi(p.Field(0))
			if err != nil || n < 1 {
				n = 1
			}
			switch p.Name {
			case "Sac", "tapXType", "Return", "Exile":
				out = append(out, costNeed{spec, engine.Battlefield, min(n, maxCostCopies)})
			case "Reveal", "Behold", "BeholdExile":
				out = append(out, costNeed{spec, engine.Hand, min(n, maxCostCopies)})
			}
		}
	}
	for _, a := range def.Faces[0].Abilities {
		if a.Record != compile.Spell {
			continue
		}
		if text, ok := a.Param("Cost"); ok {
			addCost(text)
		}
		if spec, ok := a.Param("IsPresent"); ok {
			n := 1
			if cmp, ok := a.Param("PresentCompare"); ok && strings.HasPrefix(cmp, "GE") {
				if v, err := strconv.Atoi(cmp[2:]); err == nil && v > 0 {
					n = v
				}
			}
			alt, _, _ := strings.Cut(spec, ",")
			out = append(out, costNeed{alt, engine.Battlefield, min(n, maxCostCopies)})
		}
	}
	for _, s := range def.Faces[0].Statics {
		if !strings.EqualFold(s.Name, "RaiseCost") {
			continue
		}
		if v, _ := s.Param("ValidCard"); v != "Card.Self" {
			continue
		}
		if text, ok := s.Param("Cost"); ok {
			addCost(text)
		}
	}
	for _, line := range def.Faces[0].Keywords {
		if k := keyword.Parse(line); k.Name == "AlternateAdditionalCost" {
			for _, alt := range strings.Split(k.Details, ":") {
				addCost(alt)
			}
		}
	}
	return out
}

// addCostTailored places, for each costNeed of def, that many copies of a
// corpus card its spec accepts on the probe player's side.
func (p *Prober) addCostTailored(g *engine.Game, pid engine.PlayerID, def *compile.Card) {
	for _, need := range costNeeds(def) {
		name := p.candidate(targetSpec{alt: need.spec, zone: need.zone}, true)
		d, ok := p.db.Card(name)
		if !ok {
			continue
		}
		for range need.count {
			g.NewCard(d, pid, need.zone)
		}
	}
}
