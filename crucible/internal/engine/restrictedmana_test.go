package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// restrictedRock puts a mana rock making one red mana under restrict on p's
// battlefield and activates it.
func restrictedRock(t *testing.T, g *engine.Game, p engine.PlayerID, restrict string) {
	t.Helper()
	def := creatureDefWithAbility(t, "Test Restricted Rock",
		"AB$ Mana | Cost$ T | Produced$ R | RestrictValid$ "+restrict)
	rock := g.NewCard(def, p, engine.Battlefield)
	g.Card(rock).SummonSick = false
	if !g.ActivateManaAbility(p, rock, 0, engine.NewScriptedController()) {
		t.Fatalf("ActivateManaAbility with RestrictValid$ %s returned false", restrict)
	}
}

// TestRestrictedManaPaysOnlyWhatItsRestrictionNames proves ADR-0046: red
// mana made under "Spell.Creature" cannot cast an instant, stays in the
// pool after the refused cast, and then casts a creature spell.
func TestRestrictedManaPaysOnlyWhatItsRestrictionNames(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	restrictedRock(t, g, p, "Spell.Creature")

	bolt := g.NewCard(instantDefWithAbility(t, "Bolt", "R", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 3"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(g.Players()[1])})
	if g.CastSpell(p, bolt, c) {
		t.Fatal("instant cast with creature-only mana, want it refused")
	}
	if got := g.Player(p).ManaPool.Breakdown()[3]; got != 1 {
		t.Fatalf("red after the refused cast = %d, want 1", got)
	}

	bears := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	if !g.CastSpell(p, bears, engine.NewScriptedController()) {
		t.Fatal("creature cast with creature-only mana, want it cast")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool after the creature cast holds %d, want 0", got)
	}
}

// TestRestrictedManaSpendsBeforePlainMana proves a cost payable by either is
// paid with the restricted unit first, so the plain unit stays free.
func TestRestrictedManaSpendsBeforePlainMana(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Red, 1)
	restrictedRock(t, g, p, "Spell.Creature")

	bears := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	if !g.CastSpell(p, bears, engine.NewScriptedController()) {
		t.Fatal("creature cast, want it cast")
	}
	// The plain unit is left: an instant now casts.
	bolt := g.NewCard(instantDefWithAbility(t, "Bolt", "R", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 3"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(g.Players()[1])})
	if !g.CastSpell(p, bolt, c) {
		t.Error("instant cast with the plain unit left, want it cast")
	}
}

// TestRestrictedManaPaysNoUnlessCost proves a payment made while the
// ability resolves meets no ordinary restriction (AbilityManaPart.java:
// 443-445): "Activated" mana cannot pay an unless-cost.
func TestRestrictedManaPaysNoUnlessCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	restrictedRock(t, g, p, "Activated")

	if g.PayManaCost(p, mana.MustParse("R"), engine.NewScriptedController()) {
		t.Error("PayManaCost with no payment context spent restricted mana, want it refused")
	}
	if got := g.Player(p).ManaPool.Breakdown()[3]; got != 1 {
		t.Errorf("red after the refused payment = %d, want 1", got)
	}
}

// TestManaEffectRestrictValidRefusesWhatNoPaymentCanJudge proves a resolving
// Mana line made under a restriction restrictionMet cannot judge errors
// instead of adding mana (GO-7), and one it can judge adds restricted mana.
func TestManaEffectRestrictValidRefusesWhatNoPaymentCanJudge(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	if _, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Exile", "Produced$ R | Defined$ You | RestrictValid$ Spell.wasCastFromExile"), engine.NewScriptedController()); err == nil {
		t.Error("RestrictValid$ Spell.wasCastFromExile resolved, want an error")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool after the refused line holds %d, want 0", got)
	}
	if _, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Spell", "Produced$ R | Defined$ You | RestrictValid$ Spell.Instant,Spell.Sorcery"), engine.NewScriptedController()); err != nil {
		t.Fatalf("RestrictValid$ Spell.Instant,Spell.Sorcery: %v", err)
	}
	bolt := g.NewCard(instantDefWithAbility(t, "Bolt", "R", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 3"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(g.Players()[1])})
	if !g.CastSpell(p, bolt, c) {
		t.Error("instant cast with instant-or-sorcery mana, want it cast")
	}
}

// TestRestrictedManaSpecialActionsAndKeywordSpells proves the three shapes
// Niko Defies Destiny, Smoky Lounge and Meeting of the Five name: foretelling
// is paid by Static.Foretelling mana, a three-color spell by numColorsEQ3.
func TestRestrictedManaSpecialActionsAndKeywordSpells(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	restrictedRock(t, g, p, "Static.Foretelling,Spell.withForetell")
	restrictedRock(t, g, p, "Spell.numColorsEQ3")
	restrictedRock(t, g, p, "Static.Unlock,Spell.Enchantment")

	// Colorless-cost permanents: no spell here is three-colored or has
	// foretell, so the three red units pay nothing ordinary.
	bears := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	if g.CastSpell(p, bears, engine.NewScriptedController()) {
		t.Error("red creature cast with foretell/three-color/unlock mana, want it refused")
	}
	if got := g.Player(p).ManaPool.Breakdown()[3]; got != 3 {
		t.Errorf("red after the refused cast = %d, want 3", got)
	}
}

// TestRestrictedManaKickedPaysOnlyAKickedSpell proves Spell.Kicked mana
// (Helga, Skittish Seer's kind): four units cannot help an unkicked Burst
// Lightning, and cover the kicker of a kicked one.
func TestRestrictedManaKickedPaysOnlyAKickedSpell(t *testing.T) {
	t.Parallel()

	for _, kick := range []bool{false, true} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		for range 4 {
			restrictedRock(t, g, p, "Spell.Kicked")
		}
		g.Player(p).ManaPool.Add(mana.Red, 1)
		bolt := g.NewCard(corpusCard(t, "Burst Lightning"), p, engine.Hand)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(kick)
		for range 4 {
			c.QueuePayGeneric(mana.ShardR)
		}
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		if !g.CastSpell(p, bolt, c) {
			t.Fatalf("kick=%v: CastSpell = false, want true (the plain red pays the unkicked cost)", kick)
		}
		left := g.Player(p).ManaPool.Total()
		if want := map[bool]int{false: 4, true: 0}[kick]; left != want {
			t.Errorf("kick=%v: %d mana left, want %d", kick, left, want)
		}
	}
}

// TestRestrictedManaCumulativeUpkeepPaysOnlyThatCost proves
// RestrictValid$ CumulativeUpkeep mana pays Balduvian Fallen's upkeep.
func TestRestrictedManaCumulativeUpkeepPaysOnlyThatCost(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	fallen := g.NewCard(corpusCard(t, "Balduvian Fallen"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.Untap)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	c.QueuePayGeneric(mana.ShardR)
	toStep(t, g, c, p, engine.Upkeep, func() { restrictedRock(t, g, p, "CumulativeUpkeep") })
	if got := g.Card(fallen).Zone; got != engine.Battlefield {
		t.Errorf("Balduvian Fallen is in %v after paying with upkeep-only mana, want Battlefield", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want the restricted unit spent", got)
	}
	// Count$TriggeredPayingMana.B.R: the one red unit spent pumps the Fallen.
	if power, _ := g.Card(fallen).Power(); power != 4 {
		t.Errorf("Balduvian Fallen power = %d, want 4 (3 and +1/+0 for the red mana spent)", power)
	}
}

// TestRestrictedManaAcceptsTurnFaceUpAndFaceDownShapes proves the shapes
// this port judges but never pays (no turn-face-up action, no face-down
// cast) make mana, which then pays an ordinary spell only through another
// alternative.
func TestRestrictedManaAcceptsTurnFaceUpAndFaceDownShapes(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	restrictedRock(t, g, p, "Spell.isCastFaceDown+Creature,Static.ManifestUp+Creature,Static.MorphUp")
	restrictedRock(t, g, p, "Static.isTurnFaceUp,Spell.Creature")
	bears := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	if !g.CastSpell(p, bears, engine.NewScriptedController()) {
		t.Fatal("creature cast, want it cast with the Spell.Creature unit")
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("%d mana left, want the face-down-only unit unspent", got)
	}
}
