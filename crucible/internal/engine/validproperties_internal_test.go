package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): one table walks the card and player properties of a valid
// string through Matches/matchesPlayerSpec; a scenario per property would
// need a card script per spelling.

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

func TestMatchesCardPropertiesReadSourceAndCardState(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	def := targetCandidatesTestCreature(t)
	src := g.NewCard(def, p, Battlefield)
	card := g.NewCard(def, other, Battlefield)
	g.Card(card).Suspected = true
	g.Card(card).monstrous = true
	g.Card(card).kicker = kicker1

	for _, tc := range []struct {
		spec string
		want bool
	}{
		{"Card.IsSuspected", true},
		{"Card.IsMonstrous", true},
		{"Card.faceUp", true},
		{"Card.kicked 1", true},
		{"Card.kicked 2", false},
		{"Card.EnchantedPlayerCtrl", false},
		{"Card.EnchantedPlayer", false},
		{"Card.ChosenType", false},
		{"Card.IsNotChosenType", true},
	} {
		if got := Matches(g, g.Card(card), valid.Parse(tc.spec), p, src); got != tc.want {
			t.Errorf("Matches(%q) = %v, want %v", tc.spec, got, tc.want)
		}
	}

	g.Card(src).Memory.SetChosenType("Elf", false)
	if !Matches(g, g.Card(card), valid.Parse("Card.ChosenType"), p, src) || Matches(g, g.Card(card), valid.Parse("Card.IsNotChosenType"), p, src) {
		t.Error("an Elf is the chosen type: ChosenType must match and IsNotChosenType must not")
	}
	g.Card(src).Memory.SetChosenType("NonElf", false)
	if Matches(g, g.Card(card), valid.Parse("Card.ChosenType"), p, src) {
		t.Error("chosen NonElf: an Elf must not match ChosenType")
	}
}

func TestMatchesPlayerPropertiesReadCounters(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	src := g.NewCard(targetCandidatesTestCreature(t), p, Battlefield)
	g.Player(other).Counters.Add(Poison, 3)
	g.Card(src).Memory.Remember(PlayerEntity(other))

	for _, tc := range []struct {
		spec      string
		candidate PlayerID
		want      bool
	}{
		{"Player.IsPoisoned", other, true},
		{"Player.IsPoisoned", p, false},
		{"Player.IsCorrupted", other, true},
		{"Player.IsCorrupted", p, false},
		{"Player.IsRememberedOrController", other, true},
		{"Player.IsRememberedOrController", p, false},
		{"Player.EnchantedController", other, false},
	} {
		matched, ok := matchesPlayerSpec(g, tc.candidate, p, src, tc.spec)
		if !ok || matched != tc.want {
			t.Errorf("matchesPlayerSpec(%q, %v) = (%v, %v), want (%v, true)", tc.spec, tc.candidate, matched, ok, tc.want)
		}
	}
}
