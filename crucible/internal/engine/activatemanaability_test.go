package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestActivateManaAbilityAddsLiteralColor proves the corpus's own dominant
// real Produced$ shape -- a literal single color -- adds to the pool and
// taps the source (a mana dork's own "T: Add G." shape), firing both
// "becomes tapped" and "taps for mana" (checkTapsTriggers/
// checkTapsForManaTriggers, trigger.go).
func TestActivateManaAbilityAddsLiteralColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Dork", "AB$ Mana | Cost$ T | Produced$ G")
	dork := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, dork, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if !g.Card(dork).Tapped {
		t.Error("source not tapped after a Cost$ T mana ability")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 1, 0}); got != want {
		t.Errorf("pool breakdown = %v, want one green", got)
	}
}

// TestActivateManaAbilityAddsColorless proves Produced$ C -- the corpus's
// own single largest Produced$ value -- adds colorless mana rather than
// treating "C" as a (non-existent) sixth color.
func TestActivateManaAbilityAddsColorless(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Rock", "AB$ Mana | Cost$ T | Produced$ C")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 0, 1}); got != want {
		t.Errorf("pool breakdown = %v, want one colorless", got)
	}
}

// TestActivateManaAbilityAmount proves a plain-integer Amount$ multiplies
// the mana produced -- resolveNamedAmount's own plain-digit case
// (pumpAmount's own identical reading, reused).
func TestActivateManaAbilityAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Amount", "AB$ Mana | Cost$ T | Produced$ C | Amount$ 3")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 0, 3}); got != want {
		t.Errorf("pool breakdown = %v, want three colorless", got)
	}
}

// TestActivateManaAbilitySelfSacCost proves a self-sacrifice cost composes
// here exactly as it does for ActivateAbility (activateability.go):
// sacrificeCards reused wholesale, committed after the tap.
func TestActivateManaAbilitySelfSacCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Sac", "AB$ Mana | Cost$ T Sac<1/CARDNAME> | Produced$ C")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if zone := g.Card(rock).Zone; zone != engine.Graveyard {
		t.Errorf("source zone = %v, want Graveyard", zone)
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 0, 1}); got != want {
		t.Errorf("pool breakdown = %v, want one colorless", got)
	}
}

// TestActivateManaAbilitySelfExileCost proves a self-exile cost composes
// here exactly as it does for ActivateAbility (activateability.go):
// exileCards reused wholesale, committed after the tap -- the real
// mirrored_lotus.txt shape, "T, Exile CARDNAME: Add three mana of any one
// color."
func TestActivateManaAbilitySelfExileCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Exile", "AB$ Mana | Cost$ T Exile<1/CARDNAME> | Produced$ Any | Amount$ 3")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Green)
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if zone := g.Card(rock).Zone; zone != engine.Exile {
		t.Errorf("source zone = %v, want Exile", zone)
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 3, 0}); got != want {
		t.Errorf("pool breakdown = %v, want three green", got)
	}
}

// TestActivateManaAbilityTapTypeCost proves a tapXType<N/Type> cost composes
// here exactly as it does for ActivateAbility (activateability.go):
// ChoosePermanentsToTap/tapChosenPermanents (taptype.go) reused wholesale --
// the real birchlore_rangers.txt shape, "Tap two untapped Elves you
// control: Add one mana of any color," with no separate T of its own.
func TestActivateManaAbilityTapTypeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Tap Type", "AB$ Mana | Cost$ tapXType<2/Elf> | Produced$ Any")
	source := g.NewCard(def, p, engine.Battlefield)
	elf1 := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	elf2 := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTapChoice([]engine.CardID{elf1, elf2})
	c.QueueManaColor(mana.Black)
	if !g.ActivateManaAbility(p, source, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if !g.Card(elf1).Tapped || !g.Card(elf2).Tapped {
		t.Error("chosen tapXType candidates not tapped")
	}
	if g.Card(source).Tapped {
		t.Error("source tapped, want untapped -- this cost has no separate T token")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 1, 0, 0, 0}); got != want {
		t.Errorf("pool breakdown = %v, want one black", got)
	}
}

// TestActivateManaAbilityDeclinesWhenNotEnoughTapTypeCandidates proves the
// feasibility check runs before any mana is produced -- fewer untapped,
// type-matched permanents than TapTypeN declines outright. The source
// itself is an Elf too (creatureDefWithAbility's own doc comment) and this
// cost has no separate T token, so it counts toward the total: source plus
// one more Elf is only 2 candidates, short of the 3 this cost asks for.
func TestActivateManaAbilityDeclinesWhenNotEnoughTapTypeCandidates(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Tap Type Short", "AB$ Mana | Cost$ tapXType<3/Elf> | Produced$ Any")
	source := g.NewCard(def, p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, source, 0, c) {
		t.Error("ActivateManaAbility returned true with only 2 untapped Elves for tapXType<3/Elf>, want false")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana pool total = %d, want 0 -- a declined activation must not produce mana", got)
	}
}

// TestActivateManaAbilityDeclinesWhenSummonSickWithoutHaste proves CR
// 602.5b/302.6 applies to a mana dork the identical way it applies to any
// other Tap-cost activated ability.
func TestActivateManaAbilityDeclinesWhenSummonSickWithoutHaste(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Sick", "AB$ Mana | Cost$ T | Produced$ G")
	dork := g.NewCard(def, p, engine.Battlefield)
	g.Card(dork).SummonSick = true

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, dork, 0, c) {
		t.Error("ActivateManaAbility returned true for a summoning-sick source with no haste, want false")
	}
}

// TestActivateManaAbilityDeclinesForNonManaAPI proves index naming any API
// but "Mana" declines outright -- ActivateManaAbility's own scope is the
// exact complement of ActivateAbility's "API Mana" exclusion
// (activateability.go).
func TestActivateManaAbilityDeclinesForNonManaAPI(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Not Mana", "AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, creature, 0, c) {
		t.Error("ActivateManaAbility returned true for API Pump, want false")
	}
}

// TestActivateManaAbilityAsksForChosenColor proves Produced$ Any -- CR
// 605.3b's own "choose a color" shape, the corpus's own second-largest
// Produced$ value -- asks ChooseManaColor and adds exactly that color.
func TestActivateManaAbilityAsksForChosenColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Any Color", "AB$ Mana | Cost$ T | Produced$ Any")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Red)
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 1, 0, 0}); got != want {
		t.Errorf("pool breakdown = %v, want one red -- the queued ChooseManaColor answer", got)
	}
}

// TestActivateManaAbilityDeclinesForInvalidChosenColor proves a
// ChooseManaColor answer that is not exactly one color -- the empty set, or
// more than one bit -- declines rather than reaching Pool.Add's own panic
// (ChooseManaColor's own doc comment, control.go: not re-checked by the
// interface itself, so ActivateManaAbility is where a bad answer is caught).
func TestActivateManaAbilityDeclinesForInvalidChosenColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Bad Chosen Color", "AB$ Mana | Cost$ T | Produced$ Any")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Red | mana.Green)
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true for a two-color ChooseManaColor answer, want false")
	}
}

// TestActivateManaAbilityAsksForComboColor proves the corpus's own dominant
// dual/tri-land shape -- Produced$ Combo <letters> -- offers only the listed
// colors (parseComboColors), not every color the way "Any" does.
func TestActivateManaAbilityAsksForComboColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Combo Land", "AB$ Mana | Cost$ T | Produced$ Combo R G")
	land := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Green)
	if !g.ActivateManaAbility(p, land, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 1, 0}); got != want {
		t.Errorf("pool breakdown = %v, want one green -- the queued ChooseManaColor answer", got)
	}
}

// TestActivateManaAbilityDeclinesForOutOfComboColor proves a ChooseManaColor
// answer naming a color the Combo list did not offer -- a controller
// choosing Black for a "Combo R G" land -- declines rather than adding an
// unlisted color, the identical "trust ends here, not at Pool.Add's own
// panic" contract TestActivateManaAbilityDeclinesForInvalidChosenColor
// already proves for "Any".
func TestActivateManaAbilityDeclinesForOutOfComboColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Combo Wrong Color", "AB$ Mana | Cost$ T | Produced$ Combo R G")
	land := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Black)
	if g.ActivateManaAbility(p, land, 0, c) {
		t.Error("ActivateManaAbility returned true for a Black answer on a Combo R G land, want false")
	}
}

// TestActivateManaAbilityDeclinesForNonLiteralComboShape proves
// parseComboColors refuses anything past a literal WUBRG letter list --
// "Combo Any" here, CR 605.3b's own "add two mana in any combination of
// colors" (a per-unit independent choice this dispatch does not model) --
// rather than guessing at a subset (PORT-8/GO-7).
func TestActivateManaAbilityDeclinesForNonLiteralComboShape(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Combo Any", "AB$ Mana | Cost$ T | Produced$ Combo Any | Amount$ 2")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true for Produced$ Combo Any, want false")
	}
}

// TestActivateManaAbilityDeclinesForBlockedParam proves a param past
// manaAbilityAllowedParams -- RestrictValid$ here, a mana-pool spending
// restriction this port's own Pool cannot tag mana with -- declines the
// whole line rather than producing unrestricted mana (PORT-8/GO-7).
func TestActivateManaAbilityDeclinesForBlockedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Restricted Mana",
		"AB$ Mana | Cost$ T | Produced$ R | RestrictValid$ Spell.Creature")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true with RestrictValid$ present, want false")
	}
}

// TestActivateManaAbilityDeclinesForDiscardCost proves a Discard<N/Card>
// cost component -- one of cost.Cost.ActivationShape's own five primitives,
// but 0 real corpus A:AB$ Mana lines ever carry it -- declines outright
// rather than claiming the cost was paid while never actually discarding
// anything (ActivateManaAbility's own doc comment has the reason, PORT-8/
// GO-7).
func TestActivateManaAbilityDeclinesForDiscardCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	toss := g.NewCard(nil, p, engine.Hand)

	def := creatureDefWithAbility(t, "Test Mana Discard", "AB$ Mana | Cost$ Discard<1/Card> | Produced$ C")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true for a Discard<1/Card> cost, want false")
	}
	if zone := g.Card(toss).Zone; zone != engine.Hand {
		t.Errorf("hand card zone = %v, want Hand -- a declined activation must not discard anything", zone)
	}
}

// TestActivateManaAbilityDeclinesForPayLifeCost proves a PayLife<N> cost
// component -- one of cost.Cost.ActivationShape's own five primitives, but
// 0 real corpus A:AB$ Mana lines ever carry it -- declines outright rather
// than claiming the cost was paid while never actually losing any life
// (ActivateManaAbility's own doc comment has the reason, PORT-8/GO-7).
func TestActivateManaAbilityDeclinesForPayLifeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Pay Life", "AB$ Mana | Cost$ PayLife<2> | Produced$ C")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true for a PayLife<2> cost, want false")
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("life = %d, want 20 -- a declined activation must not touch life", got)
	}
}

// TestActivateManaAbilityPaysEnergyCost proves PayEnergy<N> is not declined
// the way Discard/PayLife are -- aether_hub.txt's own real
// "T, Pay one energy counter: Add one mana of any color" shape (4 real
// corpus A:AB$ Mana lines carry PayEnergy<...>) pays it and still produces
// mana.
func TestActivateManaAbilityPaysEnergyCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).Counters.Add(engine.Energy, 2)

	def := creatureDefWithAbility(t, "Test Mana Pay Energy", "AB$ Mana | Cost$ T PayEnergy<1> | Produced$ Any")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Blue)
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if !g.Card(rock).Tapped {
		t.Error("source not tapped after a Cost$ T PayEnergy<1> mana ability")
	}
	if got := g.Player(p).Counters.Count(engine.Energy); got != 1 {
		t.Errorf("Energy count = %d, want 1 (2 - 1 paid)", got)
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 1, 0, 0, 0, 0}); got != want {
		t.Errorf("pool breakdown = %v, want one blue -- the queued ChooseManaColor answer", got)
	}
}

// TestActivateManaAbilityDeclinesWhenEnergyTooLowForPayEnergyCost proves a
// PayEnergy<N> cost declines outright with no side effect when the player's
// own Energy counter count is below N.
func TestActivateManaAbilityDeclinesWhenEnergyTooLowForPayEnergyCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).Counters.Add(engine.Energy, 0)

	def := creatureDefWithAbility(t, "Test Mana Pay Energy Too Low", "AB$ Mana | Cost$ T PayEnergy<1> | Produced$ Any")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Blue)
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true with 0 energy for a PayEnergy<1> cost, want false")
	}
	if g.Card(rock).Tapped {
		t.Error("source tapped for a declined PayEnergy<1> mana ability, want untapped")
	}
}
