package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// resolveOnHost resolves line as the Execute$ of host, a battlefield card
// etbChainDef built, with targets already chosen.
func resolveOnHost(t *testing.T, g *engine.Game, p engine.PlayerID, host engine.CardID, targets []engine.EntityID) error {
	t.Helper()
	face := g.Card(host).Def.Faces[0]
	sub := face.Triggers[0].Subs[0].Ability
	api, ok := engine.APIByName(sub.Name)
	if !ok {
		t.Fatalf("unknown API %q", sub.Name)
	}
	g.PushAbility(engine.Ability{API: api, Source: host, Controller: p, Params: sub, Amounts: face.Amounts, Targets: targets})
	return g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController())
}

// ConditionDefined$ counts the ConditionPresent$ matches among the objects it
// names (SpellAbilityCondition.java:348-373) for every effect, not Remembered
// alone: Self, Targeted, a sub-ability's ParentTarget, and an empty list.
func TestConditionDefinedCountsTheNamedObjects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, line  string
		bearsTarget bool
		svars       []string
		gain        int
	}{
		{"Self is present", "DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ Self | ConditionPresent$ Card.Self", false, nil, 3},
		{"Self against EQ0", "DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ Self | ConditionPresent$ Card.Self | ConditionCompare$ EQ0", false, nil, 0},
		{"Targeted matches", "DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ Targeted | ConditionPresent$ Creature.powerGE2", true, nil, 3},
		{"Targeted does not match", "DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ Targeted | ConditionPresent$ Creature.powerGE3", true, nil, 0},
		{"nothing remembered", "DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ Remembered | ConditionPresent$ Card", false, nil, 0},
		{"a sub-ability reads its parent's target",
			"DB$ GainLife | Defined$ You | LifeAmount$ 1 | SubAbility$ DBA", true,
			[]string{"DBA", "DB$ GainLife | Defined$ You | LifeAmount$ 10 | ConditionDefined$ ParentTarget | ConditionPresent$ Creature.powerGE2"}, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			before := g.Player(p).Life
			host := g.NewCard(etbChainDef(t, "Test Condition", tc.line, tc.svars...), p, engine.Battlefield)
			var targets []engine.EntityID
			if tc.bearsTarget {
				targets = []engine.EntityID{engine.CardEntity(g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield))}
			}
			if err := resolveOnHost(t, g, p, host, targets); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life - before; got != tc.gain {
				t.Errorf("life gained = %d, want %d", got, tc.gain)
			}
		})
	}
}

// An LKI spelling counts a card that left the battlefield by its last-known
// state: a remembered 4/4 that died is still a power-4 creature to
// RememberedLKI, while Remembered reads the card in the graveyard.
func TestConditionDefinedLKIReadsALeftCard(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		defined string
		gain    int
	}{{"RememberedLKI", 3}, {"Remembered", 0}} {
		g, p, _ := newTwoPlayerGame(t)
		before := g.Player(p).Life
		host := g.NewCard(etbChainDef(t, "Test LKI",
			"DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ "+tc.defined+" | ConditionPresent$ Creature.powerGE4"), p, engine.Battlefield)
		bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		g.Card(bears).Counters.Add(engine.P1P1, 2)
		g.Card(host).Memory.Remember(engine.CardEntity(bears))
		g.Move(bears, engine.Graveyard, p)
		if err := resolveOnHost(t, g, p, host, nil); err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.defined, err)
		}
		if got := g.Player(p).Life - before; got != tc.gain {
			t.Errorf("%s: life gained = %d, want %d", tc.defined, got, tc.gain)
		}
	}
}

// A ConditionDefined$ the port cannot read fails the ability instead of
// reading as an unmet condition (GO-7): an unknown spelling, a ParentTarget
// with no ancestor that chose targets, a Sacrificed list no cost recorded.
func TestConditionDefinedFailsClosed(t *testing.T) {
	t.Parallel()

	for _, defined := range []string{"Bogus", "ParentTarget", "Sacrificed", "TriggeredCard"} {
		g, p, _ := newTwoPlayerGame(t)
		before := g.Player(p).Life
		host := g.NewCard(etbChainDef(t, "Test Closed",
			"DB$ GainLife | Defined$ You | LifeAmount$ 3 | ConditionDefined$ "+defined+" | ConditionPresent$ Card"), p, engine.Battlefield)
		err := resolveOnHost(t, g, p, host, nil)
		if err == nil || !strings.Contains(err.Error(), "ConditionDefined$") {
			t.Errorf("%s: err = %v, want one naming ConditionDefined$", defined, err)
		}
		if g.Player(p).Life != before {
			t.Errorf("%s: life changed on a failed condition", defined)
		}
	}
}

// ConditionDefined$ Sacrificed counts what the activation cost sacrificed,
// from its last-known state (Self-sacrificing 3/3, ConditionPresent$ power 3+).
func TestConditionDefinedSacrificedReadsTheCostPicks(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 10
	raw := &carddb.Card{Filename: "sac-gainer"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Sac Gainer"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "3", "3"
	raw.Faces[0].Abilities = []string{"AB$ GainLife | Cost$ Sac<1/CARDNAME> | LifeAmount$ 4 | ConditionDefined$ Sacrificed | ConditionPresent$ Creature.powerGE3"}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	gainer := g.NewCard(def, p, engine.Battlefield)
	g.Card(gainer).SummonSick = false
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, gainer, 0, c) {
		t.Fatal("could not activate")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 14 {
		t.Errorf("life = %d, want 14", got)
	}
}
