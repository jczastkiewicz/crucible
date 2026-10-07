package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// otherCanTarget reports whether victim is among the candidates of a targeting
// sorcery other casts.
func otherCanTarget(t *testing.T, g *engine.Game, other engine.PlayerID, victim engine.CardID) bool {
	t.Helper()
	spell := g.NewCard(scriptDef(t, "Kill Spell", "Sorcery", "A:SP$ Destroy | ValidTgts$ Creature"), other, engine.Hand)
	g.SetTurnState(2, other, engine.Main1)
	c := &recordingTargets{ScriptedController: engine.NewScriptedController()}
	c.QueueTargets(entities(victim))
	g.CastSpell(other, spell, c)
	return slices.Contains(c.candidates, engine.CardEntity(victim))
}

// Pump KW$ with ChosenPlayerUID and DefinedKW$ (Courageous Resolve, Cliffside
// Rescuer, PumpEffect.java:318-371): the keyword is written once per player the
// DefinedKW$ names.
func TestPumpDefinedKWGrantsProtectionFromThePlayersNamed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		kw          string
		wantProtect bool
	}{
		{"from each opponent", "Protection:Player.PlayerUID_ChosenPlayerUID:ChosenPlayerName | DefinedKW$ Opponent", true},
		{"from opponents of you", "Protection:Player.OpponentOf PlayerUID_ChosenPlayerUID:all opponents of ChosenPlayerName | DefinedKW$ You", true},
		{"from yourself", "Protection:Player.PlayerUID_ChosenPlayerUID:ChosenPlayerName | DefinedKW$ You", false},
		{"no chosen player does nothing", "Protection:Player.PlayerUID_ChosenPlayerUID:ChosenPlayerName | DefinedKW$ ChosenPlayer", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := restrictionGame(t)
			victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			c.QueueTargets(entities(victim))
			resolveLine(t, g, p, c, "DB$ Pump | ValidTgts$ Creature.YouCtrl | KW$ "+tc.kw)
			if got := !otherCanTarget(t, g, other, victim); got != tc.wantProtect {
				t.Errorf("creature protected from the opponent = %v, want %v", got, tc.wantProtect)
			}
		})
	}
}
