package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Discord, Lord of Disharmony: at its controller's end step it names a random
// nonland card and an effect card in the Command zone lets them cast a copy of
// that card until their next end step.
func TestDiscordNamesARandomCardAtTheEndStep(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	discord := g.NewCard(corpusCard(t, "Discord, Lord of Disharmony"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	var effect engine.CardID
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).IsEffect {
			effect = id
		}
	}
	if effect == 0 {
		t.Fatal("no effect card in the Command zone")
	}
	if got := len(g.Card(effect).Memory.NamedCards()); got != 1 {
		t.Fatalf("effect card names %d cards, want 1", got)
	}
	if got := len(g.Card(discord).Memory.NamedCards()); got != 0 {
		t.Errorf("Discord still names %d cards, want 0 (ClearNamedCard$)", got)
	}
}

// The granted ability casts a copy of the named card (here a pinned
// Ornithopter): it is cast and resolves, the effect card is gone, and while
// Discord stays on the battlefield the chain grants itself again.
func TestDiscordCastsACopyOfTheNamedSpellAndRegrants(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Discord, Lord of Disharmony"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	var effect engine.CardID
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).IsEffect {
			effect = id
		}
	}
	if effect == 0 {
		t.Fatal("no effect card in the Command zone")
	}
	g.Card(effect).Memory.ClearNamedCards()
	g.Card(effect).Memory.AddNamedCard("Ornithopter")

	// Sorcery timing for a creature spell: the owner's next main phase.
	libraryCards(t, g, p, 5)
	libraryCards(t, g, g.Players()[1], 5)
	toStep(t, g, c, p, engine.Main1, nil)
	c.QueueConfirmEffect(true)
	if !g.ActivateAbility(p, effect, 0, c) {
		t.Fatalf("ActivateAbility returned false (pending error: %v)", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(effect).Zone; got == engine.Command {
		t.Errorf("the effect card is still in the Command zone")
	}
	found := false
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).Def.Name == "Ornithopter" {
			found = true
		}
	}
	if !found {
		t.Error("no Ornithopter on the battlefield")
	}
	regranted := 0
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).IsEffect && id != effect {
			regranted++
		}
	}
	if regranted != 1 {
		t.Errorf("%d new effect cards after the cast, want 1 (Discord is on the battlefield)", regranted)
	}
}

// discordCasts runs Discord's end step trigger, pins the named card to
// Ornithopter and casts its copy at the next main phase, moving Discord to the
// graveyard first when gone is set. It returns the effect cards left in the
// Command zone.
func discordCasts(t *testing.T, casts int, gone bool) int {
	t.Helper()
	g, p, o := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	discord := g.NewCard(corpusCard(t, "Discord, Lord of Disharmony"), p, engine.Battlefield)
	libraryCards(t, g, p, 10)
	libraryCards(t, g, o, 10)
	c := engine.NewScriptedController()
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if gone {
		if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(discord)}, "DB$ Destroy | ValidTgts$ Creature"); err != nil {
			t.Fatalf("destroy Discord: %v", err)
		}
	}
	for range casts {
		var effect engine.CardID
		for _, id := range g.Zone(engine.Command, p).Cards() {
			if g.Card(id).IsEffect {
				effect = id
			}
		}
		if effect == 0 {
			t.Fatal("no effect card to cast through")
		}
		g.Card(effect).Memory.ClearNamedCards()
		g.Card(effect).Memory.AddNamedCard("Ornithopter")
		if g.ActivePhase() != engine.Main1 || g.ActivePlayer() != p {
			toStep(t, g, c, p, engine.Main1, nil)
		}
		c.QueueConfirmEffect(true)
		if !g.ActivateAbility(p, effect, 0, c) {
			t.Fatalf("ActivateAbility returned false (pending error: %v)", g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("pending error: %v", err)
		}
	}
	n := 0
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).IsEffect {
			n++
		}
	}
	return n
}

// The chain keeps granting itself cast after cast while Discord stays on the
// battlefield (the second pass reads OriginalHost as Discord), and stops once
// it has left.
func TestDiscordRegrantsWhileItStaysOnTheBattlefield(t *testing.T) {
	t.Parallel()

	if got := discordCasts(t, 2, false); got != 1 {
		t.Errorf("after two casts %d effect cards, want 1 (the third grant)", got)
	}
	if got := discordCasts(t, 1, true); got != 0 {
		t.Errorf("after Discord left, %d effect cards after the cast, want 0", got)
	}
}
