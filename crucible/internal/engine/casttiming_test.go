package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castTimingCase is one attempt to cast (or activate) at a given moment.
type castTimingCase struct {
	name string
	// caster is the player's index in g.Players(); the game's active player is index 0.
	caster int
	phase  engine.PhaseType
	want   bool
}

// TestCastInstantHonoursPrintedTimingRestrictions proves an Instant's own
// "cast only..." restrictions (SpellAbilityRestriction.checkTimingRestrictions:
// ActivationPhases$, PlayerTurn$, OpponentTurn$) are enforced by CastSpell.
// A declined cast leaves the card in hand and the mana unspent.
func TestCastInstantHonoursPrintedTimingRestrictions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		param string
		cases []castTimingCase
	}{
		{"ActivationPhases single step", "ActivationPhases$ Declare Blockers", []castTimingCase{
			{"main phase", 0, engine.Main1, false},
			{"combat damage step", 0, engine.CombatDamage, false},
			{"declare blockers", 0, engine.DeclareBlockers, true},
			{"declare blockers by the opponent", 1, engine.DeclareBlockers, true},
		}},
		{"ActivationPhases range", "ActivationPhases$ Upkeep->Declare Blockers", []castTimingCase{
			{"upkeep", 0, engine.Upkeep, true},
			{"declare attackers", 0, engine.DeclareAttackers, true},
			{"combat damage step", 0, engine.CombatDamage, false},
		}},
		{"PlayerTurn", "PlayerTurn$ True", []castTimingCase{
			{"active player", 0, engine.Main1, true},
			{"opponent", 1, engine.Main1, false},
		}},
		{"OpponentTurn", "OpponentTurn$ True", []castTimingCase{
			{"active player", 0, engine.Main1, false},
			{"opponent", 1, engine.Main1, true},
		}},
		{"PlayerTurn and phases together", "PlayerTurn$ True | ActivationPhases$ Declare Attackers", []castTimingCase{
			{"active player, right step", 0, engine.DeclareAttackers, true},
			{"active player, wrong step", 0, engine.Main1, false},
			{"opponent, right step", 1, engine.DeclareAttackers, false},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, c := range tc.cases {
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()

					g, p, other := newTwoPlayerGame(t)
					caster := []engine.PlayerID{p, other}[c.caster]
					g.SetTurnState(1, p, c.phase)
					g.Player(caster).ManaPool.Add(mana.White, 1)
					def := instantDefWithAbility(t, "Test Timed Instant", "W",
						"SP$ GainLife | Defined$ You | LifeAmount$ 3 | "+tc.param)
					spell := g.NewCard(def, caster, engine.Hand)

					got := g.CastSpell(caster, spell, engine.NewScriptedController())
					if got != c.want {
						t.Fatalf("CastSpell = %v, want %v", got, c.want)
					}
					wantZone, wantMana := engine.Hand, 1
					if c.want {
						wantZone, wantMana = engine.Stack, 0
					}
					if z := g.Card(spell).Zone; z != wantZone {
						t.Errorf("spell zone = %v, want %v", z, wantZone)
					}
					if n := g.Player(caster).ManaPool.Total(); n != wantMana {
						t.Errorf("mana left = %d, want %d (a declined cast pays nothing)", n, wantMana)
					}
				})
			}
		})
	}
}

// TestCastSorceryHonoursPrintedPhases proves a Sorcery's ActivationPhases$
// narrows its ordinary sorcery-speed window: it never widens it.
func TestCastSorceryHonoursPrintedPhases(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		phase engine.PhaseType
		want  bool
	}{
		{engine.Main1, false},
		{engine.Main2, true},
		{engine.DeclareBlockers, false},
	} {
		t.Run(tc.phase.String(), func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.SetTurnState(1, p, tc.phase)
			g.Player(p).ManaPool.Add(mana.White, 1)
			def := sorceryDefWithAbility(t, "Test Second Main Sorcery", "W",
				"SP$ GainLife | Defined$ You | LifeAmount$ 3 | ActivationPhases$ Main2")
			spell := g.NewCard(def, p, engine.Hand)

			if got := g.CastSpell(p, spell, engine.NewScriptedController()); got != tc.want {
				t.Errorf("CastSpell in %v = %v, want %v", tc.phase, got, tc.want)
			}
		})
	}
}

// TestCastInstantUnreadableActivationPhasesDeclines proves an
// ActivationPhases$ naming no step refuses the cast rather than dropping the
// restriction (GO-7).
func TestCastInstantUnreadableActivationPhasesDeclines(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.Player(p).ManaPool.Add(mana.White, 1)
	def := instantDefWithAbility(t, "Test Unreadable Timing", "W",
		"SP$ GainLife | Defined$ You | LifeAmount$ 3 | ActivationPhases$ Nowhere")
	spell := g.NewCard(def, p, engine.Hand)

	if g.CastSpell(p, spell, engine.NewScriptedController()) {
		t.Error("CastSpell succeeded with an unreadable ActivationPhases$")
	}
}

// TestActivateAbilityHonoursPlayerAndOpponentTurn proves an activated ability
// carrying PlayerTurn$ / OpponentTurn$ is limited the same way a spell is.
func TestActivateAbilityHonoursPlayerAndOpponentTurn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		param  string
		active bool // true: the activator's own turn
		want   bool
	}{
		{"PlayerTurn$ True", true, true},
		{"PlayerTurn$ True", false, false},
		{"OpponentTurn$ True", true, false},
		{"OpponentTurn$ True", false, true},
	} {
		name := tc.param + map[bool]string{true: "/own turn", false: "/opponent's turn"}[tc.active]
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			activator := p
			if !tc.active {
				g.SetTurnState(1, other, engine.Main1)
			}
			def := creatureDefWithAbility(t, "Test Turn-Limited",
				"AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | "+tc.param)
			creature := g.NewCard(def, activator, engine.Battlefield)

			got := g.ActivateAbility(activator, creature, 0, engine.NewScriptedController())
			if got != tc.want {
				t.Errorf("ActivateAbility = %v, want %v", got, tc.want)
			}
			if g.Card(creature).Tapped != tc.want {
				t.Errorf("Tapped = %v, want %v (a declined activation pays no cost)", g.Card(creature).Tapped, tc.want)
			}
		})
	}
}

// TestCastInstantFirstCombatOnly proves ActivationFirstCombat$ allows the
// turn's first combat and refuses an extra one
// (SpellAbilityRestriction.java:300-304).
func TestCastInstantFirstCombatOnly(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	extra := g.NewCard(creatureDefWithAbility(t, "Test Extra Combat",
		"AB$ AddPhase | Cost$ 0 | ExtraPhase$ Combat | AfterPhase$ EndCombat"), p, engine.Battlefield)
	if !g.ActivateAbility(p, extra, 0, c) {
		t.Fatal("ActivateAbility (extra combat) declined")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	castAtDeclareAttackers := func() bool {
		g.Player(p).ManaPool.Add(mana.White, 1)
		def := instantDefWithAbility(t, "Test First Combat Only", "W",
			"SP$ GainLife | Defined$ You | LifeAmount$ 1 | ActivationFirstCombat$ True")
		return g.CastSpell(p, g.NewCard(def, p, engine.Hand), c)
	}
	combats := 0
	var results []bool
	for g.ActivePhase() != engine.Cleanup {
		g.AdvancePhase(c)
		switch g.ActivePhase() {
		case engine.CombatBegin:
			combats++
		case engine.DeclareAttackers:
			results = append(results, castAtDeclareAttackers())
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
		}
	}
	if combats != 2 {
		t.Fatalf("combats = %d, want 2", combats)
	}
	if len(results) != 2 || !results[0] || results[1] {
		t.Errorf("casts at declare attackers of combat 1 and 2 = %v, want [true false]", results)
	}
}

// TestCastInstantAfterBlockersRefusedOnceBlockersWereSkipped proves
// ActivationAfterBlockers$ (CR 506.7f, SpellAbilityRestriction.java:307-311)
// refuses once a combat with no attackers skipped its declare blockers step,
// and allows the same cast before any combat.
func TestCastInstantAfterBlockersRefusedOnceBlockersWereSkipped(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	cast := func() bool {
		g.Player(p).ManaPool.Add(mana.White, 1)
		def := instantDefWithAbility(t, "Test After Blockers", "W",
			"SP$ GainLife | Defined$ You | LifeAmount$ 1 | ActivationAfterBlockers$ True")
		return g.CastSpell(p, g.NewCard(def, p, engine.Hand), c)
	}

	if !cast() {
		t.Fatal("cast before any combat declined")
	}
	// Step, unlike AdvancePhase, is the turn driver: it skips declare blockers
	// and the damage steps when nothing attacked.
	for g.ActivePhase() != engine.Main2 {
		if err := g.Step(engine.NewRegistry(), c); err != nil {
			t.Fatalf("Step: %v", err)
		}
	}
	if cast() {
		t.Error("cast after a combat that skipped declare blockers succeeded")
	}
}

// TestTimedSpellResolvesOnceCast proves a spell whose effect used to refuse
// PlayerTurn$/ActivationPhases$ outright (the effect could not read a
// restriction) now resolves: cast timing is the restriction's whole job, so
// the effect ignores the keys the way Java does.
func TestTimedSpellResolvesOnceCast(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDef(t), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 1)
	def := instantDefWithAbility(t, "Test Timed Wrath", "W",
		"SP$ DestroyAll | ValidCards$ Creature | PlayerTurn$ True | ActivationPhases$ Main1")
	spell := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, spell, c) {
		t.Fatal("CastSpell declined in the active player's Main1")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(victim).Zone; z != engine.Graveyard {
		t.Errorf("victim zone = %v, want Graveyard", z)
	}
}

// TestPlayDoesNotOfferASpellOutsideItsPrintedTiming proves an effect's "cast
// it" still honours the spell's own ActivationPhases$/PlayerTurn$ (Java's
// "extra timing restrictions still apply", AbilityUtils.java:2944): the spell
// is not offered, so it stays in exile, while the same spell inside its window
// is cast.
func TestPlayDoesNotOfferASpellOutsideItsPrintedTiming(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		phase engine.PhaseType
		want  engine.ZoneType
	}{
		{engine.Main1, engine.Exile},
		{engine.DeclareBlockers, engine.Graveyard},
	} {
		t.Run(tc.phase.String(), func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.SetTurnState(1, p, tc.phase)
			def := instantDefWithAbility(t, "Test Blockers Gain", "W",
				"SP$ GainLife | Defined$ You | LifeAmount$ 3 | ActivationPhases$ Declare Blockers")
			spell := g.NewCard(def, p, engine.Exile)
			c := engine.NewScriptedController()
			host := pushPlayLine(t, g, p, nil, "DB$ Play | Defined$ Remembered | ValidSA$ Spell | WithoutManaCost$ True")
			if err := resolvePlay(t, g, host, c, spell); err != nil {
				t.Fatal(err)
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			if z := g.Card(spell).Zone; z != tc.want {
				t.Errorf("spell zone = %v, want %v", z, tc.want)
			}
		})
	}
}

// TestActivateManaAbilityHonoursPlayerTurn proves a mana ability naming
// PlayerTurn$ is admitted and limited to its controller's turn.
func TestActivateManaAbilityHonoursPlayerTurn(t *testing.T) {
	t.Parallel()

	for _, ownTurn := range []bool{true, false} {
		g, p, other := newTwoPlayerGame(t)
		if !ownTurn {
			g.SetTurnState(1, other, engine.Main1)
		}
		def := creatureDefWithAbility(t, "Test Own-Turn Dork", "AB$ Mana | Cost$ T | Produced$ G | PlayerTurn$ True")
		dork := g.NewCard(def, p, engine.Battlefield)

		if got := g.ActivateManaAbility(p, dork, 0, engine.NewScriptedController()); got != ownTurn {
			t.Errorf("ActivateManaAbility on own turn = %v: got %v, want %v", ownTurn, got, ownTurn)
		}
	}
}
