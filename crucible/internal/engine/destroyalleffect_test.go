package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// TestDestroyAllEffectDestroysEveryMatch proves the one shape all 238 real
// lines share: every battlefield permanent matching ValidCards$ is
// destroyed, and nothing else is.
func TestDestroyAllEffectDestroysEveryMatch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll", "DB$ DestroyAll | ValidCards$ Creature.OppCtrl"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range []engine.CardID{a, b} {
		if z := g.Card(id).Zone; z != engine.Graveyard {
			t.Errorf("opponent creature %v zone = %v, want Graveyard", id, z)
		}
	}
	if z := g.Card(mine).Zone; z != engine.Battlefield {
		t.Errorf("own creature zone = %v, want Battlefield", z)
	}
}

// TestDestroyAllEffectSkipsIndestructibleAndRemembers proves canBeDestroyed
// filters first and RememberDestroyed$ records only what actually died.
func TestDestroyAllEffectSkipsIndestructibleAndRemembers(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	dies := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	lives := g.NewCard(indestructibleCreatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Remember", "DB$ DestroyAll | ValidCards$ Creature.OppCtrl | RememberDestroyed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(lives).Zone; z != engine.Battlefield {
		t.Errorf("indestructible zone = %v, want Battlefield", z)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(dies) {
		t.Errorf("Remembered = %v, want [%v]", got, engine.CardEntity(dies))
	}
}

// TestDestroyAllEffectSubstitutesXInValidCards proves every X in ValidCards$
// is replaced by the X SVar's value before the string is parsed
// (DestroyAllEffect.java:57-60), and that an X no SVar defines fails the
// ability instead of matching nothing (GO-7).
func TestDestroyAllEffectSubstitutesXInValidCards(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	small := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	big := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test DestroyAll X", "DB$ DestroyAll | ValidCards$ Creature.powerGEX", "X", "Number$2")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(big).Zone; z != engine.Graveyard {
		t.Errorf("power 3 zone = %v, want Graveyard", z)
	}
	if z := g.Card(small).Zone; z != engine.Battlefield {
		t.Errorf("power 1 zone = %v, want Battlefield", z)
	}

	def = etbChainDef(t, "Test DestroyAll Bare X", "DB$ DestroyAll | ValidCards$ Creature.cmcLEX")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for an X no SVar defines")
	}
}

// TestDestroyAllEffectValidTgtsNarrowsToTargetedPlayer proves ValidTgts$
// naming a player narrows the sweep to that player's permanents -- Java's
// getFirstTargetedPlayer filter.
func TestDestroyAllEffectValidTgtsNarrowsToTargetedPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Tgt", "DB$ DestroyAll | ValidTgts$ Player | ValidCards$ Creature"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(theirs).Zone; z != engine.Graveyard {
		t.Errorf("targeted player's creature zone = %v, want Graveyard", z)
	}
	if z := g.Card(mine).Zone; z != engine.Battlefield {
		t.Errorf("other player's creature zone = %v, want Battlefield", z)
	}
}

// TestDestroyAllEffectOptionalAsksTheActivator proves Optional$ (Awaken the
// Sleeper, DestroyAllEffect.java:77-82): declining destroys nothing.
func TestDestroyAllEffectOptionalAsksTheActivator(t *testing.T) {
	t.Parallel()

	for _, accept := range []bool{false, true} {
		g, p, other := newTwoPlayerGame(t)
		victim := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(accept)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Optional", "DB$ DestroyAll | ValidCards$ Creature.OppCtrl | Optional$ True | RememberAllObjects$ True"), c)
		if err != nil {
			t.Fatalf("accept=%v: %v", accept, err)
		}
		if died := g.Card(victim).Zone == engine.Graveyard; died != accept {
			t.Errorf("accept=%v: victim died = %v", accept, died)
		}
		if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(victim) {
			t.Errorf("accept=%v: remembered = %v, want the victim whatever the answer", accept, got)
		}
	}
}

// TestDestroyAllEffectIgnoresHiddenAndZone proves Hidden$ and Zone$ are
// dead params in DestroyAllEffect.java:43-101: the battlefield is always
// swept.
func TestDestroyAllEffectIgnoresHiddenAndZone(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Dead", "DB$ DestroyAll | ValidCards$ Creature.OppCtrl | Hidden$ True | Zone$ Battlefield"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(a).Zone; z != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard", z)
	}
}

// TestDestroyAllMostProminentColor proves `Card.MostProminentColor <color>`
// (CardProperty.java:694-707): with two black permanents against one red,
// black is the most prominent color and red is not.
func TestDestroyAllMostProminentColor(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(creatureDefManaCost(t, "B"), p, engine.Battlefield)
	g.NewCard(creatureDefManaCost(t, "B"), p, engine.Battlefield)
	red := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Prominent", "DB$ DestroyAll | ValidCards$ Card.MostProminentColor red"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(red).Zone; z != engine.Battlefield {
		t.Errorf("red creature zone = %v, want Battlefield (red is not the most prominent)", z)
	}
}

// TestSharesNameWithTargeted proves `sharesNameWith Targeted`
// (CardProperty.java:818, AbilityUtils.java:146-151) as Winnow reads it: the
// target is destroyed only when another permanent shares its name.
func TestSharesNameWithTargeted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		twin     bool
		wantZone engine.ZoneType
	}{{"twin in play", true, engine.Graveyard}, {"alone", false, engine.Battlefield}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			def := creatureDefPT(t, "2", "2")
			target := g.NewCard(def, p, engine.Battlefield)
			if tc.twin {
				g.NewCard(def, p, engine.Battlefield)
			}
			c := engine.NewScriptedController()
			_, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(target)},
				"DB$ Destroy | ValidTgts$ Permanent | ConditionCheckSVar$ Check | ConditionSVarCompare$ GE2",
				"Check", "Count$Valid Permanent.sharesNameWith Targeted")
			if err != nil {
				t.Fatal(err)
			}
			engine.CheckStateBasedActions(g, c)
			if z := g.Card(target).Zone; z != tc.wantZone {
				t.Errorf("target zone = %v, want %v", z, tc.wantZone)
			}
		})
	}
}

// `sharesNameWith Valid <spec>` (Pattern Matcher, AbilityUtils.java:287-297)
// matches a card that shares a name with a battlefield card the spec names.
func TestSharesNameWithValidList(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	def := creatureDefPT(t, "2", "2")
	twinned := g.NewCard(def, other, engine.Hand)
	host := g.NewCard(def, p, engine.Battlefield)
	loner := g.NewCard(namedCreature(t, "Loner", "1 G"), other, engine.Hand)
	spec := valid.Parse("Card.sharesNameWith Valid Creature.YouCtrl")
	for _, tc := range []struct {
		card engine.CardID
		want bool
	}{{twinned, true}, {loner, false}} {
		if got := engine.Matches(g, g.Card(tc.card), spec, p, host); got != tc.want {
			t.Errorf("card %v: matches = %v, want %v", tc.card, got, tc.want)
		}
	}
}

// RememberDrainedMana$ remembers how many units the drain emptied
// (DrainManaEffect.java:59-61), Pygmy Hippo's Count$RememberedNumber.
func TestDrainManaRemembersTheDrainedAmount(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.Player(other).ManaPool.Add(mana.Red, 3)
	before := g.Player(p).Life
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ DrainMana | Defined$ Player.Opponent | RememberDrainedMana$ True | SubAbility$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ X",
		"X", "Count$RememberedNumber")
	if got := g.Player(p).Life - before; got != 3 {
		t.Errorf("gained %d, want 3", got)
	}
}

// Tapper$ names the player who decides tap or untap (TapOrUntapEffect.java:
// 38-42); the opponent here taps the creature the activator targets.
func TestTapOrUntapTapperDecides(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTapOrUntap(true)
	c.QueueTapOrUntap(true) // the resolving host is a creature of the activator too
	resolveLine(t, g, p, c, "DB$ TapOrUntap | Defined$ Valid Creature.YouCtrl | Tapper$ Player.Opponent")
	_ = other
	if !g.Card(victim).Tapped {
		t.Error("creature untapped, want tapped by the Tapper's decision")
	}
}
