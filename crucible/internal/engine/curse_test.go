package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// castCurse puts a synthetic Aura with the given Enchant keyword in p's hand,
// casts it with queued targets and resolves it, reporting the card.
func castCurse(t *testing.T, g *engine.Game, p engine.PlayerID, enchant string, targets ...engine.EntityID) (engine.CardID, bool) {
	t.Helper()
	card := g.NewCard(scriptDef(t, "Test Curse", "Enchantment Aura Curse", "K:Enchant:"+enchant), p, engine.Hand)
	c := engine.NewScriptedController()
	if len(targets) > 0 {
		c.QueueTargets(targets)
	}
	cast := g.CastSpell(p, card, c)
	if cast {
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	return card, cast
}

// CR 303.4h: an Aura with "Enchant player" targets a player as it is cast and
// enchants them on resolution; Player.Attachments is the reverse of
// Card.AttachedToPlayer.
func TestEnchantPlayerAuraEnchantsTheTargetedPlayer(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	card, cast := castCurse(t, g, p, "Player", engine.PlayerEntity(opp))
	if !cast {
		t.Fatal("the Curse could not be cast")
	}
	if got, ok := g.Card(card).AttachedToPlayer(); !ok || got != opp {
		t.Errorf("AttachedToPlayer = %v, %v; want %v, true", got, ok, opp)
	}
	if !slices.Equal(g.Player(opp).Attachments(), []engine.CardID{card}) {
		t.Errorf("opponent attachments = %v, want [%v]", g.Player(opp).Attachments(), card)
	}
	if _, ok := g.Card(card).AttachedTo(); ok {
		t.Error("the Curse is attached to a card as well")
	}
	if g.Card(card).Zone != engine.Battlefield {
		t.Errorf("Curse zone = %v, want Battlefield", g.Card(card).Zone)
	}
}

// "Enchant player" lets its caster enchant themselves.
func TestEnchantPlayerAuraMayTargetItsController(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	card, cast := castCurse(t, g, p, "Player", engine.PlayerEntity(p))
	if !cast {
		t.Fatal("the Curse could not be cast")
	}
	if got, _ := g.Card(card).AttachedToPlayer(); got != p {
		t.Errorf("enchanted player = %v, want its controller %v", got, p)
	}
}

// "Enchant opponent" has one legal target in a two-player game, so no choice
// is asked (the scripted controller has no answer queued and would panic).
func TestEnchantOpponentAuraTargetsTheOnlyOpponent(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	card, cast := castCurse(t, g, p, "Opponent")
	if !cast {
		t.Fatal("the Curse could not be cast")
	}
	if got, _ := g.Card(card).AttachedToPlayer(); got != opp {
		t.Errorf("enchanted player = %v, want the opponent %v", got, opp)
	}
}

// CR 702.11b: a player with hexproof cannot be the target of an opponent's
// Aura spell, so with no other legal player the Curse cannot be cast.
func TestEnchantPlayerAuraCannotTargetAHexproofOpponent(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(opp).KeywordMod.Add(engine.KeywordEffect{AddKeywords: []string{"Hexproof"}})
	if _, cast := castCurse(t, g, p, "Opponent"); cast {
		t.Error("an Enchant opponent Aura was cast against a hexproof opponent")
	}
}

// CR 704.5m: an Aura enchanting a player that left the game goes to its
// owner's graveyard, and the player's attachment list is emptied with it. A
// third player keeps the game going.
func TestCurseOnAPlayerWhoLeftTheGameGoesToTheGraveyard(t *testing.T) {
	t.Parallel()

	g, p, opp, _ := newThreePlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	card, cast := castCurse(t, g, p, "Player", engine.PlayerEntity(opp))
	if !cast {
		t.Fatal("the Curse could not be cast")
	}
	g.Player(opp).Lost = true
	sba(g)
	if g.Card(card).Zone != engine.Graveyard {
		t.Errorf("Curse zone = %v, want Graveyard", g.Card(card).Zone)
	}
	if got := g.Player(opp).Attachments(); len(got) != 0 {
		t.Errorf("opponent attachments = %v, want none", got)
	}
}

// CR 704.5m: "Enchant opponent" is checked as state, not only when cast. A
// Curse that ends up enchanting its own controller (here forced with
// AttachToPlayer, as a control change would) goes to the graveyard.
func TestEnchantOpponentAuraOnItsOwnControllerGoesToTheGraveyard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	card, cast := castCurse(t, g, p, "Opponent")
	if !cast {
		t.Fatal("the Curse could not be cast")
	}
	g.AttachToPlayer(card, p)
	sba(g)
	if g.Card(card).Zone != engine.Graveyard {
		t.Errorf("Curse zone = %v, want Graveyard", g.Card(card).Zone)
	}
}

// CR 704.5n: anything but an Aura attached to a player becomes unattached and
// stays on the battlefield.
func TestEquipmentAttachedToAPlayerBecomesUnattached(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	eq := g.NewCard(scriptDef(t, "Test Equipment", "Artifact Equipment"), p, engine.Battlefield)
	g.AttachToPlayer(eq, opp)
	sba(g)
	if _, ok := g.Card(eq).AttachedToPlayer(); ok {
		t.Error("an Equipment is still attached to a player")
	}
	if g.Card(eq).Zone != engine.Battlefield {
		t.Errorf("Equipment zone = %v, want Battlefield", g.Card(eq).Zone)
	}
	if got := g.Player(opp).Attachments(); len(got) != 0 {
		t.Errorf("opponent attachments = %v, want none", got)
	}
}

// Leaving the battlefield detaches the Curse from the player it enchanted.
func TestACurseThatLeavesTheBattlefieldIsNoLongerAttachedToThePlayer(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	card, cast := castCurse(t, g, p, "Player", engine.PlayerEntity(opp))
	if !cast {
		t.Fatal("the Curse could not be cast")
	}
	g.Move(card, engine.Exile, p)
	if got := g.Player(opp).Attachments(); len(got) != 0 {
		t.Errorf("opponent attachments = %v, want none", got)
	}
}

// Player.EnchantedBy in a trigger's ValidPlayer$ (Curse of Thirst's shape):
// the Curse fires on the enchanted player's upkeep only.
func TestTriggerValidPlayerEnchantedByFiresForTheEnchantedPlayerOnly(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	card := g.NewCard(scriptDef(t, "Test Curse", "Enchantment Aura Curse", "K:Enchant:Player",
		"T:Mode$ Phase | Phase$ Upkeep | ValidPlayer$ Player.EnchantedBy | TriggerZones$ Battlefield | Execute$ TrigDmg",
		"SVar:TrigDmg:DB$ LoseLife | Defined$ EnchantedPlayer | LifeAmount$ 2"), p, engine.Battlefield)
	g.AttachToPlayer(card, opp)
	c := engine.NewScriptedController()
	for _, active := range []engine.PlayerID{p, opp} {
		g.SetTurnState(1, active, engine.Untap)
		if err := g.Step(engine.NewRegistry(), c); err != nil {
			t.Fatalf("Step: %v", err)
		}
	}
	if got := g.Player(opp).Life; got != 18 {
		t.Errorf("enchanted player's life = %d, want 18", got)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("Curse controller's life = %d, want 20", got)
	}
}
