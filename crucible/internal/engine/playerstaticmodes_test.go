package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ WitherDamage (Everlasting Torment, CR 120.3d): all damage is dealt as
// though its source had wither, so a creature dealt 2 gets two -1/-1 counters
// and no damage.
func TestWitherDamageStaticTurnsDamageIntoCounters(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Everlasting Torment"), other, engine.Battlefield)
	giant := g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(giant)},
		"DB$ DealDamage | ValidTgts$ Creature | NumDmg$ 2"); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(giant).Counters.Count(engine.M1M1); got != 2 {
		t.Errorf("-1/-1 counters %d, want 2", got)
	}
	if got := g.Card(giant).Damage.Marked; got != 0 {
		t.Errorf("damage %d, want 0", got)
	}
}

// Mode$ InfectDamage (Phyrexian Unlife, CR 120.3b): at 0 or less life, damage
// to its controller is poison counters; above 0 it is life loss.
func TestInfectDamageStaticAtZeroLife(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		life, wantLife, wantPoison int
	}{
		{0, 0, 2},
		{5, 3, 0},
	} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, other, engine.Main1)
		g.NewCard(corpusCard(t, "Phyrexian Unlife"), p, engine.Battlefield)
		g.Player(p).Life = tc.life
		c := engine.NewScriptedController()
		if _, err := resolveNow(t, g, other, c, nil, "DB$ DealDamage | Defined$ Opponent | NumDmg$ 2"); err != nil {
			t.Fatal(err)
		}
		if got := g.Player(p).Life; got != tc.wantLife {
			t.Errorf("life %d: life after %d, want %d", tc.life, got, tc.wantLife)
		}
		if got := g.Player(p).Counters.Count(engine.Poison); got != tc.wantPoison {
			t.Errorf("life %d: poison %d, want %d", tc.life, got, tc.wantPoison)
		}
	}
}

// surveilRecorder records how many cards each surveil looks at.
type surveilRecorder struct {
	*engine.ScriptedController
	seen []int
}

func (r *surveilRecorder) ArrangeForSurveil(_ *engine.Game, _ engine.PlayerID, cards []engine.CardID) (toTop, toGraveyard []engine.CardID) {
	r.seen = append(r.seen, len(cards))
	return cards, nil
}

// Mode$ SurveilNum (Enhanced Surveillance): its controller may look at two
// more cards each time they surveil; declining keeps the printed number.
func TestSurveilNumAddsCardsWhenAccepted(t *testing.T) {
	t.Parallel()
	for _, accept := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, "Enhanced Surveillance"), p, engine.Battlefield)
		libraryCards(t, g, p, 5)
		c := &surveilRecorder{ScriptedController: engine.NewScriptedController()}
		c.QueueConfirmEffect(accept)
		def := etbChainDef(t, "Test Now", "DB$ Surveil | Amount$ 1")
		host := g.NewCard(def, p, engine.Battlefield)
		g.PushAbility(engine.Ability{API: engine.APISurveil, Source: host, Controller: p, Params: def.Faces[0].Triggers[0].Subs[0].Ability})
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		want := 1
		if accept {
			want = 3
		}
		if len(c.seen) != 1 || c.seen[0] != want {
			t.Errorf("accept %v: surveil looked at %v, want [%d]", accept, c.seen, want)
		}
	}
}

// Mode$ CantDiscard (Tamiyo, Collector of Tales): an opponent's effect cannot
// make its controller discard, their own effect still can.
func TestCantDiscardStopsOpponentsEffect(t *testing.T) {
	t.Parallel()
	for _, own := range []bool{false, true} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.NewCard(corpusCard(t, "Tamiyo, Collector of Tales"), p, engine.Battlefield)
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
		caster, defined := other, "Opponent"
		if own {
			caster, defined = p, "You"
		}
		g.SetTurnState(1, caster, engine.Main1)
		c := engine.NewScriptedController()
		if _, err := resolveNow(t, g, caster, c, nil, "DB$ Discard | Defined$ "+defined+" | Mode$ Hand"); err != nil {
			t.Fatal(err)
		}
		want := 1
		if own {
			want = 0
		}
		if got := len(g.Zone(engine.Hand, p).Cards()); got != want {
			t.Errorf("own %v: hand %d, want %d", own, got, want)
		}
	}
}
