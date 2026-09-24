package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// TestFlipCoinEffectCalledFlipPicksWinOrLose proves CR 705: the flipper
// calls, one nextBoolean decides, and exactly one of WinSubAbility$/
// LoseSubAbility$ resolves -- which one follows the game's seeded stream.
func TestFlipCoinEffectCalledFlipPicksWinOrLose(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueCoinCall(true)
	def := etbChainDef(t, "Test Coin",
		"DB$ FlipCoin | WinSubAbility$ DBWin | LoseSubAbility$ DBLose",
		"DBWin", "DB$ GainLife | Defined$ You | LifeAmount$ 5",
		"DBLose", "DB$ LoseLife | Defined$ You | LifeAmount$ 5")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	want := 15
	if javarand.New(1).Bool() {
		want = 25
	}
	if got := g.Player(p).Life; got != want {
		t.Errorf("life = %d, want %d", got, want)
	}
}

// TestRollDiceEffectResultPicksRangeAndBindsSVar proves the result picks
// the ResultSubAbilities$ range covering it and ResultSVar$ carries the
// total into the SubAbility$ chain.
func TestRollDiceEffectResultPicksRangeAndBindsSVar(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test Die",
		"DB$ RollDice | Sides$ 6 | ResultSVar$ Result | ResultSubAbilities$ 1-3:DBLow,4-6:DBHigh | SubAbility$ DBResult",
		"DBLow", "DB$ GainLife | Defined$ You | LifeAmount$ 100",
		"DBHigh", "DB$ GainLife | Defined$ You | LifeAmount$ 200",
		"DBResult", "DB$ GainLife | Defined$ You | LifeAmount$ Result")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	roll := int(javarand.New(1).Int32n(6)) + 1
	want := 20 + roll + 100
	if roll >= 4 {
		want = 20 + roll + 200
	}
	if got := g.Player(p).Life; got != want {
		t.Errorf("life = %d, want %d (roll %d)", got, want, roll)
	}
}

// TestClashEffectHigherManaValueWins proves CR 701.30: the clasher's top
// card has the higher mana value, so WinSubAbility$ resolves, and a player
// answering "bottom" sends their revealed card under their library.
func TestClashEffectHigherManaValueWins(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefManaCost(t, "2 G"), p, engine.Library)
	cheap := g.NewCard(creatureDefManaCost(t, "G"), other, engine.Library)
	under := g.NewCard(creatureDefManaCost(t, "G"), other, engine.Library)
	c := engine.NewScriptedController()
	c.QueueCardOnTop(true)
	c.QueueCardOnTop(false)
	def := etbChainDef(t, "Test Clash",
		"DB$ Clash | WinSubAbility$ DBWin | OtherwiseSubAbility$ DBLose",
		"DBWin", "DB$ GainLife | Defined$ You | LifeAmount$ 4",
		"DBLose", "DB$ LoseLife | Defined$ You | LifeAmount$ 4")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 24 {
		t.Errorf("life = %d, want 24 (clash won)", got)
	}
	if lib := g.Zone(engine.Library, other).Cards(); lib[0] != under || lib[1] != cheap {
		t.Errorf("other's library = %v, want the revealed card moved under %v", lib, under)
	}
}

// TestSeekEffectPutsMatchingCardInHand proves Seek draws only from cards
// matching Type$.
func TestSeekEffectPutsMatchingCardInHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(landDef(t, "Test Forest", "Land Forest"), p, engine.Library)
	creature := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Library)
	g.NewCard(landDef(t, "Test Forest", "Land Forest"), p, engine.Library)
	def := etbChainDef(t, "Test Seeker", "DB$ Seek | Type$ Creature | RememberFound$ True")
	host, err := castETBChain(t, g, p, def, engine.NewScriptedController())
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(creature).Zone; z != engine.Hand {
		t.Errorf("creature zone = %v, want Hand", z)
	}
	if r := g.Card(host).Memory.Remembered(); len(r) != 1 || r[0] != engine.CardEntity(creature) {
		t.Errorf("host remembers %v, want the sought creature", r)
	}
}

// TestStoreSVarEffectFeedsLaterAmount proves StoreSVar's value is what a
// later param naming that SVar reads (Number$, then CountSVar$ X/Plus.1).
func TestStoreSVarEffectFeedsLaterAmount(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test Store",
		"DB$ StoreSVar | SVar$ X | Type$ Number | Expression$ 4 | SubAbility$ DBBump",
		"DBBump", "DB$ StoreSVar | SVar$ X | Type$ CountSVar | Expression$ X/Plus.1 | SubAbility$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ X")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 25 {
		t.Errorf("life = %d, want 25", got)
	}
}

// TestBalanceEffectSacrificesDownToFewest proves every player sacrifices
// down to the smallest count among them.
func TestBalanceEffectSacrificesDownToFewest(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{a, b})
	def := etbChainDef(t, "Test Balance", "DB$ Balance | Valid$ Creature")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range []engine.CardID{a, b} {
		if z := g.Card(id).Zone; z != engine.Graveyard {
			t.Errorf("sacrificed creature %v zone = %v, want Graveyard", id, z)
		}
	}
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("host zone = %v, want Battlefield", z)
	}
}

// TestAddPhaseEffectExtraCombatThenMain proves CR 500.8: an extra combat
// followed by an extra main phase after EndCombat, then the turn's own
// second main phase.
func TestAddPhaseEffectExtraCombatThenMain(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Relentless",
		"DB$ AddPhase | ExtraPhase$ Combat | AfterPhase$ EndCombat | FollowedBy$ Main2")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	var seen []engine.PhaseType
	for g.ActivePhase() != engine.Cleanup {
		g.AdvancePhase(c)
		seen = append(seen, g.ActivePhase())
	}
	want := []engine.PhaseType{
		engine.CombatBegin, engine.DeclareAttackers, engine.DeclareBlockers, engine.FirstStrikeDamage, engine.CombatDamage, engine.CombatEnd,
		engine.CombatBegin, engine.DeclareAttackers, engine.DeclareBlockers, engine.FirstStrikeDamage, engine.CombatDamage, engine.CombatEnd,
		engine.Main2, engine.Main2, engine.EndOfTurn, engine.Cleanup,
	}
	if len(seen) != len(want) {
		t.Fatalf("phases = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("phases = %v, want %v", seen, want)
		}
	}
}

// TestSkipPhaseEffectSkipsOpponentsDraw proves the targeted player's next
// draw step is passed over: their turn goes Upkeep -> Main1 and their hand
// does not grow.
func TestSkipPhaseEffectSkipsOpponentsDraw(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	libraryCards(t, g, p, 3)
	libraryCards(t, g, other, 3)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Skip Draw", "DB$ SkipPhase | Defined$ Opponent | Step$ Draw")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for g.ActivePlayer() != other || g.ActivePhase() != engine.Upkeep {
		g.AdvancePhase(c)
	}
	g.AdvancePhase(c)
	if got := g.ActivePhase(); got != engine.Main1 {
		t.Errorf("phase after other's upkeep = %v, want Main1", got)
	}
	if got := len(g.Zone(engine.Hand, other).Cards()); got != 0 {
		t.Errorf("other's hand = %d, want 0", got)
	}
}

// TestFirstCombatOnlyInFirstOfTwoCombats proves FirstCombat$ reads the
// combats begun this turn: with an AddPhase extra combat, an "end of
// combat, first combat only" trigger fires once, not twice.
func TestFirstCombatOnlyInFirstOfTwoCombats(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "test_first_combat"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test First Combat"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigAdd",
		"Mode$ Phase | Phase$ EndCombat | FirstCombat$ True | TriggerZones$ Battlefield | Execute$ TrigGain",
	}
	raw.Faces[0].SVars.Set("TrigAdd", "DB$ AddPhase | ExtraPhase$ Combat | AfterPhase$ EndCombat | ConditionFirstCombat$ True")
	raw.Faces[0].SVars.Set("TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	combats := 0
	for g.ActivePhase() != engine.Cleanup {
		g.AdvancePhase(c)
		if g.ActivePhase() == engine.CombatBegin {
			combats++
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	if combats != 2 {
		t.Fatalf("combats = %d, want 2", combats)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21 (first combat only)", got)
	}
}
