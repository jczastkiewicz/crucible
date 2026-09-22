package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// creatureDefWithAbility builds a *compile.Card for a 2/2 Elf creature
// carrying one real A:AB$ line (abilityText, no leading "AB$ " needed --
// the caller writes it exactly as a card script would, e.g.
// "AB$ Pump | Cost$ T | ...") -- ActivateAbility itself has no other
// exported way to drive it, so every case here goes through the real
// compiled param parser rather than a hand-built compile.Ability (TEST-1).
func creatureDefWithAbility(t *testing.T, name, abilityText string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].Abilities = []string{abilityText}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// creatureDefWithAbilityAndTrigger is creatureDefWithAbility's own sibling,
// additionally carrying one real T: line (triggerLine, no leading "T$ "
// needed) -- the SelfExile-cost-fires-a-leaves-the-battlefield-trigger tests
// need a card that both pays an Exile<1/CARDNAME> cost and watches for the
// identical event, which creatureDefWithAbility alone has no way to build.
func creatureDefWithAbilityAndTrigger(t *testing.T, name, abilityText, triggerLine string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].Abilities = []string{abilityText}
	raw.Faces[0].Triggers = []string{triggerLine}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestActivateAbilityTapCostRunsEffectAndTaps proves the corpus's own
// dominant real Cost$ shape -- a bare Tap token, 2,515 of the corpus's own
// 10,879 real A:AB$ lines -- pays through the SummonSick/Haste check
// (DeclareCombatAttackers' own identical gate, attack.go, reused) rather
// than a mana payment, taps the source, fires Mode$ Taps
// (checkTapsTriggers), and pushes the named API (Pump here) onto the stack
// for ResolveStack to run.
func TestActivateAbilityTapCostRunsEffectAndTaps(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Pump", "AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(creature).Tapped {
		t.Error("creature not tapped after a Cost$ T activation")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	pw, _ := g.Card(creature).Power()
	tg, _ := g.Card(creature).Toughness()
	if pw != 3 || tg != 3 {
		t.Errorf("power/toughness = %d/%d, want 3/3 (2/2 base + 1/1 pump)", pw, tg)
	}
}

// TestActivateAbilityDeclinesWhenAlreadyTapped proves a Cost$ T ability
// cannot be activated a second time before the source untaps -- CR 602.5b's
// own cost feasibility check, with no side effect on a decline (the pump
// must never apply).
func TestActivateAbilityDeclinesWhenAlreadyTapped(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Already Tapped", "AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)
	g.Card(creature).Tapped = true

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for an already-tapped source, want false")
	}
}

// TestActivateAbilityDeclinesWhenSummonSickWithoutHaste proves CR 602.5b/
// 302.6: a Tap-cost ability on a permanent that has not been under its
// controller's control since their most recent turn began cannot be
// activated unless it has haste.
func TestActivateAbilityDeclinesWhenSummonSickWithoutHaste(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Summon Sick", "AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)
	g.Card(creature).SummonSick = true

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for a summoning-sick source with no haste, want false")
	}
}

// TestActivateAbilityPaysMana proves the corpus's second dominant real
// Cost$ shape -- pure mana, no tap at all -- pays through PayManaCost
// exactly as CastSpell's own does, and does not touch Card.Tapped.
func TestActivateAbilityPaysMana(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Black, 1)

	def := creatureDefWithAbility(t, "Test Mana Cost", "AB$ Pump | Cost$ B | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if g.Card(creature).Tapped {
		t.Error("creature tapped by a mana-only Cost$, want untapped")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana pool total = %d, want 0 -- the {B} cost must actually be charged", got)
	}
}

// TestActivateAbilityDeclinesWhenManaUnaffordable proves a failed mana
// payment declines the whole activation -- PayManaCost's own "unchanged
// pool on failure" guarantee, threaded through.
func TestActivateAbilityDeclinesWhenManaUnaffordable(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Unaffordable", "AB$ Pump | Cost$ B | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true with no mana in the pool, want false")
	}
}

// TestActivateAbilityDeclinesForManaAPI proves CR 605.3a's own no-stack
// mana ability stays out of scope here: naming API Mana is rejected
// outright, never pushed onto the stack the ordinary way.
func TestActivateAbilityDeclinesForManaAPI(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana API", "AB$ Mana | Cost$ T | Produced$ G")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for API Mana, want false")
	}
}

// TestActivateAbilityDeclinesForNonPureManaCost proves a Cost$ naming
// anything past mana and Tap -- a Sac<.../Discard<.../... part, here --
// fails loudly by returning false rather than silently paying the mana
// half and ignoring the rest (PORT-8/GO-7).
func TestActivateAbilityDeclinesForNonPureManaCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Sac Cost", "AB$ Pump | Cost$ Sac<1/Creature> | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for a Sac<...> cost, want false")
	}
}

// TestActivateAbilityDeclinesForSpellRecordLine proves index selecting a
// card's own A:SP$ line (an Instant's/Adventure's own spell half, sharing
// compile.Face's own Abilities slice with A:AB$ lines) is rejected: only a
// literal Activated record can be activated this way.
func TestActivateAbilityDeclinesForSpellRecordLine(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Spell Record", "SP$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for an A:SP$ line, want false")
	}
}

// TestActivateAbilitySelfSacCostSacrificesSourceAndRunsEffect proves the
// corpus's own second-most-common real activation cost shape past a bare
// Tap -- a self-sacrifice token, Sac<1/CARDNAME>, 947 of the corpus's own
// real non-Mana-API A:AB$ lines -- actually sacrifices the
// source through sacrificeCards (sacrificeeffect.go, reused wholesale) once
// the rest of the cost is paid, and the ability still resolves off the
// stack afterward even though its own source has already left the
// battlefield (CR 112.7a).
func TestActivateAbilitySelfSacCostSacrificesSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Self Sac", "AB$ GainLife | Cost$ Sac<1/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Graveyard {
		t.Errorf("source zone = %v, want Graveyard -- the Sac<1/CARDNAME> cost must actually sacrifice it", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 -- the ability must still resolve with its source already gone", got)
	}
}

// TestActivateAbilityCombinesManaTapAndSelfSac proves all three payment
// primitives compose on one line -- mana charged, the source tapped, then
// sacrificed, in that order (activateability.go's own doc comment: mana
// first, tap second, sacrifice last).
func TestActivateAbilityCombinesManaTapAndSelfSac(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.White, 1)

	def := creatureDefWithAbility(t, "Test Mana Tap Sac", "AB$ GainLife | Cost$ W T Sac<1/CARDNAME> | Defined$ You | LifeAmount$ 2")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana pool total = %d, want 0 -- the {W} cost must actually be charged", got)
	}
	if zone := g.Card(creature).Zone; zone != engine.Graveyard {
		t.Errorf("source zone = %v, want Graveyard", zone)
	}
}

// TestActivateAbilityDeclinesForChosenSacCost proves a Sac<...> naming
// anything but the literal self-reference CARDNAME -- a chosen count, or a
// chosen valid spec -- still declines outright rather than silently
// sacrificing the wrong thing: ActivationShape's own doc comment has the
// reason (internal/cost).
func TestActivateAbilityDeclinesForChosenSacCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Chosen Sac", "AB$ GainLife | Cost$ Sac<2/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for Sac<2/CARDNAME>, want false")
	}
}

// TestActivateAbilityDiscardCostDiscardsChosenCardsAndRunsEffect proves
// Discard<N/Card> -- the corpus's own dominant real Discard-as-cost shape,
// 244 of the corpus's own real non-Mana-API A:AB$ lines -- asks
// ChooseCardsToDiscard for exactly N cards and discards them (discardCards,
// discardeffect.go, reused wholesale) once the rest of the cost is paid, and
// the ability still resolves.
func TestActivateAbilityDiscardCostDiscardsChosenCardsAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	keep := g.NewCard(nil, p, engine.Hand)
	toss := g.NewCard(nil, p, engine.Hand)

	def := creatureDefWithAbility(t, "Test Discard Cost", "AB$ GainLife | Cost$ Discard<1/Card> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueDiscardChoice([]engine.CardID{toss})
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(toss).Zone; zone != engine.Graveyard {
		t.Errorf("discarded card zone = %v, want Graveyard", zone)
	}
	if zone := g.Card(keep).Zone; zone != engine.Hand {
		t.Errorf("kept card zone = %v, want Hand -- only the chosen card should be discarded", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
}

// TestActivateAbilityDeclinesWhenHandTooSmallForDiscardCost proves a
// Discard<N/Card> cost the activating player's own hand cannot pay -- fewer
// than N cards -- declines outright with no side effect, rather than
// discarding fewer cards than the cost demands.
func TestActivateAbilityDeclinesWhenHandTooSmallForDiscardCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	only := g.NewCard(nil, p, engine.Hand)

	def := creatureDefWithAbility(t, "Test Discard Too Few", "AB$ GainLife | Cost$ Discard<2/Card> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true with only 1 card in hand for a Discard<2/Card> cost, want false")
	}
	if zone := g.Card(only).Zone; zone != engine.Hand {
		t.Errorf("hand card zone = %v, want Hand -- a declined activation must not discard anything", zone)
	}
}

// TestActivateAbilityCombinesTapAndDiscardCost proves Tap and Discard
// compose on one line -- the source taps, then the chosen card is
// discarded, then the ability still resolves.
func TestActivateAbilityCombinesTapAndDiscardCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	toss := g.NewCard(nil, p, engine.Hand)

	def := creatureDefWithAbility(t, "Test Tap Discard", "AB$ GainLife | Cost$ T Discard<1/Card> | Defined$ You | LifeAmount$ 2")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueDiscardChoice([]engine.CardID{toss})
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(creature).Tapped {
		t.Error("creature not tapped after a Cost$ T Discard<1/Card> activation")
	}
	if zone := g.Card(toss).Zone; zone != engine.Graveyard {
		t.Errorf("discarded card zone = %v, want Graveyard", zone)
	}
}

// TestActivateAbilityDeclinesForNonLiteralDiscardCost proves a Discard<...>
// naming anything but the literal "N/Card" shape -- a self-discard here,
// Discard<1/CARDNAME> -- still declines outright: ActivationShape's own doc
// comment has the reason (internal/cost).
func TestActivateAbilityDeclinesForNonLiteralDiscardCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Discard Self", "AB$ GainLife | Cost$ Discard<1/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for Discard<1/CARDNAME>, want false")
	}
}

// TestActivateAbilityPayLifeCostRunsEffectAndEmitsLifeChanged proves
// PayLife<N> -- the corpus's own dominant real "pay life" activation-cost
// shape -- subtracts N from the activating player's own life, emits the
// identical LifeChanged event loseLifeEffect's own does, and still lets the
// ability resolve.
func TestActivateAbilityPayLifeCostRunsEffectAndEmitsLifeChanged(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	var sink recordingSink
	g.SetSink(&sink)

	def := creatureDefWithAbility(t, "Test Pay Life", "AB$ Draw | Cost$ PayLife<2> | Defined$ You | NumCards$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.Player(p).Life; got != 18 {
		t.Errorf("life = %d, want 18", got)
	}
	var saw bool
	for _, e := range sink.events {
		if e.Kind == engine.LifeChanged {
			saw = true
			if e.Amount != -2 {
				t.Errorf("LifeChanged Amount = %d, want -2", e.Amount)
			}
		}
	}
	if !saw {
		t.Error("no LifeChanged event seen")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand size = %d, want 1 -- Draw must still resolve", got)
	}
}

// TestActivateAbilityDeclinesWhenLifeTooLowForPayLifeCost proves CR 119.4:
// a life payment can never bring the payer below 0, so a PayLife<N> cost
// declines outright with no side effect when the player's own life is
// below N.
func TestActivateAbilityDeclinesWhenLifeTooLowForPayLifeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 1, 20

	def := creatureDefWithAbility(t, "Test Pay Life Too Low", "AB$ GainLife | Cost$ PayLife<2> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true with life 1 for a PayLife<2> cost, want false")
	}
	if got := g.Player(p).Life; got != 1 {
		t.Errorf("life = %d, want 1 -- a declined activation must not touch life", got)
	}
}

// TestActivateAbilityCombinesTapAndPayLifeCost proves Tap and PayLife
// compose on one line -- the source taps, then life is paid, then the
// ability still resolves.
func TestActivateAbilityCombinesTapAndPayLifeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Pay Life", "AB$ GainLife | Cost$ T PayLife<1> | Defined$ You | LifeAmount$ 4")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(creature).Tapped {
		t.Error("creature not tapped after a Cost$ T PayLife<1> activation")
	}
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("life = %d, want 19 (20 - 1 paid)", got)
	}
}

// TestActivateAbilityDeclinesForNonLiteralPayLifeCost proves a PayLife<...>
// naming anything but a literal positive integer -- an X-cost here,
// PayLife<X> -- still declines outright: ActivationShape's own doc comment
// has the reason (internal/cost).
func TestActivateAbilityDeclinesForNonLiteralPayLifeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Pay Life X", "AB$ GainLife | Cost$ X PayLife<X> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for PayLife<X>, want false")
	}
}

// TestActivateAbilityPayEnergyCostRunsEffectAndEmitsCounterChanged proves
// PayEnergy<N> -- CR 122.5's "pay N energy counters" activation-cost shape --
// subtracts N from the activating player's own Energy counter, emits the
// identical CounterChanged event putCounterEffect's own does, and still lets
// the ability resolve.
func TestActivateAbilityPayEnergyCostRunsEffectAndEmitsCounterChanged(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Counters.Add(engine.Energy, 3)
	g.NewCard(nil, p, engine.Library)
	var sink recordingSink
	g.SetSink(&sink)

	def := creatureDefWithAbility(t, "Test Pay Energy", "AB$ Draw | Cost$ PayEnergy<2> | Defined$ You | NumCards$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.Player(p).Counters.Count(engine.Energy); got != 1 {
		t.Errorf("Energy count = %d, want 1", got)
	}
	var saw bool
	for _, e := range sink.events {
		if e.Kind == engine.CounterChanged && e.Detail == uint32(engine.CounterDetailEnergy) {
			saw = true
			if e.Amount != -2 {
				t.Errorf("CounterChanged Amount = %d, want -2", e.Amount)
			}
		}
	}
	if !saw {
		t.Error("no Energy CounterChanged event seen")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand size = %d, want 1 -- Draw must still resolve", got)
	}
}

// TestActivateAbilityDeclinesWhenEnergyTooLowForPayEnergyCost proves a
// PayEnergy<N> cost declines outright with no side effect when the player's
// own Energy counter count is below N.
func TestActivateAbilityDeclinesWhenEnergyTooLowForPayEnergyCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Counters.Add(engine.Energy, 1)

	def := creatureDefWithAbility(t, "Test Pay Energy Too Low", "AB$ GainLife | Cost$ PayEnergy<2> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true with 1 energy for a PayEnergy<2> cost, want false")
	}
	if got := g.Player(p).Counters.Count(engine.Energy); got != 1 {
		t.Errorf("Energy count = %d, want 1 -- a declined activation must not touch it", got)
	}
}

// TestActivateAbilityCombinesTapAndPayEnergyCost proves Tap and PayEnergy
// compose on one line -- the source taps, then energy is paid, then the
// ability still resolves.
func TestActivateAbilityCombinesTapAndPayEnergyCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Counters.Add(engine.Energy, 1)

	def := creatureDefWithAbility(t, "Test Tap Pay Energy", "AB$ GainLife | Cost$ T PayEnergy<1> | Defined$ You | LifeAmount$ 4")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(creature).Tapped {
		t.Error("creature not tapped after a Cost$ T PayEnergy<1> activation")
	}
	if got := g.Player(p).Counters.Count(engine.Energy); got != 0 {
		t.Errorf("Energy count = %d, want 0 (1 - 1 paid)", got)
	}
}

// TestActivateAbilityDeclinesForNonLiteralPayEnergyCost proves a
// PayEnergy<...> naming anything but a literal positive integer -- an
// X-cost here, PayEnergy<X> -- still declines outright: ActivationShape's
// own doc comment has the reason (internal/cost).
func TestActivateAbilityDeclinesForNonLiteralPayEnergyCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Counters.Add(engine.Energy, 5)

	def := creatureDefWithAbility(t, "Test Pay Energy X", "AB$ GainLife | Cost$ X PayEnergy<X> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for PayEnergy<X>, want false")
	}
}

// TestActivateAbilitySelfExileCostExilesSourceAndRunsEffect proves
// Exile<1/CARDNAME> -- Sac<1/CARDNAME>'s own sibling shape -- actually moves
// the source to Exile (exileCards, exile.go) and still lets the ability
// resolve with its source already gone.
func TestActivateAbilitySelfExileCostExilesSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Self Exile", "AB$ GainLife | Cost$ Exile<1/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Exile {
		t.Errorf("source zone = %v, want Exile -- the Exile<1/CARDNAME> cost must actually exile it", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 -- the ability must still resolve with its source already gone", got)
	}
}

// TestActivateAbilityCombinesTapAndSelfExile proves Tap and SelfExile
// compose on one line -- the source taps, then it is exiled.
func TestActivateAbilityCombinesTapAndSelfExile(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Exile", "AB$ GainLife | Cost$ T Exile<1/CARDNAME> | Defined$ You | LifeAmount$ 2")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Exile {
		t.Errorf("source zone = %v, want Exile", zone)
	}
}

// TestActivateAbilityDeclinesForChosenExileCost proves an Exile<...> naming
// anything but the literal self-reference CARDNAME -- a chosen count here --
// still declines outright rather than silently exiling the wrong thing:
// ActivationShape's own doc comment has the reason (internal/cost).
func TestActivateAbilityDeclinesForChosenExileCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Chosen Exile", "AB$ GainLife | Cost$ Exile<2/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for Exile<2/CARDNAME>, want false")
	}
}

// TestActivateAbilityExileCostFiresOwnLeavesBattlefieldTrigger proves
// checkExiledTriggers (exile.go) actually fires CR 603.6d's own "leaves the
// battlefield" trigger family for an Exile<1/CARDNAME> cost -- isDiesTrigger's
// own sibling for the Exile destination -- on the exiled card's own trigger,
// not only on a watcher (below).
func TestActivateAbilityExileCostFiresOwnLeavesBattlefieldTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.NewCard(nil, p, engine.Library)

	def := creatureDefWithAbilityAndTrigger(t, "Test Exile Own Trigger",
		"AB$ GainLife | Cost$ Exile<1/CARDNAME> | Defined$ You | LifeAmount$ 1",
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Exile | ValidCard$ Card.Self | Execute$ TrigDraw")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 (the activated ability plus the leaves-the-battlefield trigger's own Draw)", got)
	}
}

// TestActivateAbilityExileCostFiresOtherWatcherLeavesBattlefieldTrigger
// proves otherExiledTriggerMatches (exile.go) -- a permanent still on the
// battlefield watching another card leave for Exile -- fires too, the
// identical "own" vs. "other" split checkDiesTriggers/otherDiesTriggerMatches
// already have for the graveyard-destination case.
func TestActivateAbilityExileCostFiresOtherWatcherLeavesBattlefieldTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.NewCard(nil, p, engine.Library)

	watcher := diesTriggerCreatureDefWithLine(t, "2", "2",
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Exile | ValidCard$ Card.Elf | Execute$ TrigDraw")
	g.NewCard(watcher, p, engine.Battlefield)

	def := creatureDefWithAbility(t, "Test Exile Other Watcher", "AB$ GainLife | Cost$ Exile<1/CARDNAME> | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 (the activated ability plus the watcher's own leaves-the-battlefield Draw)", got)
	}
}

// TestActivateAbilityTapTypeCostTapsChosenPermanentsAndRunsEffect proves
// tapXType<N/Type> -- CR 602's own "tap N untapped permanents of a type" --
// asks ChoosePermanentsToTap for exactly N candidates and taps them
// (tapChosenPermanents, taptype.go), leaving the source itself untapped
// since this cost carries no separate T token.
func TestActivateAbilityTapTypeCostTapsChosenPermanentsAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Type", "AB$ GainLife | Cost$ tapXType<2/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)
	other1 := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	other2 := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTapChoice([]engine.CardID{other1, other2})
	if !g.ActivateAbility(p, source, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(other1).Tapped || !g.Card(other2).Tapped {
		t.Error("chosen tapXType candidates not tapped")
	}
	if g.Card(source).Tapped {
		t.Error("source tapped, want untapped -- this cost has no separate T token")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
}

// TestActivateAbilityDeclinesWhenNotEnoughTapTypeCandidates proves the
// feasibility check (tapTypeCandidates, taptype.go) runs before anything is
// committed: fewer untapped, type-matched permanents than TapTypeN declines
// outright rather than asking the controller for more than exist.
func TestActivateAbilityDeclinesWhenNotEnoughTapTypeCandidates(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Type Short", "AB$ GainLife | Cost$ tapXType<3/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, source, 0, c) {
		t.Error("ActivateAbility returned true with only 2 untapped Creatures for tapXType<3/Creature>, want false")
	}
}

// TestActivateAbilityTapTypeExcludesSourceWhenCostAlsoTapsIt proves
// CostTapType.java's own canTapSource = !costHasTapSource rule
// (tapTypeCandidates' own doc comment, taptype.go): when the cost also taps
// its own source through a plain T token, the source itself can never also
// count toward tapXType's own total, even though it matches the type spec --
// with the source as the only Creature on the battlefield, this cost
// declines rather than double-counting it.
func TestActivateAbilityTapTypeExcludesSourceWhenCostAlsoTapsIt(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Self Excludes", "AB$ GainLife | Cost$ T tapXType<1/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, source, 0, c) {
		t.Error("ActivateAbility returned true when the source is the only Creature and T already taps it, want false")
	}
}

// TestActivateAbilityDeclinesForNonLiteralTapTypeCost proves a
// tapXType<...> naming anything but a literal positive integer -- an X-cost
// here -- still declines outright: ActivationShape's own doc comment has
// the reason (internal/cost).
func TestActivateAbilityDeclinesForNonLiteralTapTypeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Type X", "AB$ GainLife | Cost$ X tapXType<X/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, source, 0, c) {
		t.Error("ActivateAbility returned true for tapXType<X/Creature>, want false")
	}
}

// TestActivateAbilityDeclinesForUnresolvableTapTypeSpec proves
// tapTypeResolvable (taptype.go) refuses CostTapType.java's own
// withTotalPowerGE shape rather than reaching Matches with a spec it cannot
// evaluate correctly.
func TestActivateAbilityDeclinesForUnresolvableTapTypeSpec(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Type Power", "AB$ GainLife | Cost$ tapXType<1/Creature+withTotalPowerGE3> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, source, 0, c) {
		t.Error("ActivateAbility returned true for a withTotalPowerGE-shaped tapXType spec, want false")
	}
}

// TestActivateAbilityTapTypeMatchesSemicolonSeparatedTypeList proves a
// Cost-syntax ";"-separated type list becomes valid.Parse's own
// ","-separated OR (tapTypeCandidates' own doc comment, taptype.go): an
// Artifact permanent matches tapXType<1/Artifact;Creature> even though it is
// not itself a Creature.
func TestActivateAbilityTapTypeMatchesSemicolonSeparatedTypeList(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Type OR", "AB$ GainLife | Cost$ tapXType<1/Artifact;Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)
	art := g.NewCard(artifactDefManaCost(t, "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTapChoice([]engine.CardID{art})
	if !g.ActivateAbility(p, source, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(art).Tapped {
		t.Error("the Artifact not tapped by tapXType<1/Artifact;Creature>")
	}
}

// TestActivateAbilitySelfReturnCostReturnsSourceAndRunsEffect proves
// Return<1/CARDNAME> -- SelfSac's/SelfExile's own third sibling shape --
// actually moves the source to Hand (returnCards, returncost.go) and still
// lets the ability resolve with its source already gone.
func TestActivateAbilitySelfReturnCostReturnsSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Self Return", "AB$ GainLife | Cost$ Return<1/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Hand {
		t.Errorf("source zone = %v, want Hand -- the Return<1/CARDNAME> cost must actually return it", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 -- the ability must still resolve with its source already gone", got)
	}
}

// TestActivateAbilityCombinesTapAndSelfReturn proves Tap and SelfReturn
// compose on one line -- the source taps, then it is returned to hand.
func TestActivateAbilityCombinesTapAndSelfReturn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Return", "AB$ GainLife | Cost$ T Return<1/CARDNAME> | Defined$ You | LifeAmount$ 2")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Hand {
		t.Errorf("source zone = %v, want Hand", zone)
	}
}

// TestActivateAbilityDeclinesForChosenReturnCost proves a Return<...> naming
// anything but the literal self-reference CARDNAME/NICKNAME -- a chosen
// count here -- still declines outright rather than silently returning the
// wrong thing: ActivationShape's own doc comment has the reason
// (internal/cost).
func TestActivateAbilityDeclinesForChosenReturnCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Chosen Return", "AB$ GainLife | Cost$ Return<2/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for Return<2/CARDNAME>, want false")
	}
}

// TestActivateAbilityReturnCostFiresOwnLeavesBattlefieldTrigger proves
// checkReturnedTriggers (returncost.go) actually fires CR 603.6d's own
// "leaves the battlefield" trigger family for a Return<1/CARDNAME> cost --
// isDiesTrigger's/isExiledTrigger's own third sibling, Destination$ Hand --
// on the returned card's own trigger, not only on a watcher (below).
func TestActivateAbilityReturnCostFiresOwnLeavesBattlefieldTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbilityAndTrigger(t, "Test Return Own Trigger",
		"AB$ GainLife | Cost$ Return<1/CARDNAME> | Defined$ You | LifeAmount$ 1",
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Hand | ValidCard$ Card.Self | Execute$ TrigDraw")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 (the activated ability plus the leaves-the-battlefield trigger's own Draw)", got)
	}
}

// TestActivateAbilityReturnCostFiresOtherWatcherLeavesBattlefieldTrigger
// proves otherReturnedTriggerMatches (returncost.go) -- a permanent still on
// the battlefield watching another card leave for Hand -- fires too, the
// identical "own" vs. "other" split checkDiesTriggers/checkExiledTriggers
// already have.
func TestActivateAbilityReturnCostFiresOtherWatcherLeavesBattlefieldTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	watcher := diesTriggerCreatureDefWithLine(t, "2", "2",
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Hand | ValidCard$ Card.Elf | Execute$ TrigDraw")
	g.NewCard(watcher, p, engine.Battlefield)

	def := creatureDefWithAbility(t, "Test Return Other Watcher", "AB$ GainLife | Cost$ Return<1/CARDNAME> | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 (the activated ability plus the watcher's own leaves-the-battlefield Draw)", got)
	}
}

// TestActivateAbilityReturnTypeCostReturnsChosenPermanentsAndRunsEffect
// proves Return<N/Type> -- tapXType's own sibling for "return to hand"
// rather than "tap" -- asks ChoosePermanentsToReturn for exactly N
// candidates and returns them (returnCards, returncost.go).
func TestActivateAbilityReturnTypeCostReturnsChosenPermanentsAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Return Type", "AB$ GainLife | Cost$ Return<2/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)
	other1 := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	other2 := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueReturnChoice([]engine.CardID{other1, other2})
	if !g.ActivateAbility(p, source, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(other1).Zone; zone != engine.Hand {
		t.Errorf("other1 zone = %v, want Hand", zone)
	}
	if zone := g.Card(other2).Zone; zone != engine.Hand {
		t.Errorf("other2 zone = %v, want Hand", zone)
	}
	if zone := g.Card(source).Zone; zone != engine.Battlefield {
		t.Errorf("source zone = %v, want Battlefield -- this cost did not choose the source", zone)
	}
}

// TestActivateAbilityDeclinesWhenNotEnoughReturnTypeCandidates proves the
// feasibility check (returnTypeCandidates, returncost.go) runs before
// anything is committed: fewer type-matched permanents than ReturnTypeN
// declines outright rather than asking the controller for more than exist.
func TestActivateAbilityDeclinesWhenNotEnoughReturnTypeCandidates(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Return Type Short", "AB$ GainLife | Cost$ Return<3/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, source, 0, c) {
		t.Error("ActivateAbility returned true with only 2 Creatures for Return<3/Creature>, want false")
	}
}

// TestActivateAbilityDeclinesForNonLiteralReturnTypeCost proves a
// Return<...> naming anything but a literal positive integer -- an X-cost
// here -- still declines outright: ActivationShape's own doc comment has
// the reason (internal/cost).
func TestActivateAbilityDeclinesForNonLiteralReturnTypeCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Return Type X", "AB$ GainLife | Cost$ X Return<X/Creature> | Defined$ You | LifeAmount$ 3")
	source := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, source, 0, c) {
		t.Error("ActivateAbility returned true for Return<X/Creature>, want false")
	}
}

// TestActivateAbilityDeclinesOutsideMainPhaseWithEmptyStack proves timing
// collapses to CastSpell's own sorcery-speed shape: wrong phase, a
// nonempty stack and a wrong-controller/off-battlefield source are all
// declined the identical way.
func TestActivateAbilityDeclinesOutsideMainPhaseWithEmptyStack(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.CombatBegin)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Wrong Phase", "AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true during combat, want false")
	}
}
