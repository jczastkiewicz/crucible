package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestChooseTypeNoteRecordsWithoutChoosingAndSkipsNotedTypes proves Note$
// (ChooseTypeEffect.java:117-121, :138-142): the pick is appended to the
// host's noted types, not written as its chosen type, and a later ChooseType
// on the same card no longer offers it.
func TestChooseTypeNoteRecordsWithoutChoosingAndSkipsNotedTypes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Noter", "DB$ ChooseType | Defined$ You | Type$ Creature | ValidTypes$ Elf,Goblin,Human | Note$ True")
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	rec := &optionRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueOption(0)
	rec.QueueOption(0)
	for range 2 {
		g.PushAbility(engine.Ability{API: engine.APIChooseType, Source: host, Controller: p, Params: sub})
		if err := g.ResolveStack(engine.NewRegistry(), rec); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	if got := g.Card(host).Memory.NotedTypes(); !slices.Equal(got, []string{"Elf", "Goblin"}) {
		t.Errorf("noted = %v, want [Elf Goblin]", got)
	}
	if got := g.Card(host).Memory.ChosenType(false); got != "" {
		t.Errorf("chosen type = %q, want none", got)
	}
	if len(rec.offers) != 2 || !slices.Equal(rec.offers[1], []string{"Goblin", "Human"}) {
		t.Errorf("offers = %v, want the second without Elf", rec.offers)
	}
}

// TestChooseTypeMostPrevalentInDefinedZone proves MostPrevalentInDefinedZone$
// (Favored Enemy): the creature types held by the most cards of the named
// player's zone are offered, and the NotedType valid property then matches a
// creature of the noted type (CardStateProperty.java:107-115).
func TestChooseTypeMostPrevalentInDefinedZone(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(etbChainDef(t, "Elf One", "DB$ BlankLine"), opp, engine.Library)
	g.NewCard(etbChainDef(t, "Elf Two", "DB$ BlankLine"), opp, engine.Library)
	rec := &optionRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueOption(0)
	host, err := resolveNow(t, g, p, rec.ScriptedController, nil, "DB$ ChooseType | Defined$ You | Type$ Creature | MostPrevalentInDefinedZone$ Player.Opponent_Library | Note$ True")
	if err != nil {
		t.Fatalf("ChooseType: %v", err)
	}
	if got := g.Card(host).Memory.NotedTypes(); !slices.Equal(got, []string{"Elf"}) {
		t.Errorf("noted = %v, want [Elf]", got)
	}
}

// TestRepeatEachYourTeamAndAlliesWithoutTeams proves the team-less reading of
// PlayerProperty.java:50-57: Player.YourTeam is the controller alone and
// Player.Allies nobody (Azra Bladeseeker, Imperial Mask).
func TestRepeatEachYourTeamAndAlliesWithoutTeams(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		spec string
		want int
	}{{"Player.YourTeam", 22}, {"Player.Allies", 20}} {
		g, p, _ := newTwoPlayerGame(t)
		_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
			"DB$ RepeatEach | RepeatPlayers$ "+tc.spec+" | RepeatSubAbility$ DBGain", "DBGain", "DB$ GainLife | Defined$ Remembered | LifeAmount$ 2")
		if err != nil {
			t.Fatalf("%s: %v", tc.spec, err)
		}
		if got := g.Player(p).Life; got != tc.want {
			t.Errorf("%s: life = %d, want %d", tc.spec, got, tc.want)
		}
	}
}

// TestRepeatEachNextTurnForEachPlayerWaitsForThatPlayersTurn proves
// NextTurnForEachPlayer$ (RepeatEachEffect.java:169-183): the sub-ability for
// an opponent resolves when their next turn begins, with that player
// remembered, and not before.
func TestRepeatEachNextTurnForEachPlayerWaitsForThatPlayersTurn(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, opp, 8)
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ RepeatEach | RepeatPlayers$ Player.Opponent | NextTurnForEachPlayer$ True | RepeatSubAbility$ DBGain", "DBGain", "DB$ GainLife | Defined$ Remembered | LifeAmount$ 3"); err != nil {
		t.Fatalf("RepeatEach: %v", err)
	}
	if got := g.Player(opp).Life; got != 20 {
		t.Fatalf("opponent life = %d right away, want 20", got)
	}
	toStep(t, g, c, opp, engine.Upkeep, nil)
	if got := g.Player(opp).Life; got != 23 {
		t.Errorf("opponent life = %d on their turn, want 23", got)
	}
}
