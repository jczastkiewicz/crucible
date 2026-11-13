package replay

import "github.com/jczastkiewicz/crucible/internal/engine"

// Cards is what the normalizer reads about a Go card when its event is
// emitted. The sink runs inside the game's goroutine (ADR-0013), so a card's
// state is the one the event happened in: a planeswalker that was dealt
// damage is still one, a token that died is still a token.
type Cards interface {
	// JavaID is the Java id the replay paired card with (ADR-0059), false
	// for a card it has not paired yet.
	JavaID(card engine.CardID) (int, bool)
	Name(card engine.CardID) string
	IsToken(card engine.CardID) bool
	IsCreature(card engine.CardID) bool
	// CounterName is the CounterType a CounterChanged Detail names
	// (Game.CounterTypeOf).
	CounterName(detail engine.CounterDetail) string
}

// Normalizer is an [engine.Sink] that maps a Go game's events to ADR-0063's
// normalised events.
type Normalizer struct {
	cards  Cards
	events []Event
}

// NewNormalizer maps events through cards.
func NewNormalizer(cards Cards) *Normalizer { return &Normalizer{cards: cards} }

// Events is what the normalizer kept, in emit order.
func (n *Normalizer) Events() []Event { return n.events }

// Emit maps e and keeps it, or drops it when ADR-0063 compares nothing for
// it.
func (n *Normalizer) Emit(e engine.Event) {
	var out Event
	switch e.Kind {
	case engine.TurnBegan:
		out = Event{Type: KindTurn, Turn: int(e.Turn), Active: seat(e.Active)}
	case engine.PhaseBegan:
		out = Event{Type: KindStep, Step: stepName(e.Phase), Active: seat(e.Active)}
	case engine.AbilityActivated:
		// A spell's SpellCast beside it is the same push.
		out = Event{Type: KindPush, Card: n.card(e.Source), Controller: seat(e.Actor)}
	case engine.AbilityResolved:
		out = Event{Type: KindResolve, Card: n.card(e.Source)}
	case engine.GameEnded:
		out = Event{Type: KindEnd, Winner: seat(e.Actor)}
	case engine.ZoneChanged:
		// Java fires no event for a token leaving the battlefield
		// (GameAction.java:124-127).
		if e.From == engine.Battlefield && n.cards.IsToken(e.Source) {
			return
		}
		out = Event{Type: KindMove, Card: n.card(e.Source), From: e.From.String(), To: e.To.String()}
	case engine.DamageDealt:
		target, ok := n.entity(e.Target)
		if !ok {
			return
		}
		if id, isCard := e.Target.AsCard(); isCard && !n.cards.IsCreature(id) {
			// Java's damage to a planeswalker or a battle is its counter
			// change alone (Card.java:6383-6399); Go emits that too.
			return
		}
		out = Event{Type: KindDamage, Source: n.card(e.Source), Target: target, Amount: int(e.Amount)}
	case engine.LifeChanged:
		p, ok := e.Target.AsPlayer()
		if !ok {
			return
		}
		out = Event{Type: KindLife, Player: seat(p), Delta: int(e.Amount)}
	case engine.CounterChanged:
		target, ok := n.entity(e.Target)
		if !ok {
			return
		}
		out = Event{Type: KindCounter, Target: target, Counter: n.cards.CounterName(engine.CounterDetail(e.Detail)),
			Delta: int(e.Amount)}
	default:
		// SpellCast (its AbilityActivated is the push), CardDrawn (its
		// ZoneChanged is the move), ManaLost (Java's event has no amount),
		// Phased (not yet checked against a game that phases).
		return
	}
	n.events = append(n.events, out)
}

func (n *Normalizer) card(id engine.CardID) Ref {
	if id == engine.NoCard {
		return noRef()
	}
	if j, ok := n.cards.JavaID(id); ok {
		return CardRef(j, n.cards.Name(id))
	}
	return Ref{Seat: -1, GoCard: id, Name: n.cards.Name(id)}
}

func (n *Normalizer) entity(e engine.EntityID) (Ref, bool) {
	if id, ok := e.AsCard(); ok {
		return n.card(id), true
	}
	if p, ok := e.AsPlayer(); ok {
		return SeatRef(seat(p)), true
	}
	return noRef(), false
}

// seat is a Go player's seat: players are numbered from 1 in seat order
// (NewGame), and NoPlayer is no seat.
func seat(p engine.PlayerID) int {
	if p == engine.NoPlayer {
		return -1
	}
	return int(p) - 1
}

// stepName is Java's PhaseType constant name for p, the name the recorder
// logs (PhaseType.java).
func stepName(p engine.PhaseType) string {
	switch p {
	case engine.Untap:
		return "UNTAP"
	case engine.Upkeep:
		return "UPKEEP"
	case engine.Draw:
		return "DRAW"
	case engine.Main1:
		return "MAIN1"
	case engine.CombatBegin:
		return "COMBAT_BEGIN"
	case engine.DeclareAttackers:
		return "COMBAT_DECLARE_ATTACKERS"
	case engine.DeclareBlockers:
		return "COMBAT_DECLARE_BLOCKERS"
	case engine.FirstStrikeDamage:
		return "COMBAT_FIRST_STRIKE_DAMAGE"
	case engine.CombatDamage:
		return "COMBAT_DAMAGE"
	case engine.CombatEnd:
		return "COMBAT_END"
	case engine.Main2:
		return "MAIN2"
	case engine.EndOfTurn:
		return "END_OF_TURN"
	default:
		return "CLEANUP"
	}
}
