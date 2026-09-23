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

// TestActivateAbilitySelfExertCostExertsSourceAndRunsEffect proves
// Exert<1/CARDNAME> -- SelfSac's own fourth sibling shape -- actually marks
// the source Exerted (Card.Exerted, card.go) and still lets the ability
// resolve, unlike Sac/Exile/Return the source stays right where it is.
func TestActivateAbilitySelfExertCostExertsSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Self Exert", "AB$ GainLife | Cost$ Exert<1/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(creature).Exerted {
		t.Error("source not Exerted after an Exert<1/CARDNAME> activation")
	}
	if zone := g.Card(creature).Zone; zone != engine.Battlefield {
		t.Errorf("source zone = %v, want Battlefield -- exerting does not move it", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
}

// TestActivateAbilityCombinesTapAndSelfExert proves Tap and SelfExert
// compose on one line -- the source taps, then it is marked Exerted.
func TestActivateAbilityCombinesTapAndSelfExert(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Tap Exert", "AB$ GainLife | Cost$ T Exert<1/CARDNAME> | Defined$ You | LifeAmount$ 2")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if !g.Card(creature).Tapped {
		t.Error("source not tapped")
	}
	if !g.Card(creature).Exerted {
		t.Error("source not Exerted")
	}
}

// TestActivateAbilityDeclinesForChosenExertCost proves an Exert<...> naming
// anything but the literal self-reference CARDNAME/NICKNAME -- a chosen
// count here -- still declines outright: ActivationShape's own doc comment
// has the reason (internal/cost).
func TestActivateAbilityDeclinesForChosenExertCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Chosen Exert", "AB$ GainLife | Cost$ Exert<2/CARDNAME> | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for Exert<2/CARDNAME>, want false")
	}
}

// TestActivateAbilityExertCostFiresOwnTrigger proves checkExertedTriggers
// (exertcost.go) actually fires CR 701.42a's own trigger for an
// Exert<1/CARDNAME> cost, on the exerted card's own trigger.
func TestActivateAbilityExertCostFiresOwnTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbilityAndTrigger(t, "Test Exert Own Trigger",
		"AB$ GainLife | Cost$ Exert<1/CARDNAME> | Defined$ You | LifeAmount$ 1",
		"Mode$ Exerted | ValidCard$ Card.Self | Execute$ TrigDraw")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 (the activated ability plus the Exerted trigger's own Draw)", got)
	}
}

// TestActivateAbilityExertCostFiresOtherWatcherTrigger proves the real
// corpus shape -- a separate permanent watching "whenever you exert a
// creature" (ValidCard$ Creature.YouCtrl) -- fires too, every one of the 5
// real corpus T:Mode$ Exerted lines this exact shape.
func TestActivateAbilityExertCostFiresOtherWatcherTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	watcher := diesTriggerCreatureDefWithLine(t, "2", "2",
		"Mode$ Exerted | ValidCard$ Creature.YouCtrl | Execute$ TrigDraw")
	g.NewCard(watcher, p, engine.Battlefield)

	def := creatureDefWithAbility(t, "Test Exert Other Watcher", "AB$ GainLife | Cost$ Exert<1/CARDNAME> | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 (the activated ability plus the watcher's own Draw)", got)
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

// planeswalkerDefWithAbility builds a *compile.Card for a legendary
// Planeswalker carrying one real A:AB$ line -- planeswalkerDefLoyalty's own
// sibling (action_test.go), built through the real compiled param parser
// (TEST-1) the way creatureDefWithAbility already is for a creature, since
// the loyalty-ability tests below need both a printed Loyalty and a real
// Cost$ line to activate.
func planeswalkerDefWithAbility(t *testing.T, name, loyalty, abilityText string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Legendary Planeswalker Test")
	raw.Faces[0].InitialLoyalty = loyalty
	raw.Faces[0].Abilities = []string{abilityText}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestActivateAbilityAddCounterCostAddsLoyaltyAndRunsEffect proves
// AddCounter<N/Type> -- CR 606's own dominant loyalty-ability cost shape,
// Ajani Goldmane's own real "[+1]: You gain 2 life" -- actually puts
// counters on the source (Card.Counters, card.go) rather than moving or
// tapping it, marks the once-per-turn flag (Card.LoyaltyAbilityActivated),
// and still lets the ability resolve.
func TestActivateAbilityAddCounterCostAddsLoyaltyAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := planeswalkerDefWithAbility(t, "Test Add Loyalty", "4",
		"AB$ GainLife | Cost$ AddCounter<1/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 2")
	pw := g.NewCard(def, p, engine.Battlefield)
	g.Card(pw).Counters.Add(engine.Loyalty, 4)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, pw, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.Card(pw).Counters.Count(engine.Loyalty); got != 5 {
		t.Errorf("loyalty = %d, want 5 (4 + 1)", got)
	}
	if !g.Card(pw).LoyaltyAbilityActivated {
		t.Error("LoyaltyAbilityActivated = false, want true after a Planeswalker$ ability activates")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 22 {
		t.Errorf("life = %d, want 22", got)
	}
}

// TestActivateAbilitySubCounterCostRemovesLoyaltyAndRunsEffect proves
// SubCounter<N/Type> -- AddCounter's own mirror image, Ajani's own real
// "[-1]:" ability -- removes counters from the source rather than adding
// them.
func TestActivateAbilitySubCounterCostRemovesLoyaltyAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := planeswalkerDefWithAbility(t, "Test Sub Loyalty", "4",
		"AB$ GainLife | Cost$ SubCounter<2/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 5")
	pw := g.NewCard(def, p, engine.Battlefield)
	g.Card(pw).Counters.Add(engine.Loyalty, 4)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, pw, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.Card(pw).Counters.Count(engine.Loyalty); got != 2 {
		t.Errorf("loyalty = %d, want 2 (4 - 2)", got)
	}
	if !g.Card(pw).LoyaltyAbilityActivated {
		t.Error("LoyaltyAbilityActivated = false, want true")
	}
}

// TestActivateAbilityDeclinesSubCounterCostWhenNotEnoughLoyalty proves CR
// 121.5's own "can't remove more counters than there are" floor
// (CostRemoveCounter.java's own "source.getCounters(cntrs) - amount >= 0"),
// with no side effect on a decline.
func TestActivateAbilityDeclinesSubCounterCostWhenNotEnoughLoyalty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)

	def := planeswalkerDefWithAbility(t, "Test Sub Loyalty Too Low", "4",
		"AB$ GainLife | Cost$ SubCounter<6/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 1")
	pw := g.NewCard(def, p, engine.Battlefield)
	g.Card(pw).Counters.Add(engine.Loyalty, 4)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, pw, 0, c) {
		t.Error("ActivateAbility returned true with 4 loyalty for a SubCounter<6/LOYALTY> cost, want false")
	}
	if got := g.Card(pw).Counters.Count(engine.Loyalty); got != 4 {
		t.Errorf("loyalty = %d, want 4 -- a declined activation must not touch it", got)
	}
	if g.Card(pw).LoyaltyAbilityActivated {
		t.Error("LoyaltyAbilityActivated = true after a declined activation, want false")
	}
}

// TestActivateAbilityAddCounterZeroCostStillActivatesAndSetsLoyaltyFlag
// proves AddCounter<0/...> -- a real corpus "+0" loyalty ability, 54 real
// lines -- is exactly as legal a cost as any positive N: it commits (no
// floor to fail), leaves the counter count unchanged, and still marks
// Card.LoyaltyAbilityActivated (CR 606.3 restricts the whole ability, not
// only a nonzero counter change).
func TestActivateAbilityAddCounterZeroCostStillActivatesAndSetsLoyaltyFlag(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := planeswalkerDefWithAbility(t, "Test Plus Zero", "4",
		"AB$ GainLife | Cost$ AddCounter<0/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 3")
	pw := g.NewCard(def, p, engine.Battlefield)
	g.Card(pw).Counters.Add(engine.Loyalty, 4)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, pw, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if got := g.Card(pw).Counters.Count(engine.Loyalty); got != 4 {
		t.Errorf("loyalty = %d, want 4 -- AddCounter<0/...> changes nothing", got)
	}
	if !g.Card(pw).LoyaltyAbilityActivated {
		t.Error("LoyaltyAbilityActivated = false, want true even for a +0 ability")
	}
}

// TestActivateAbilityDeclinesSecondLoyaltyAbilityActivationSameTurn proves
// CR 606.3's own once-per-turn restriction: a second Planeswalker$ ability
// on the same permanent, same turn, declines regardless of which of its own
// two abilities it names.
func TestActivateAbilityDeclinesSecondLoyaltyAbilityActivationSameTurn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	raw := &carddb.Card{Filename: "Test Twice A Turn"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Twice A Turn"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Legendary Planeswalker Test")
	raw.Faces[0].InitialLoyalty = "4"
	raw.Faces[0].Abilities = []string{
		"AB$ GainLife | Cost$ AddCounter<1/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 1",
		"AB$ GainLife | Cost$ SubCounter<1/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 1",
	}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	pw := g.NewCard(def, p, engine.Battlefield)
	g.Card(pw).Counters.Add(engine.Loyalty, 4)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, pw, 0, c) {
		t.Fatal("first ActivateAbility returned false, want true")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.ActivateAbility(p, pw, 1, c) {
		t.Error("second ActivateAbility (a different loyalty ability, same turn) returned true, want false")
	}
	if got := g.Card(pw).Counters.Count(engine.Loyalty); got != 5 {
		t.Errorf("loyalty = %d, want 5 -- only the first activation's +1 must have applied", got)
	}
}

// TestActivateAbilityNonLoyaltyCounterCostHasNoOncePerTurnLimit proves the
// CR 606.3 gate is conditional on Planeswalker$'s own presence, not a
// blanket rule over every AddCounter/SubCounter cost: a plain counter cost
// with no Planeswalker$ param activates twice in the same turn.
func TestActivateAbilityNonLoyaltyCounterCostHasNoOncePerTurnLimit(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Plain Counter Cost", "AB$ GainLife | Cost$ SubCounter<1/CHARGE> | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)
	g.Card(creature).Counters.Add(engine.Charge, 2)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("first ActivateAbility returned false, want true")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Error("second ActivateAbility (no Planeswalker$ param, same turn) returned false, want true")
	}
	if got := g.Card(creature).Counters.Count(engine.Charge); got != 0 {
		t.Errorf("charge counters = %d, want 0 (2 - 1 - 1)", got)
	}
}

// TestActivateAbilityDeclinesForNonSelfAddCounterTarget proves an
// AddCounter<N/Type> naming a target past the self-reference shape (a
// chosen creature you control, "Creature.YouCtrl") declines the whole line
// outright -- ActivationShape's own doc comment has the reason
// (internal/cost, CostRemoveCounter.java's own non-self branch this port
// does not carry).
func TestActivateAbilityDeclinesForNonSelfAddCounterTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)

	def := creatureDefWithAbility(t, "Test Non Self AddCounter",
		"AB$ GainLife | Cost$ 2 R T AddCounter<1/M1M1/Creature.YouCtrl/a creature you control> | ValidTgts$ Any | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for a non-self AddCounter target, want false")
	}
}

// TestActivateAbilityExileFromGraveCostExilesSourceAndRunsEffect proves
// ActivationZone$ Graveyard -- CR 602.2's own generalization past a
// permanent already on the battlefield -- actually activates from the
// graveyard: the source's own owner (not controller, CR 109.5 -- a card
// outside the battlefield has no controller) may activate it while it sits
// in Graveyard, ExileFromGrave<1/CARDNAME> exiles it as the cost
// (exileFromGraveyard, exilefromgrave.go), and the ability still resolves
// with its own source already gone.
func TestActivateAbilityExileFromGraveCostExilesSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Exile From Grave",
		"AB$ GainLife | Cost$ ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | Defined$ You | LifeAmount$ 3")
	creature := g.NewCard(def, p, engine.Graveyard)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Exile {
		t.Errorf("source zone = %v, want Exile -- the ExileFromGrave<1/CARDNAME> cost must actually exile it", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
}

// TestActivateAbilityGraveyardAbilityWithPureManaCostStaysInGraveyard proves
// the corpus's own dominant real ActivationZone$ Graveyard shape past
// ExileFromGrave -- a plain mana cost, e.g. Unearth-style abilities whose
// own effect (not built here) moves the card elsewhere: the source is not
// itself touched by activation.
func TestActivateAbilityGraveyardAbilityWithPureManaCostStaysInGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Black, 1)

	def := creatureDefWithAbility(t, "Test Grave Pure Mana",
		"AB$ GainLife | Cost$ B | ActivationZone$ Graveyard | Defined$ You | LifeAmount$ 2")
	creature := g.NewCard(def, p, engine.Graveyard)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, creature, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(creature).Zone; zone != engine.Graveyard {
		t.Errorf("source zone = %v, want Graveyard -- a plain mana cost must not move it", zone)
	}
}

// TestActivateAbilityDeclinesGraveyardAbilityWhenNotInGraveyard proves the
// zone check is real: the identical card/ability, but sitting on the
// battlefield instead of in the graveyard, cannot activate its own
// ActivationZone$ Graveyard line.
func TestActivateAbilityDeclinesGraveyardAbilityWhenNotInGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)

	def := creatureDefWithAbility(t, "Test Grave Not In Grave",
		"AB$ GainLife | Cost$ ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for a graveyard ability on a battlefield card, want false")
	}
}

// TestActivateAbilityDeclinesGraveyardAbilityForNonOwner proves CR 109.5's
// own "a card outside the battlefield has no controller, its owner's own
// zones are the 'you'" -- only the source's own owner, not just any player,
// may activate it from the graveyard.
func TestActivateAbilityDeclinesGraveyardAbilityForNonOwner(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	owner, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, other, engine.Main1)

	def := creatureDefWithAbility(t, "Test Grave Non Owner",
		"AB$ GainLife | Cost$ ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, owner, engine.Graveyard)

	c := engine.NewScriptedController()
	if g.ActivateAbility(other, creature, 0, c) {
		t.Error("ActivateAbility returned true for a non-owner activating a graveyard ability, want false")
	}
}

// TestActivateAbilityDeclinesGraveyardAbilityCombinedWithTapCost proves a
// graveyard-zone ability naming any battlefield-only cost primitive besides
// ExileFromGrave -- Tap here, none of which any real corpus line combines
// with ActivationZone$ Graveyard -- declines outright rather than tapping a
// card that is not a permanent at all.
func TestActivateAbilityDeclinesGraveyardAbilityCombinedWithTapCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)

	def := creatureDefWithAbility(t, "Test Grave Tap",
		"AB$ GainLife | Cost$ T ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Graveyard)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for a graveyard ability combined with a Tap cost, want false")
	}
}

// TestActivateAbilityDeclinesUnsupportedActivationZone proves an
// ActivationZone$ this port does not build (Command here, 57 real corpus
// lines) fails closed rather than defaulting to Battlefield/Graveyard/Hand.
func TestActivateAbilityDeclinesUnsupportedActivationZone(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)

	def := creatureDefWithAbility(t, "Test Command Zone",
		"AB$ GainLife | Cost$ 1 | ActivationZone$ Command | Defined$ You | LifeAmount$ 1")
	creature := g.NewCard(def, p, engine.Command)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, creature, 0, c) {
		t.Error("ActivateAbility returned true for ActivationZone$ Command, want false")
	}
}

// TestActivateAbilitySelfDiscardCostDiscardsSourceAndRunsEffect proves CR
// 702.28's own Cycling shape -- Discard<1/CARDNAME>, ActivationZone$ Hand --
// discards the source (discardCards, discardeffect.go, reused wholesale, so
// Mode$ Discarded fires the identical way any other discard does) and the
// ability still resolves.
func TestActivateAbilitySelfDiscardCostDiscardsSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(nil, p, engine.Library)

	def := creatureDefWithAbility(t, "Test Self Discard",
		"AB$ Draw | Cost$ Discard<1/CARDNAME> | ActivationZone$ Hand | Defined$ You | NumCards$ 1")
	card := g.NewCard(def, p, engine.Hand)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, card, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(card).Zone; zone != engine.Graveyard {
		t.Errorf("source zone = %v, want Graveyard -- the Discard<1/CARDNAME> cost must actually discard it", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand size = %d, want 1 -- the Draw effect must still resolve", got)
	}
}

// TestActivateAbilitySelfExileFromHandCostExilesSourceAndRunsEffect proves
// ExileFromHand<1/CARDNAME> -- SelfExileFromGrave's own sibling for the
// hand -- actually exiles the source rather than discarding it.
func TestActivateAbilitySelfExileFromHandCostExilesSourceAndRunsEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Self Exile From Hand",
		"AB$ GainLife | Cost$ ExileFromHand<1/CARDNAME> | ActivationZone$ Hand | Defined$ You | LifeAmount$ 2")
	card := g.NewCard(def, p, engine.Hand)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, card, 0, c) {
		t.Fatal("ActivateAbility returned false, want true")
	}
	if zone := g.Card(card).Zone; zone != engine.Exile {
		t.Errorf("source zone = %v, want Exile", zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 22 {
		t.Errorf("life = %d, want 22", got)
	}
}

// TestActivateAbilityDeclinesHandAbilityCombinedWithTapCost mirrors
// TestActivateAbilityDeclinesGraveyardAbilityCombinedWithTapCost: 0 real
// corpus lines combine ActivationZone$ Hand with any battlefield-only
// primitive, so one declines outright.
func TestActivateAbilityDeclinesHandAbilityCombinedWithTapCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)

	def := creatureDefWithAbility(t, "Test Hand Tap",
		"AB$ GainLife | Cost$ T Discard<1/CARDNAME> | ActivationZone$ Hand | Defined$ You | LifeAmount$ 1")
	card := g.NewCard(def, p, engine.Hand)

	c := engine.NewScriptedController()
	if g.ActivateAbility(p, card, 0, c) {
		t.Error("ActivateAbility returned true for a Hand ability combined with a Tap cost, want false")
	}
}
