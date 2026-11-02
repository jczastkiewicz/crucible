package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// chainActivatedDef builds a 2/2 whose one activated ability is head, with
// SVar Sub as its SubAbility$ link.
func chainActivatedDef(t *testing.T, head, sub string) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: "Test Chain"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Chain"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].Abilities = []string{head}
	raw.Faces[0].SVars.Set("Sub", sub)
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestSubAbilityTargetsAreRecheckedAtResolution proves CR 608.2b over a
// SubAbility$ chain (MagicStack.hasFizzled recurses into the sub-ability): an
// ability whose only target is its link's and which is gone fizzles whole, a
// legal link target keeps the ability going, and a head target that is still
// legal keeps the head resolving while the link's gone target is dropped.
func TestSubAbilityTargetsAreRecheckedAtResolution(t *testing.T) {
	t.Parallel()

	const sub = "DB$ Destroy | ValidTgts$ Creature.OppCtrl | TgtPrompt$ Select target creature"
	tests := []struct {
		name       string
		head       string
		targets    int // head targets queued before the link's
		removeLink bool
		wantPump   bool
		wantKilled bool
	}{
		{"link target gone fizzles the ability", "AB$ Pump | Cost$ 0 | Defined$ Self | NumAtt$ 1 | SubAbility$ Sub", 0, true, false, false},
		{"link target legal resolves", "AB$ Pump | Cost$ 0 | Defined$ Self | NumAtt$ 1 | SubAbility$ Sub", 0, false, true, true},
		{"head target legal and link target gone", "AB$ Pump | Cost$ 0 | ValidTgts$ Creature.YouCtrl | NumAtt$ 1 | SubAbility$ Sub", 1, true, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			host := g.NewCard(chainActivatedDef(t, tc.head, sub), p, engine.Battlefield)
			victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

			c := engine.NewScriptedController()
			if tc.targets > 0 {
				c.QueueTargets([]engine.EntityID{engine.CardEntity(host)})
			}
			c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
			if !g.ActivateAbility(p, host, 0, c) {
				t.Fatal("ActivateAbility failed")
			}
			if tc.removeLink {
				g.Move(victim, engine.Graveyard, other)
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if pw, _ := g.Card(host).Power(); (pw == 3) != tc.wantPump {
				t.Errorf("host power = %d, pumped want %v", pw, tc.wantPump)
			}
			if killed := g.Card(victim).Zone == engine.Graveyard; killed != (tc.wantKilled || tc.removeLink) {
				t.Errorf("victim in graveyard = %v", killed)
			}
		})
	}
}

// Mutual Destruction targeting the creature it sacrifices as its cost is cast
// and fizzles on resolution. Java's MagicStack.add re-checks only the target
// count before the spell is cast (hasLegalTargeting, MagicStack.java:787-795,
// abortIllegalAddition); legality waits for hasFizzled (:797-820, CR 608.2b).
func TestSpellTargetingItsOwnSacrificeIsCastAndFizzles(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	spell := g.NewCard(corpusCard(t, "Mutual Destruction"), p, engine.Hand)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	c.QueueSacrificeChoice([]engine.CardID{bear})
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("Mutual Destruction not cast: %v", g.TakePendingError())
	}
	if got := g.Card(spell).Zone; got != engine.Stack {
		t.Fatalf("Mutual Destruction in %v, want on the stack", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(spell).Zone; got != engine.Graveyard {
		t.Errorf("Mutual Destruction in %v, want the graveyard", got)
	}
	if got := g.Card(bear).Zone; got != engine.Graveyard {
		t.Errorf("Grizzly Bears in %v, want sacrificed", got)
	}
}
