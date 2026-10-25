package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestEffectsRefuseWhileTheirReplacementEventIsLive proves
// Proliferate, Learn, AssembleContraption and Tap fail while a replacement for their event is in
// play, instead of ignoring it (GO-7). Learn's host sits in a graveyard
// (ActiveZones$ Graveyard, Retriever Phoenix).
func TestEffectsRefuseWhileTheirReplacementEventIsLive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		event, line string
		zone        engine.ZoneType
	}{
		{"Proliferate", "DB$ Proliferate", engine.Battlefield},
		{"Learn", "DB$ Learn", engine.Graveyard},
		{"AssembleContraption", "DB$ AssembleContraption", engine.Battlefield},
		{"Tap", "DB$ Tap | Defined$ Self", engine.Battlefield},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			host := g.NewCard(planarDef(t, "Replacer", "Creature Elf", nil, []string{
				"Event$ " + tt.event + " | ActiveZones$ Battlefield,Graveyard | ReplaceWith$ Plus",
			}, "Plus", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, tt.zone)

			_, err := castETBChain(t, g, p, etbChainDef(t, "Actor", tt.line), engine.NewScriptedController())
			if err == nil || !strings.Contains(err.Error(), tt.event+" replacement") {
				t.Fatalf("err = %v, want %s replacement refused", err, tt.event)
			}
			g.Move(host, engine.Exile, p)
		})
	}
}
