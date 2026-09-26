package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// instantDefWithSubAbility is instantDefWithAbility plus one SVar, for a
// spell whose fizzle has to be observed through a sub-ability that does
// not target (CR 608.2b: none of the spell's effects happen).
func instantDefWithSubAbility(t *testing.T, name, abilityText, svar, svarText string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Instant")
	raw.Faces[0].ManaCost = mana.MustParse("0")
	raw.Faces[0].Abilities = []string{abilityText}
	raw.Faces[0].SVars.Set(svar, svarText)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestFizzleWhenTargetChangedZones is CR 400.7 under CR 608.2b: a creature
// that left the battlefield and came back after being targeted is a new
// object, so the spell targeting the old one has no legal target and does
// nothing -- Java's equalsWithGameTimestamp check (MagicStack.java:716-722).
func TestFizzleWhenTargetChangedZones(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	ping := g.NewCard(instantDefWithAbility(t, "Test Ping", "0", "SP$ DealDamage | ValidTgts$ Creature | NumDmg$ 1"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(creature)})
	if !g.CastSpell(p, ping, c) {
		t.Fatal("cast failed")
	}
	g.Move(creature, engine.Graveyard, other)
	g.Move(creature, engine.Battlefield, other)

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(creature).Damage.Marked; got != 0 {
		t.Errorf("damage on the returned creature = %d, want 0 (the spell fizzled)", got)
	}
	if got := g.Card(ping).Zone; got != engine.Graveyard {
		t.Errorf("fizzled spell zone = %v, want Graveyard (CR 608.2b)", got)
	}
}

// TestFizzleDropsOnlyIllegalTargets is CR 608.2b's partial case: with one
// of two targets gone, the spell still resolves, for the legal one only
// (MagicStack.java:748-750 strips the illegal target first).
func TestFizzleDropsOnlyIllegalTargets(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	kept := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	flickered := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	spray := g.NewCard(instantDefWithAbility(t, "Test Spray", "0",
		"SP$ DealDamage | ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2 | NumDmg$ 1"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(kept), engine.CardEntity(flickered)})
	if !g.CastSpell(p, spray, c) {
		t.Fatal("cast failed")
	}
	g.Move(flickered, engine.Exile, other)
	g.Move(flickered, engine.Battlefield, other)

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(kept).Damage.Marked; got != 1 {
		t.Errorf("legal target damage = %d, want 1", got)
	}
	if got := g.Card(flickered).Damage.Marked; got != 0 {
		t.Errorf("flickered target damage = %d, want 0 (dropped as illegal)", got)
	}
}

// TestFizzleWhenTargetNoLongerMatchesValidTgts: CR 608.2b re-checks the
// targeting restriction itself, against the object as it is now
// (SpellAbility.canTarget's isValid, SpellAbility.java:1591-1594). A spell
// that may only target an untapped creature fizzles once its target taps.
func TestFizzleWhenTargetNoLongerMatchesValidTgts(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	ping := g.NewCard(instantDefWithAbility(t, "Test Ping", "0", "SP$ DealDamage | ValidTgts$ Creature.untapped | NumDmg$ 1"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(creature)})
	if !g.CastSpell(p, ping, c) {
		t.Fatal("cast failed")
	}
	g.Card(creature).Tapped = true

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(creature).Damage.Marked; got != 0 {
		t.Errorf("damage = %d, want 0 (target no longer untapped, the spell fizzled)", got)
	}
}

// TestFizzleWhenPlayerTargetLeftTheGame: a player who lost is no longer a
// legal target (Player.canBeTargetedBy's hasLost check,
// Player.java:1033-1043).
func TestFizzleWhenPlayerTargetLeftTheGame(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, c3 := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life, g.Player(c3).Life = 20, 20, 20
	gain := g.NewCard(instantDefWithSubAbility(t, "Test Gift",
		"SP$ GainLife | ValidTgts$ Player | LifeAmount$ 5 | SubAbility$ DBGain",
		"DBGain", "DB$ GainLife | LifeAmount$ 1"), a, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})
	if !g.CastSpell(a, gain, c) {
		t.Fatal("cast failed")
	}
	g.Player(b).Life = 0
	engine.CheckStateBasedActions(g, c)
	if !g.Player(b).Lost {
		t.Fatal("setup: b did not lose")
	}

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(a).Life; got != 20 {
		t.Errorf("caster life = %d, want 20 (the spell fizzled, its sub-ability with it)", got)
	}
}
