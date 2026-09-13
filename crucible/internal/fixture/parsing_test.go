package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

func parse(t *testing.T, text string) *fixture.State {
	t.Helper()
	st, err := fixture.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return st
}

func TestGlobalDirectives(t *testing.T) {
	t.Parallel()

	st := parse(t, `# a comment
turn=4
activeplayer=human
activephase=Main1
removesummoningsickness=true
`)
	if st.Turn != 4 {
		t.Errorf("turn %d, want 4", st.Turn)
	}
	if st.ActivePlayer != "human" {
		t.Errorf("active player %q", st.ActivePlayer)
	}
	if !st.Phased || st.ActivePhase != engine.Main1 {
		t.Errorf("phase %v phased=%v, want Main1", st.ActivePhase, st.Phased)
	}
	if !st.RemoveSummoningSickness {
		t.Error("removesummoningsickness did not take")
	}
}

// Untap is a real phase and the zero value of PhaseType, so "no phase given"
// needs its own flag. Without one, every fixture that omits the phase would
// silently start in untap.
func TestUnsetPhaseIsNotUntap(t *testing.T) {
	t.Parallel()

	st := parse(t, "turn=1\n")
	if st.Phased {
		t.Error("a fixture with no phase reports one")
	}
	if st.ActivePhase != engine.Untap {
		t.Errorf("zero phase is %v", st.ActivePhase)
	}
}

// Player slots: human is 0, ai is 1, and p<n> is n.
func TestPlayerSlots(t *testing.T) {
	t.Parallel()

	st := parse(t, `humanlife=20
ailife=17
p3life=11
`)
	if got := st.Players[0].Life; got != 20 {
		t.Errorf("human life %d", got)
	}
	if got := st.Players[1].Life; got != 17 {
		t.Errorf("ai life %d", got)
	}
	if got := st.Players[3].Life; got != 11 {
		t.Errorf("p3 life %d", got)
	}
	if st.Players[2].Named {
		t.Error("an unaddressed slot is marked as named")
	}
}

// `p` takes one digit and one only, which is what
// Integer.parseInt(String.valueOf(key.charAt(1))) does. `p10life` is player 1
// with an unrecognised category, not player 10 (PORT-7).
func TestPlayerIndexIsOneDigit(t *testing.T) {
	t.Parallel()

	st := parse(t, "p10life=5\n")
	if got := st.Players[1].Life; got != 5 {
		t.Errorf("p10life gave player 1 life %d, want 5 -- the suffix still matches", got)
	}
	if !st.Players[1].Named {
		t.Error("p10life did not address player 1")
	}
}

// Zones keep their value verbatim: turning a card list into cards needs the
// database and the arena, so the parser must not touch it.
func TestZoneValuesAreVerbatim(t *testing.T) {
	t.Parallel()

	const list = "Grizzly Bears|Set:M10|Tapped:True;Llanowar Elves"
	st := parse(t, "humanbattlefield="+list+"\nhumanplay="+list+"\n")
	if st.Players[0].Battlefield != list {
		t.Errorf("battlefield %q, want it unchanged", st.Players[0].Battlefield)
	}
}

// Every zone besides battlefield/play keeps its value verbatim too.
func TestEveryZoneIsVerbatim(t *testing.T) {
	t.Parallel()

	st := parse(t, `humangraveyard=Lightning Bolt
humanlibrary=Mountain
humanexile=Path to Exile
humancommand=Emrakul, the Aeons Torn
humansideboard=Pyroblast
`)
	p := st.Players[0]
	if p.Graveyard != "Lightning Bolt" {
		t.Errorf("graveyard %q", p.Graveyard)
	}
	if p.Library != "Mountain" {
		t.Errorf("library %q", p.Library)
	}
	if p.Exile != "Path to Exile" {
		t.Errorf("exile %q", p.Exile)
	}
	if p.Command != "Emrakul, the Aeons Torn" {
		t.Errorf("command %q", p.Command)
	}
	if p.Sideboard != "Pyroblast" {
		t.Errorf("sideboard %q", p.Sideboard)
	}
}

// Mana pool, persistent mana, precast spells and a card already on the stack
// are each their own category.
func TestManaAndStackCategories(t *testing.T) {
	t.Parallel()

	st := parse(t, `humanmanapool=R R
humanpersistentmana=G
humanprecast=Ponder
humanputonstack=Lightning Bolt
`)
	p := st.Players[0]
	if p.ManaPool != "R R" {
		t.Errorf("mana pool %q", p.ManaPool)
	}
	if p.PersistentMana != "G" {
		t.Errorf("persistent mana %q", p.PersistentMana)
	}
	if p.PrecastSpells != "Ponder" {
		t.Errorf("precast %q", p.PrecastSpells)
	}
	if p.PutOnStack != "Lightning Bolt" {
		t.Errorf("put on stack %q", p.PutOnStack)
	}
}

// An invented activephaseadvance fails the same way an invented activephase
// does -- both name a real PhaseType or the fixture is wrong, not empty.
func TestUnknownPhaseAdvanceFails(t *testing.T) {
	t.Parallel()

	if _, err := fixture.Parse(strings.NewReader("activephaseadvance=Teatime\n")); err == nil {
		t.Error("an invented phase advance parsed without error")
	}
}

// A value may contain `=`; only the first one separates. Counter values are
// written `NAME=count`, so this is not a corner case.
func TestOnlyTheFirstEqualsSeparates(t *testing.T) {
	t.Parallel()

	st := parse(t, "humancounters=POISON=3\n")
	if got := st.Players[0].Counters; got != "POISON=3" {
		t.Errorf("counters %q, want POISON=3", got)
	}
}

// An empty value is a value, not a missing line: `humanhand=` means an empty
// hand.
func TestEmptyValueIsAValue(t *testing.T) {
	t.Parallel()

	st := parse(t, "humanhand=\n")
	if !st.Players[0].Named {
		t.Error("an empty value did not address the player")
	}
	if st.Players[0].Hand != "" {
		t.Errorf("hand %q, want empty", st.Players[0].Hand)
	}
}

// Comments, blank lines and lines with no `=` are dropped. Java crashes on a
// blank line; skipping it is the one deliberate difference, and it must not
// affect anything else.
func TestNoiseIsDropped(t *testing.T) {
	t.Parallel()

	st := parse(t, `# comment

not a directive
humanlife=20
`)
	if st.Players[0].Life != 20 {
		t.Errorf("life %d after noise, want 20", st.Players[0].Life)
	}
	if len(st.Unknown) != 0 {
		t.Errorf("noise was reported as unknown keys: %v", st.Unknown)
	}
}

// A key whose shape is right but whose category is not is returned rather than
// dropped. Java prints these to stderr and continues; a fixture that silently
// ignores a line is a test asserting something other than what it says.
func TestUnknownCategoriesAreReported(t *testing.T) {
	t.Parallel()

	st := parse(t, `humanwidgets=3
activewidget=x
sometotalnonsense=1
`)
	if len(st.Unknown) != 3 {
		t.Errorf("unknown keys %v, want three", st.Unknown)
	}
}

// Keys are case-insensitive, because Java lowercases the whole key before
// matching. Values are not.
func TestKeysFoldValuesDoNot(t *testing.T) {
	t.Parallel()

	st := parse(t, "HumanLife=20\nHUMANHAND=Grizzly Bears\n")
	if st.Players[0].Life != 20 {
		t.Error("a capitalised key did not match")
	}
	if st.Players[0].Hand != "Grizzly Bears" {
		t.Errorf("hand %q -- the value was folded", st.Players[0].Hand)
	}
}

// A malformed number is an error naming the line, not a silent zero: a
// fixture with a typo in a life total would otherwise assert against 0 life
// and pass for the wrong reason.
func TestMalformedNumbersFail(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"humanlife=twenty", "turn=x", "humanlandsplayed=-"} {
		if _, err := fixture.Parse(strings.NewReader(line + "\n")); err == nil {
			t.Errorf("%q parsed without error", line)
		}
	}
}

// landsplayedlastturn has to be tested before landsplayed, or the shorter
// suffix swallows it.
func TestLongerSuffixWins(t *testing.T) {
	t.Parallel()

	st := parse(t, "humanlandsplayed=2\nhumanlandsplayedlastturn=1\n")
	if st.Players[0].LandsPlayed != 2 || st.Players[0].LandsPlayedLastTurn != 1 {
		t.Errorf("landsplayed=%d lastturn=%d, want 2 and 1",
			st.Players[0].LandsPlayed, st.Players[0].LandsPlayedLastTurn)
	}
}

func TestUnknownPhaseFails(t *testing.T) {
	t.Parallel()

	if _, err := fixture.Parse(strings.NewReader("activephase=Teatime\n")); err == nil {
		t.Error("an invented phase parsed without error")
	}
}

// activephaseadvance is a second phase, distinct from activephase, and must
// not be swallowed by the "phase" suffix match -- "activephaseadvance" does
// not end in "phase".
func TestActivePhaseAdvance(t *testing.T) {
	t.Parallel()

	st := parse(t, "activephase=Main1\nactivephaseadvance=BeginCombat\n")
	if !st.Phased || st.ActivePhase != engine.Main1 {
		t.Errorf("phase %v phased=%v, want Main1", st.ActivePhase, st.Phased)
	}
	if !st.PhaseAdvanced || st.ActivePhaseAdvance != engine.CombatBegin {
		t.Errorf("phase advance %v advanced=%v, want CombatBegin", st.ActivePhaseAdvance, st.PhaseAdvanced)
	}
}

// GameState.toString dumps PhaseType's bare enum name (MAIN1, COMBAT_BEGIN),
// not the script name (Main1, BeginCombat) engine.PhaseByName matches. A
// fixture generated by the Java oracle must still parse.
func TestJavaDumpedPhaseNames(t *testing.T) {
	t.Parallel()

	st := parse(t, "activephase=COMBAT_BEGIN\n")
	if !st.Phased || st.ActivePhase != engine.CombatBegin {
		t.Errorf("phase %v phased=%v, want CombatBegin", st.ActivePhase, st.Phased)
	}
}

// The Ring's temptation count and Speed (Alchemy) are per-player integers
// alongside life and lands played.
func TestRingTemptationAndSpeed(t *testing.T) {
	t.Parallel()

	st := parse(t, "humannumringtemptedyou=3\nhumanspeed=5\n")
	if st.Players[0].NumRingTemptedYou != 3 {
		t.Errorf("numringtemptedyou %d, want 3", st.Players[0].NumRingTemptedYou)
	}
	if st.Players[0].Speed != 5 {
		t.Errorf("speed %d, want 5", st.Players[0].Speed)
	}
}

// ability<key> lines are not player-scoped in Java -- they resolve targets
// for puzzle-mode precast spells by a key of the fixture author's choosing.
func TestAbilityStringsAreGlobal(t *testing.T) {
	t.Parallel()

	st := parse(t, "ability1=Targeting$ Player\n")
	if got := st.AbilityStrings["1"]; got != "Targeting$ Player" {
		t.Errorf("ability string %q, want %q", got, "Targeting$ Player")
	}
	if st.Players[0].Named {
		t.Error("an ability line addressed a player slot")
	}
}
