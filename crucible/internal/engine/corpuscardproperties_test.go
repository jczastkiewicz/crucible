package engine_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Every property a corpus card's valid string tests against a card resolves:
// none reaches checkTypeWord's "not resolvable yet" (R19b, GO-7). Java's
// CardProperty answers an unknown word by type name, false for any card, so
// a property this port does not read would otherwise filter silently.
//
// Player and spell-ability bases and keys are left out: they run through
// their own chains (PlayerProperty, SpellAbilityProperty), not Matches.
// CanPayManaCost is Play's alone: playeffect.go reads it from the
// activation's paid mana before Matches sees the rest of Valid$.
func TestEveryCorpusCardPropertyResolves(t *testing.T) {
	t.Parallel()
	db := scenarioDB(t)
	notCard := map[string]bool{"Player": true, "You": true, "Opponent": true, "Any": true, "Each": true,
		"Defined": true, "Activated": true, "Spell": true, "SpellAbility": true, "Triggered": true,
		"Ability": true, "LandAbility": true, "Static": true}
	notCardKey := map[string]bool{"ValidSA": true, "ValidAfterStack": true, "ValidActivatingPlayer": true,
		"ValidPlayer": true, "ValidPlayers": true, "ValidDefender": true}
	props := map[string]string{} // property -> a card that tests it
	var walk func(card string, a *compile.Ability)
	walk = func(card string, a *compile.Ability) {
		for _, p := range a.Params {
			if !vocab.IsValidKey(p.Key) || notCardKey[p.Key] {
				continue
			}
			for _, alt := range valid.Parse(p.Value).Alternatives {
				if notCard[alt.Base.Name] {
					continue
				}
				for _, pr := range alt.Properties {
					if _, ok := props[pr.Name]; !ok && pr.Compare == nil && pr.Name != "CanPayManaCost" {
						props[pr.Name] = card
					}
				}
			}
		}
		for _, s := range a.Subs {
			if s.Ability != nil {
				walk(card, s.Ability)
			}
		}
	}
	for _, c := range db.Cards() {
		for i := range c.Faces {
			f := &c.Faces[i]
			for _, list := range [][]*compile.Ability{f.Abilities, f.Triggers, f.Statics, f.Replacements} {
				for _, a := range list {
					walk(c.Name, a)
				}
			}
		}
	}
	if len(props) < 800 {
		t.Fatalf("found %d properties, want the corpus's (over 800)", len(props))
	}

	g, p, _ := newTwoPlayerGameOn(t, db)
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	source := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	var unresolved []string
	for name, card := range props {
		_ = g.TakePendingError()
		engine.Matches(g, g.Card(bears), valid.Parse("Card."+name), p, source)
		if err := g.TakePendingError(); err != nil && strings.Contains(err.Error(), "not resolvable yet") {
			unresolved = append(unresolved, name+" ("+card+"): "+err.Error())
		}
	}
	sort.Strings(unresolved)
	for _, u := range unresolved {
		t.Error(u)
	}
}
