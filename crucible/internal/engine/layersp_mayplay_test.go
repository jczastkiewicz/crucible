package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Layer 8 MayPlay$ past the first slice: the mana relaxation
// (MayPlayIgnoreColor$), the cost raise (RaiseCost$), the spell-ability
// filters (ValidSA$, ValidAfterStack$), ReplaceGraveyard$, and the
// IsPresent$/CheckSVar$ conditions routed through layerStaticApplies. Each
// runs through a real or minimal static line and the public cast API.

// exileGrant is a game with a battlefield static granting MayPlay$ over p's
// exiled cards, plus one exiled creature of the given cost.
func exileGrant(t *testing.T, params, cost string) (*engine.Game, engine.PlayerID, engine.CardID) {
	t.Helper()
	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(continuousDef(t, "Grant Source",
		"Mode$ Continuous | Affected$ Card.YouOwn | AffectedZone$ Exile | MayPlay$ True | "+params),
		p, engine.Battlefield)
	card := g.NewCard(creatureDefCost(t, "Exiled Creature", cost), p, engine.Exile)
	sba(g)
	return g, p, card
}

// payWith is a controller that pays each generic unit with the given mana
// types, in order.
func payWith(shards ...mana.Shard) *engine.ScriptedController {
	c := engine.NewScriptedController()
	for _, s := range shards {
		c.QueuePayGeneric(s)
	}
	return c
}

func TestMayPlayIgnoreColorLetsAnyManaPayAColoredCost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		params  string
		cost    string
		pool    func(*engine.Pool)
		generic mana.Shard
		want    bool
	}{
		{"colorless pays a colored shard", "MayPlayIgnoreColor$ True", "G", func(p *engine.Pool) { p.AddColorless(1) }, mana.ShardC, true},
		{"without the param it does not", "MayPlayWithFlash$ True", "G", func(p *engine.Pool) { p.AddColorless(1) }, mana.ShardC, false},
		{"a {C} shard still needs colorless", "MayPlayIgnoreColor$ True", "C", func(p *engine.Pool) { p.Add(mana.Green, 1) }, mana.ShardG, false},
		{"ignore type pays {C} with anything", "MayPlayIgnoreType$ True", "C", func(p *engine.Pool) { p.Add(mana.Green, 1) }, mana.ShardG, true},
		{"type wins over color", "MayPlayIgnoreColor$ True | MayPlayIgnoreType$ True", "C", func(p *engine.Pool) { p.Add(mana.Green, 1) }, mana.ShardG, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, card := exileGrant(t, tc.params, tc.cost)
			tc.pool(&g.Player(p).ManaPool)
			if got := g.CastSpell(p, card, payWith(tc.generic)); got != tc.want {
				t.Errorf("CastSpell = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMayPlayRaiseCostIsPaidOnTopOfTheCost(t *testing.T) {
	t.Parallel()
	t.Run("life", func(t *testing.T) {
		t.Parallel()
		g, p, card := exileGrant(t, "RaiseCost$ PayLife<3>", "G")
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if !g.CastSpell(p, card, engine.NewScriptedController()) {
			t.Fatal("CastSpell = false")
		}
		if got := g.Player(p).Life; got != 17 {
			t.Errorf("life = %d, want 17", got)
		}
	})
	t.Run("life that cannot be paid declines the cast", func(t *testing.T) {
		t.Parallel()
		g, p, card := exileGrant(t, "RaiseCost$ PayLife<3>", "G")
		g.Player(p).Life = 2
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if g.CastSpell(p, card, engine.NewScriptedController()) {
			t.Error("cast while unable to pay the raised life cost")
		}
		if g.Card(card).Zone != engine.Exile {
			t.Errorf("card in %v, want exile", g.Card(card).Zone)
		}
	})
	t.Run("mana", func(t *testing.T) {
		t.Parallel()
		g, p, card := exileGrant(t, "RaiseCost$ 2", "G")
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if g.CastSpell(p, card, payWith(mana.ShardG, mana.ShardG)) {
			t.Fatal("cast without paying the raised mana")
		}
		g.Player(p).ManaPool.AddColorless(2)
		if !g.CastSpell(p, card, payWith(mana.ShardC, mana.ShardC)) {
			t.Error("CastSpell with the raised mana = false")
		}
	})
	t.Run("a free cast still pays the raise", func(t *testing.T) {
		t.Parallel()
		g, p, card := exileGrant(t, "MayPlayWithoutManaCost$ True | RaiseCost$ PayLife<1>", "7 G")
		if !g.CastSpell(p, card, engine.NewScriptedController()) {
			t.Fatal("CastSpell = false")
		}
		if got := g.Player(p).Life; got != 19 {
			t.Errorf("life = %d, want 19", got)
		}
	})
	t.Run("an SVar name is its amount in generic mana", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(continuousDefWithSVar(t, "Svar Raise",
			"Mode$ Continuous | Affected$ Card.YouOwn | AffectedZone$ Exile | MayPlay$ True | RaiseCost$ X", "X", "Number$2"),
			p, engine.Battlefield)
		card := g.NewCard(creatureDefCost(t, "Exiled Creature", "G"), p, engine.Exile)
		sba(g)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if g.CastSpell(p, card, payWith(mana.ShardG, mana.ShardG)) {
			t.Fatal("cast without the two extra mana")
		}
		g.Player(p).ManaPool.AddColorless(2)
		if !g.CastSpell(p, card, payWith(mana.ShardC, mana.ShardC)) {
			t.Error("CastSpell with X = 2 paid = false")
		}
	})
	t.Run("a cost shape this port cannot pay grants nothing", func(t *testing.T) {
		t.Parallel()
		g, p, card := exileGrant(t, "RaiseCost$ Forage", "G")
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if g.CastSpell(p, card, engine.NewScriptedController()) {
			t.Error("cast under a RaiseCost$ it cannot pay")
		}
	})
}

func TestMayPlayValidAfterStackAndValidSA(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		params string
		cost   string
		want   bool
	}{
		{"cmc within the filter", "ValidAfterStack$ Spell.cmcLE2", "1 G", true},
		{"cmc past the filter", "ValidAfterStack$ Spell.cmcLE2", "2 G", false},
		{"a plain ValidSA$ Spell is every cast", "ValidSA$ Spell", "G", true},
		{"an alternative cast the port cannot make", "ValidSA$ Spell.Blitz", "G", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, card := exileGrant(t, tc.params, tc.cost)
			g.Player(p).ManaPool.Add(mana.Green, 1)
			g.Player(p).ManaPool.AddColorless(2)
			if got := g.CastSpell(p, card, payWith(mana.ShardC, mana.ShardC)); got != tc.want {
				t.Errorf("CastSpell = %v, want %v", got, tc.want)
			}
		})
	}
}

// Kess, Dissident Mage: an instant or sorcery from the graveyard on your
// turn, exiled instead of going back to the graveyard.
func TestMayPlayReplaceGraveyardExilesTheSpell(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Kess, Dissident Mage"), p, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Divination"), p, engine.Graveyard)
	g.NewCard(corpusCard(t, "Forest"), p, engine.Library)
	g.NewCard(corpusCard(t, "Forest"), p, engine.Library)
	sba(g)

	c := payWith(mana.ShardU, mana.ShardU)
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	if !g.CastSpell(p, spell, c) {
		t.Fatal("CastSpell(Divination from the graveyard) = false")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(spell).Zone; got != engine.Exile {
		t.Errorf("resolved spell in %v, want exile", got)
	}
}

// Gravecrawler's "cast from your graveyard as long as you control a Zombie":
// the host sits in the graveyard, so its EffectZone$ Graveyard line is walked
// there, and IsPresent$ gates it.
func TestMayPlayFromAGraveyardHostWhileTheConditionHolds(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	crawler := g.NewCard(corpusCard(t, "Gravecrawler"), p, engine.Graveyard)
	sba(g)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	if g.CastSpell(p, crawler, c) {
		t.Fatal("cast Gravecrawler with no Zombie")
	}
	g.NewCard(corpusCard(t, "Walking Corpse"), p, engine.Battlefield)
	sba(g)
	if !g.CastSpell(p, crawler, c) {
		t.Fatal("CastSpell(Gravecrawler) with a Zombie = false")
	}
	if g.Card(crawler).Zone != engine.Stack {
		t.Errorf("Gravecrawler in %v, want the stack", g.Card(crawler).Zone)
	}
}

// IsPresent$ and the full CheckSVar$ chain reach MayPlay$ through
// layerStaticApplies.
func TestMayPlayConditionsRouteThroughTheStaticGate(t *testing.T) {
	t.Parallel()
	g, p, card := exileGrant(t, "IsPresent$ Creature.YouCtrl+Other", "G")
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	if g.CastSpell(p, card, c) {
		t.Fatal("cast with the IsPresent$ condition false")
	}
	g.NewCard(creatureDefCost(t, "Bystander", "G"), p, engine.Battlefield)
	sba(g)
	if !g.CastSpell(p, card, c) {
		t.Error("CastSpell with the IsPresent$ condition true = false")
	}
}
