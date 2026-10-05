package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// wardedCreatureDef is a 3/3 creature with one keyword line (a Ward) and the
// SVars it names.
func wardedCreatureDef(t *testing.T, keyword string, svars ...string) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: "Warded"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Warded"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "3", "3"
	raw.Faces[0].Keywords = []string{keyword}
	for i := 0; i+1 < len(svars); i += 2 {
		raw.Faces[0].SVars.Set(svars[i], svars[i+1])
	}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", keyword, err)
	}
	return c
}

// TestWardOnAnActivatedAbility is CR 702.21a's "spell or ability an opponent
// controls": an activated ability that targets the warded creature raises a
// Ward trigger. Declining the cost counters the ability (the stack item,
// never a card: its source stays on the battlefield, still tapped for its
// cost); paying lets it resolve.
func TestWardOnAnActivatedAbility(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		pay        bool
		wantTarget engine.ZoneType
		wantLife   int
	}{
		{"declined", false, engine.Battlefield, 20},
		{"paid", true, engine.Graveyard, 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, opp := newTwoPlayerGame(t)
			target := g.NewCard(wardedCreatureDef(t, "Ward:PayLife<2>"), p, engine.Battlefield)
			pinger := g.NewCard(creatureDefWithAbility(t, "Pinger", "AB$ DealDamage | Cost$ T | ValidTgts$ Creature | NumDmg$ 3"), opp, engine.Battlefield)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
			if !g.ActivateAbility(opp, pinger, 0, c) {
				t.Fatal("ActivateAbility failed")
			}
			if got := g.StackLen(); got != 2 {
				t.Fatalf("StackLen() = %d, want 2 (the ability plus Ward's trigger)", got)
			}
			c.QueueConfirmPayCost(tc.pay)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(target).Zone; got != tc.wantTarget {
				t.Errorf("warded creature zone = %v, want %v", got, tc.wantTarget)
			}
			if got := g.Player(opp).Life; got != tc.wantLife {
				t.Errorf("payer life = %d, want %d", got, tc.wantLife)
			}
			if g.Card(pinger).Zone != engine.Battlefield || !g.Card(pinger).Tapped {
				t.Errorf("pinger zone %v tapped %v, want Battlefield and tapped: countering an ability moves no card", g.Card(pinger).Zone, g.Card(pinger).Tapped)
			}
		})
	}
}

// TestWardOnAnAbilityNeedsAnOpponent proves the controller's own ability does
// not trigger its Ward.
func TestWardOnAnAbilityNeedsAnOpponent(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	target := g.NewCard(wardedCreatureDef(t, "Ward:PayLife<2>"), p, engine.Battlefield)
	pinger := g.NewCard(creatureDefWithAbility(t, "Pinger", "AB$ DealDamage | Cost$ T | ValidTgts$ Creature | NumDmg$ 1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	if !g.ActivateAbility(p, pinger, 0, c) {
		t.Fatal("ActivateAbility failed")
	}
	if got := g.StackLen(); got != 1 {
		t.Errorf("StackLen() = %d, want 1: no Ward against the controller's own ability", got)
	}
}

// TestWardOnATriggeredAbility is the triggered half of CR 702.21a: an
// opponent's triggered ability that targets the warded creature raises Ward
// too, and declining counters that trigger.
func TestWardOnATriggeredAbility(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGame(t)
	target := g.NewCard(wardedCreatureDef(t, "Ward:PayLife<2>"), p, engine.Battlefield)
	watcher := g.NewCard(triggerWatcherDef(t, "Zapper",
		"Mode$ SpellCast | ValidCard$ Card | ValidActivatingPlayer$ You | Execute$ TrigZap",
		"TrigZap", "DB$ DealDamage | ValidTgts$ Creature | NumDmg$ 3"), opp, engine.Battlefield)
	_ = watcher
	spell := g.NewCard(instantDefWithAbility(t, "Cantrip", "0", "SP$ GainLife | LifeAmount$ 1"), opp, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	if !g.CastSpell(opp, spell, c) {
		t.Fatal("CastSpell failed")
	}
	// The spell, Zapper's trigger, and Ward's trigger above it.
	if got := g.StackLen(); got != 3 {
		t.Fatalf("StackLen() = %d, want 3", got)
	}
	c.QueueConfirmPayCost(false)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(target).Zone; got != engine.Battlefield {
		t.Errorf("warded creature zone = %v, want Battlefield: the trigger was countered", got)
	}
}

// TestWardAfterChangeTargets proves a spell retargeted onto a warded permanent
// raises Ward for the spell's own controller (ChangeTargets, ADR-0028).
func TestWardAfterChangeTargets(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	target := g.NewCard(wardedCreatureDef(t, "Ward:PayLife<2>"), other, engine.Battlefield)
	seatRetargeter(t, g, p, "DB$ ChangeTargets | TargetType$ Spell.singleTarget | ValidTgts$ Card")
	bolt := g.NewCard(instantDefWithAbility(t, "Bolt", "W", "SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bolt)})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	c.QueueConfirmPayCost(false)
	if err := castW(t, g, p, bolt, c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(target).Zone; got != engine.Battlefield || g.Card(target).Damage.Marked != 0 {
		t.Errorf("warded creature zone %v damage %d, want undamaged on the battlefield: Ward countered the retargeted Bolt", got, g.Card(target).Damage.Marked)
	}
	if got := g.Card(bolt).Zone; got != engine.Graveyard {
		t.Errorf("bolt zone = %v, want Graveyard (countered)", got)
	}
}

// TestWardAfterCopySpellRetarget proves a copy whose new target is a warded
// permanent raises Ward: declining counters the copy, the original is
// untouched.
func TestWardAfterCopySpellRetarget(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	target := g.NewCard(wardedCreatureDef(t, "Ward:PayLife<2>"), other, engine.Battlefield)
	bolt := g.NewCard(instantDefWithAbility(t, "Bolt", "W", "SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueConfirmEffect(true)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	c.QueueConfirmPayCost(false)
	if err := castWithCopyWatcher(t, g, p, bolt, c, "Instant", "DB$ CopySpellAbility | Defined$ TriggeredSpellAbility | MayChooseTarget$ True"); err != nil {
		t.Fatal(err)
	}
	if g.Card(target).Zone != engine.Battlefield || g.Card(target).Damage.Marked != 0 {
		t.Errorf("warded creature zone %v damage %d, want undamaged: Ward countered the copy", g.Card(target).Zone, g.Card(target).Damage.Marked)
	}
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("opponent life = %d, want 17: only the original Bolt resolved", got)
	}
}

// TestWardCostShapes proves the Ward lines that used to be skipped before the
// stack: each now counters unless its cost is paid.
func TestWardCostShapes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		keyword string
		svars   []string
		setup   func(g *engine.Game, opp engine.PlayerID)
		queue   func(c *engine.ScriptedController, g *engine.Game, opp engine.PlayerID)
		paid    func(g *engine.Game, opp engine.PlayerID) bool
	}{
		{
			name:    "life equal to an SVar",
			keyword: "Ward:PayLife<X/life equal to CARDNAME's power>",
			svars:   []string{"X", "3"},
			paid:    func(g *engine.Game, opp engine.PlayerID) bool { return g.Player(opp).Life == 17 },
		},
		{
			name:    "poison counters",
			keyword: "Ward:AddCounterYou<2/POISON>",
			paid: func(g *engine.Game, opp engine.PlayerID) bool {
				return g.Player(opp).Counters.Count(engine.Poison) == 2
			},
		},
		{
			name:    "exile from the graveyard",
			keyword: "Ward:ExileFromGrave<1/Card>",
			setup: func(g *engine.Game, opp engine.PlayerID) {
				g.NewCard(creatureDef(t), opp, engine.Graveyard)
			},
			queue: func(c *engine.ScriptedController, g *engine.Game, opp engine.PlayerID) {
				c.QueueCardChoice([]engine.CardID{g.Zone(engine.Graveyard, opp).Cards()[0]})
			},
			paid: func(g *engine.Game, opp engine.PlayerID) bool { return len(g.Zone(engine.Exile, opp).Cards()) == 1 },
		},
		{
			name:    "collect evidence",
			keyword: "Ward:CollectEvidence<2>",
			setup: func(g *engine.Game, opp engine.PlayerID) {
				g.NewCard(creatureDefManaCost(t, "2"), opp, engine.Graveyard)
			},
			queue: func(c *engine.ScriptedController, g *engine.Game, opp engine.PlayerID) {
				c.QueueCardChoice([]engine.CardID{g.Zone(engine.Graveyard, opp).Cards()[0]})
			},
			paid: func(g *engine.Game, opp engine.PlayerID) bool { return len(g.Zone(engine.Exile, opp).Cards()) == 1 },
		},
		{
			name:    "blight",
			keyword: "Ward:Blight<1>",
			setup: func(g *engine.Game, opp engine.PlayerID) {
				g.NewCard(creatureDefPT(t, "3", "3"), opp, engine.Battlefield)
			},
			queue: func(c *engine.ScriptedController, g *engine.Game, opp engine.PlayerID) {
				c.QueueCardChoice([]engine.CardID{g.Zone(engine.Battlefield, opp).Cards()[0]})
			},
			paid: func(g *engine.Game, opp engine.PlayerID) bool {
				return g.Card(g.Zone(engine.Battlefield, opp).Cards()[0]).Counters.Count(engine.M1M1) == 1
			},
		},
	} {
		for _, pay := range []bool{true, false} {
			name := tc.name + " declined"
			if pay {
				name = tc.name + " paid"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				g, p, opp := newTwoPlayerGame(t)
				target := g.NewCard(wardedCreatureDef(t, tc.keyword, tc.svars...), p, engine.Battlefield)
				if tc.setup != nil {
					tc.setup(g, opp)
				}
				spell := g.NewCard(instantDefWithAbility(t, "Test Bolt", "0", "SP$ DealDamage | ValidTgts$ Creature | NumDmg$ 3"), opp, engine.Hand)
				c := engine.NewScriptedController()
				c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
				if !g.CastSpell(opp, spell, c) {
					t.Fatal("CastSpell failed")
				}
				if got := g.StackLen(); got != 2 {
					t.Fatalf("StackLen() = %d, want 2: Ward must reach the stack", got)
				}
				c.QueueConfirmPayCost(pay)
				if pay && tc.queue != nil {
					tc.queue(c, g, opp)
				}
				if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
					t.Fatalf("ResolveStack: %v", err)
				}
				wantTarget := engine.Battlefield
				if pay {
					wantTarget = engine.Graveyard
				}
				if got := g.Card(target).Zone; got != wantTarget {
					t.Errorf("warded creature zone = %v, want %v", got, wantTarget)
				}
				if got := tc.paid(g, opp); got != pay {
					t.Errorf("cost paid = %v, want %v", got, pay)
				}
			})
		}
	}
}

// TestWardSeveralCostsPickOne proves "Ward:PayLife<2>:Discard<1/Card>"
// (Ward.parse splits on ':'): the payer chooses one cost to pay, a
// GenericChoice, and the spell is countered when they pay none.
func TestWardSeveralCostsPickOne(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		option int
		pay    bool
		life   int
		hand   int
		zone   engine.ZoneType
	}{
		{"pays life", 0, true, 18, 1, engine.Graveyard},
		{"discards", 1, true, 20, 0, engine.Graveyard},
		{"declines", 0, false, 20, 1, engine.Battlefield},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, opp := newTwoPlayerGame(t)
			target := g.NewCard(wardedCreatureDef(t, "Ward:PayLife<2>:Discard<1/Card>"), p, engine.Battlefield)
			g.NewCard(creatureDef(t), opp, engine.Hand)
			spell := g.NewCard(instantDefWithAbility(t, "Test Bolt", "0", "SP$ DealDamage | ValidTgts$ Creature | NumDmg$ 3"), opp, engine.Hand)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
			if !g.CastSpell(opp, spell, c) {
				t.Fatal("CastSpell failed")
			}
			c.QueueOption(tc.option)
			c.QueueConfirmPayCost(tc.pay)
			if tc.pay && tc.option == 1 {
				c.QueueDiscardChoice(g.Zone(engine.Hand, opp).Cards())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(target).Zone; got != tc.zone {
				t.Errorf("warded creature zone = %v, want %v", got, tc.zone)
			}
			if g.Player(opp).Life != tc.life || len(g.Zone(engine.Hand, opp).Cards()) != tc.hand {
				t.Errorf("life %d hand %d, want %d and %d", g.Player(opp).Life, len(g.Zone(engine.Hand, opp).Cards()), tc.life, tc.hand)
			}
		})
	}
}
