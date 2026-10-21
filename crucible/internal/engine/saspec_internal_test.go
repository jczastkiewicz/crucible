package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): ValidSA$ is read by statics and triggers through
// spellAbilityMatches, and each property of its vocabulary (cmc, numTargets,
// XCost, IsTargeting, the keyword names) has its own comparison; a scenario
// per spelling would need a card per spelling.

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func TestSpellAbilityMatchesProperties(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	def := targetCandidatesTestCreature(t)
	def.Faces[0].ManaCost = mana.MustParse("2 R")
	host := g.NewCard(def, p, Battlefield)
	target := g.NewCard(def, other, Battlefield)
	spellCard := g.NewCard(def, other, Stack)

	activated := func(params ...string) *compile.Ability {
		a := &compile.Ability{Record: compile.Activated, Name: "Draw"}
		for i := 0; i+1 < len(params); i += 2 {
			a.Params = append(a.Params, vocab.Param{Key: params[i], Value: params[i+1]})
		}
		return a
	}
	spell := &Ability{Source: spellCard, Controller: other, spell: true, Targets: []EntityID{CardEntity(target), CardEntity(target)}}
	ability := &Ability{Source: host, Controller: other, Params: activated("Cost", "2 R T", "Planeswalker", "True")}
	crew := &Ability{Source: host, Controller: p, Params: &compile.Ability{Record: compile.Activated, Name: "Animate", Keyword: "Crew:1"}}
	mana1 := &Ability{Source: host, Controller: p, Params: &compile.Ability{Record: compile.Activated, Name: "Mana"}}
	noParams := &Ability{Source: host, Controller: p}
	xSpell := &Ability{Source: spellCard, Controller: other, spell: true, hasXManaCostPaid: true, xManaCostPaid: 3}
	g.Card(spellCard).castX = 2
	onStack := &Ability{Source: spellCard, Controller: other, spell: true, Target: target}

	for _, tc := range []struct {
		name           string
		a              *Ability
		spec           string
		matched, known bool
	}{
		{"cmc of the spell on the stack", spell, "Spell.cmcEQ3", true, true},
		{"cmc too short", spell, "Spell.cmc", false, false},
		{"cmc of an ability cost", ability, "Ability.cmcGE3", true, true},
		{"cmc of an ability without a cost", noParams, "Ability.cmcEQ0", true, true},
		{"cmc ability lacking Cost$", &Ability{Source: host, Controller: p, Params: activated()}, "Ability.cmcEQ0", false, true},
		{"cmc unresolvable amount", spell, "Spell.cmcEQBogus", false, false},
		{"numTargets distinct", spell, "Spell.numTargets EQ1", true, true},
		{"numTargets malformed", spell, "Spell.numTargets", false, false},
		{"numTargets unresolvable", spell, "Spell.numTargets EQBogus", false, false},
		{"XCost recorded", xSpell, "Spell.XCostEQ3", true, true},
		{"XCost on the card", &Ability{Source: spellCard, Controller: other, spell: true}, "Spell.XCostEQ2", true, true},
		{"XCost too short", xSpell, "Spell.XCost", false, false},
		{"XCost unresolvable", xSpell, "Spell.XCostEQBogus", false, false},
		{"IsTargeting a card", onStack, "Spell.IsTargeting Valid Creature", true, true},
		{"IsTargeting nothing offered", spell, "Spell.IsTargeting Valid Land", false, true},
		{"IsTargeting malformed", spell, "Spell.IsTargeting Defined", false, false},
		{"hasTapCost", ability, "Ability.hasTapCost", true, true},
		{"hasTapCost without params", noParams, "Ability.hasTapCost", false, true},
		{"Loyalty", ability, "Ability.Loyalty", true, true},
		{"Loyalty without params", noParams, "Ability.Loyalty", false, true},
		{"ManaAbility", mana1, "Ability.ManaAbility", true, true},
		{"OppCtrl", ability, "Ability.OppCtrl", true, true},
		{"YouCtrl", ability, "Ability.YouCtrl", false, true},
		{"singleTarget", &Ability{Source: spellCard, spell: true, Targets: []EntityID{CardEntity(target)}}, "Spell.singleTarget", true, true},
		{"keyword property", crew, "Ability.Crew", true, true},
		{"keyword property of another keyword", crew, "Ability.Equip", false, true},
		{"keyword property without params", noParams, "Ability.Equip", false, true},
		{"unresolved prefix", spell, "Spell.Kicked", false, false},
		{"negated property", spell, "Spell.!Kicked", false, false},
		{"Static head", spell, "Static", false, true},
		{"unknown head", spell, "Bogus", false, false},
		{"negated head that fails", ability, "!Spell", true, true},
		{"instant head", spell, "Instant", false, true},
		{"sorcery head", spell, "Sorcery", false, true},
		{"triggered head", &Ability{Source: host, isTrigger: true}, "Triggered", true, true},
		{"activated head", ability, "Activated", true, true},
		{"alternatives", spell, "Activated,Spell.cmcEQ3", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matched, known := g.spellAbilityMatches(tc.a, tc.spec, g.Card(host), p, nil)
			if matched != tc.matched || known != tc.known {
				t.Errorf("spellAbilityMatches(%q) = (%v, %v), want (%v, %v)", tc.spec, matched, known, tc.matched, tc.known)
			}
		})
	}
}
