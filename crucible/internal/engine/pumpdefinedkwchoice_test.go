package engine_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// DefinedKW$ ChosenType and ChosenPlayer read what the host (here the spell)
// chose (PumpEffect.java:320-340); a host that chose nothing pumps nothing.
func TestPumpDefinedKWReadsTheHostsChoice(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		defined string
		kw      string
		choose  func(g *engine.Game, host engine.CardID, other engine.PlayerID)
		want    func(g *engine.Game, other engine.PlayerID) string
	}{
		{"chosen type", "ChosenType", "Protection:Creature.ChosenType:chosen",
			func(g *engine.Game, h engine.CardID, _ engine.PlayerID) { g.Card(h).Memory.SetChosenType("Elf", false) },
			func(*engine.Game, engine.PlayerID) string { return "Protection:Creature.Elf:chosen" }},
		{"chosen player", "ChosenPlayer", "Protection:Player.PlayerUID_ChosenPlayerUID:ChosenPlayerName",
			func(g *engine.Game, h engine.CardID, o engine.PlayerID) { g.Card(h).Memory.SetChosenPlayer(o) },
			func(g *engine.Game, o engine.PlayerID) string {
				return "Protection:Player.PlayerUID_" + strconv.Itoa(int(o)) + ":" + g.Player(o).Name
			}},
		{"nothing chosen", "ChosenType", "Protection:Creature.ChosenType:chosen",
			func(*engine.Game, engine.CardID, engine.PlayerID) {},
			func(*engine.Game, engine.PlayerID) string { return "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := restrictionGame(t)
			victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			spell := g.NewCard(scriptDef(t, "Defined Spell", "Instant",
				"A:SP$ Pump | ValidTgts$ Creature | KW$ "+tc.kw+" | DefinedKW$ "+tc.defined), p, engine.Hand)
			tc.choose(g, spell, other)
			c := engine.NewScriptedController()
			c.QueueTargets(entities(victim))
			if !g.CastSpell(p, spell, c) {
				t.Fatal("CastSpell = false, want true")
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			lines := g.Card(victim).KeywordLines()
			want := tc.want(g, other)
			if want == "" && len(lines) != 0 {
				t.Errorf("keywords = %q, want none", lines)
			}
			if want != "" && !slices.Contains(lines, want) {
				t.Errorf("keywords = %q, want %q among them", lines, want)
			}
		})
	}
}

// DefinedKW$ ChosenColor with no single color chosen is an error, never a guess.
func TestPumpDefinedKWChosenColorNeedsOneColor(t *testing.T) {
	t.Parallel()

	g, p, _ := restrictionGame(t)
	victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	spell := g.NewCard(scriptDef(t, "Color Spell", "Instant",
		"A:SP$ Pump | ValidTgts$ Creature | KW$ Protection:Card.ChosenColor:chosenColor | DefinedKW$ ChosenColor"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets(entities(victim))
	if !g.CastSpell(p, spell, c) {
		t.Fatal("CastSpell = false, want true")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err == nil {
		t.Error("ResolveStack error = nil, want the unresolvable ChosenColor")
	}
}
