package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// drawerGame is a two-player game with one library card for p and a host that
// carries trigger (Execute$ TrigDraw draws p one card).
func drawerGame(t *testing.T, trigger string) (g *engine.Game, p, other engine.PlayerID, host engine.CardID) {
	t.Helper()
	g, p, other = newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	def := creatureDefWithAbilityAndTrigger(t, "Test Static Drawer", "AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1", trigger)
	return g, p, other, g.NewCard(def, p, engine.Battlefield)
}

func resolvedAbilities(sink *recordingSink) int {
	n := 0
	for _, e := range sink.events {
		if e.Kind == engine.AbilityResolved {
			n++
		}
	}
	return n
}

// A Static$ True ChangesZone trigger resolves at the zone change, never on the
// stack (TriggerHandler.java:300-309): the only resolution is the Destroy.
func TestStaticChangesZoneTriggerOfADeathResolvesInline(t *testing.T) {
	t.Parallel()

	g, p, _, host := drawerGame(t, "Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self | Execute$ TrigDraw | Static$ True")
	var sink recordingSink
	g.SetSink(&sink)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(host)}, "DB$ Destroy | ValidTgts$ Creature"); err != nil {
		t.Fatal(err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatal(err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand = %d, want 1: the static trigger drew", got)
	}
	if got := resolvedAbilities(&sink); got != 1 {
		t.Errorf("%d abilities resolved on the stack, want only the Destroy", got)
	}
}

// The same shape for a bounce and a tuck (left-to-hand and left-to-library
// paths), and for an Origin$ Graveyard line, which an effect moving a card out
// of a graveyard to exile fires.
func TestStaticChangesZoneTriggersFireForBounceTuckAndGraveyardExit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, trigger, effect string }{
		{"bounce", "Mode$ ChangesZone | Origin$ Battlefield | Destination$ Hand | ValidCard$ Card.Self | Execute$ TrigDraw | Static$ True",
			"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Hand"},
		{"tuck", "Mode$ ChangesZone | Origin$ Battlefield | Destination$ Library | ValidCard$ Card.Self | Execute$ TrigDraw | Static$ True",
			"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Library | LibraryPosition$ -1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _, host := drawerGame(t, tc.trigger)
			if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(host)}, tc.effect); err != nil {
				t.Fatal(err)
			}
			if err := g.TakePendingError(); err != nil {
				t.Fatal(err)
			}
			// The bounced host is in hand, so the draw is one more; a tucked host
			// is in the library, so the drawn card is the original one.
			want := 1
			if tc.name == "bounce" {
				want = 2
			}
			if got := g.Zone(engine.Hand, p).Len(); got != want {
				t.Errorf("hand = %d, want %d", got, want)
			}
		})
	}

	t.Run("graveyard exit", func(t *testing.T) {
		t.Parallel()
		g, p, other, _ := drawerGame(t, "Mode$ ChangesZone | Origin$ Graveyard | Destination$ Exile | ValidCard$ Creature | Execute$ TrigDraw | Static$ True")
		victim := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Graveyard)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(victim)},
			"DB$ ChangeZone | ValidTgts$ Card | Origin$ Graveyard | Destination$ Exile"); err != nil {
			t.Fatal(err)
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatal(err)
		}
		if got := g.Zone(engine.Hand, p).Len(); got != 1 {
			t.Errorf("hand = %d, want 1", got)
		}
	})
}

// Mode$ TurnBegin fires as each turn is handed over, ValidPlayer$ narrowing it
// to the named player's turns (PhaseHandler.java:522-524, TriggerTurnBegin).
func TestTurnBeginStaticTriggerFiresPerTurn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, trigger string
		want          int
	}{
		{"any player", "Mode$ TurnBegin | Execute$ TrigDraw | Static$ True", 1},
		{"its controller", "Mode$ TurnBegin | ValidPlayer$ You | Execute$ TrigDraw | Static$ True", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _, _ := drawerGame(t, tc.trigger)
			g.SetTurnState(1, p, engine.Cleanup)
			g.AdvancePhase(engine.NewScriptedController()) // the opponent's turn begins
			if err := g.TakePendingError(); err != nil {
				t.Fatal(err)
			}
			if got := g.Zone(engine.Hand, p).Len(); got != tc.want {
				t.Errorf("hand after the opponent's turn began = %d, want %d", got, tc.want)
			}
		})
	}
}

// ThisTurnEnteredFrom_<Zone> (CardProperty.java:954-967): the card entered its
// current zone this turn from the named zone, by a move.
func TestThisTurnEnteredFromProperty(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	spec := valid.Parse("Creature.ThisTurnEnteredFrom_Battlefield")
	placed := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	died := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Move(died, engine.Graveyard, p)
	discarded := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	g.Move(discarded, engine.Graveyard, p)

	if engine.Matches(g, g.Card(placed), spec, p, died) {
		t.Error("a card set up in the graveyard entered from the battlefield this turn")
	}
	if !engine.Matches(g, g.Card(died), spec, p, died) {
		t.Error("a creature that died this turn did not match")
	}
	if engine.Matches(g, g.Card(discarded), spec, p, died) {
		t.Error("a card that came from the hand matched ThisTurnEnteredFrom_Battlefield")
	}
	if !engine.Matches(g, g.Card(discarded), valid.Parse("Creature.ThisTurnEnteredFrom_Hand"), p, died) {
		t.Error("a card that came from the hand did not match ThisTurnEnteredFrom_Hand")
	}
	if engine.Matches(g, g.Card(died), valid.Parse("Creature.ThisTurnEnteredFrom"), p, died) {
		t.Error("a name without the zone matched")
	}
	g.SetTurnState(2, p, engine.Main1)
	if engine.Matches(g, g.Card(died), spec, p, died) {
		t.Error("the entry still counted next turn")
	}
}
