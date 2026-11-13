package replay

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Kind is a normalised event's type, ADR-0063's table.
type Kind string

// The kinds. The first five are anchors, compared in order; the rest happen
// between two anchors and are compared as a multiset or a net change.
const (
	KindTurn    Kind = "turn"
	KindStep    Kind = "step"
	KindPush    Kind = "push"
	KindResolve Kind = "resolve"
	KindEnd     Kind = "end"
	KindMove    Kind = "move"
	KindDamage  Kind = "damage"
	KindLife    Kind = "life"
	KindCounter Kind = "counter"
)

// Anchor reports whether events of kind k are compared in order.
func (k Kind) Anchor() bool {
	switch k {
	case KindTurn, KindStep, KindPush, KindResolve, KindEnd:
		return true
	default:
		return false
	}
}

// net reports whether events of kind k are summed per entity within an
// interval: Java batches a damage step's life loss into one change and Go
// does not (ADR-0063).
func (k Kind) net() bool { return k == KindLife || k == KindCounter }

// Event is one normalised event. Which fields a kind uses is ADR-0063's
// table; the rest are zero. The JSON names are the Java recorder's.
type Event struct {
	Type Kind `json:"type"`
	// Turn is the turn number of a turn event.
	Turn int `json:"turn"`
	// Active is the active seat of a turn or step event.
	Active int `json:"active"`
	// Step is Java's PhaseType constant name ("COMBAT_DAMAGE").
	Step string `json:"step"`
	// Card is the host of a push or resolve, the card a move moves.
	Card Ref `json:"card"`
	// Controller is the seat that put a push's ability on the stack.
	Controller int `json:"controller"`
	// Ability is a push's ADR-0059 identity, Java only and never compared:
	// Go's event names the host alone.
	Ability json.RawMessage `json:"ability"`
	// Winner is an end event's winning seat, -1 for a draw.
	Winner int `json:"winner"`
	// From and To are a move's zones, Java's ZoneType names; a token's
	// creation comes from "None".
	From string `json:"from"`
	To   string `json:"to"`
	// Source and Target are a damage event's; Target is also a counter
	// event's.
	Source Ref `json:"source"`
	Target Ref `json:"target"`
	Amount int `json:"amount"`
	// Player is a life event's seat.
	Player int `json:"player"`
	// Counter is a counter event's type: Go's CounterType, Java's
	// CounterEnumType constant name.
	Counter string `json:"counter"`
	// Delta is a life or counter event's change.
	Delta int `json:"delta"`
}

// key identifies what is compared: two events with the same key are the same
// event, and for a net kind the same entity whose deltas are summed. Names
// are left out; they are for the report.
func (e Event) key() string { return e.text(false) }

// String is the event for a report, card names included.
func (e Event) String() string {
	s := e.text(true)
	if e.Type.net() {
		s += " " + strconv.Itoa(e.Delta)
	}
	return s
}

func (e Event) text(names bool) string {
	ref := func(r Ref) string {
		if names {
			return r.String()
		}
		return r.key()
	}
	switch e.Type {
	case KindTurn:
		return fmt.Sprintf("turn %d, seat %d active", e.Turn, e.Active)
	case KindStep:
		return fmt.Sprintf("step %s, seat %d active", e.Step, e.Active)
	case KindPush:
		return fmt.Sprintf("push %s by seat %d", ref(e.Card), e.Controller)
	case KindResolve:
		return "resolve " + ref(e.Card)
	case KindEnd:
		if e.Winner < 0 {
			return "end, draw"
		}
		return fmt.Sprintf("end, seat %d wins", e.Winner)
	case KindMove:
		return fmt.Sprintf("move %s %s -> %s", ref(e.Card), e.From, e.To)
	case KindDamage:
		return fmt.Sprintf("damage %s -> %s %d", ref(e.Source), ref(e.Target), e.Amount)
	case KindLife:
		return fmt.Sprintf("life seat %d", e.Player)
	case KindCounter:
		return fmt.Sprintf("counter %s %s", ref(e.Target), e.Counter)
	default:
		return "unknown event " + strconv.Quote(string(e.Type))
	}
}

// Ref is a card or a seat. A Java log's cards are Java ids; a Go card with no
// Java id yet is kept by its Go id so a report can name it. A Ref with no
// card and seat -1 is no entity (a damage event with no source card).
type Ref struct {
	// Card is the Java card id, 0 when the ref is not a Java card.
	Card int
	// Seat is the seat, -1 when the ref is not a player.
	Seat int
	// GoCard is a Go card the replay has no Java id for, 0 otherwise.
	GoCard engine.CardID
	// Name is the card's name. Never compared.
	Name string
}

// CardRef is a Java card.
func CardRef(id int, name string) Ref { return Ref{Card: id, Seat: -1, Name: name} }

// SeatRef is a player.
func SeatRef(seat int) Ref { return Ref{Seat: seat} }

func noRef() Ref { return Ref{Seat: -1} }

// UnmarshalJSON reads the recorder's {"card":id,"name":..} or {"seat":n}, and
// null as no entity.
func (r *Ref) UnmarshalJSON(b []byte) error {
	var v *struct {
		Card *int   `json:"card"`
		Seat *int   `json:"seat"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = noRef()
	switch {
	case v == nil:
	case v.Card != nil:
		*r = CardRef(*v.Card, v.Name)
	case v.Seat != nil:
		*r = SeatRef(*v.Seat)
	default:
		return fmt.Errorf("replay: entity %s is neither a card nor a seat", b)
	}
	return nil
}

func (r Ref) key() string {
	switch {
	case r.Card != 0:
		return "card " + strconv.Itoa(r.Card)
	case r.GoCard != engine.NoCard:
		return fmt.Sprintf("unmapped Go card %d", r.GoCard)
	case r.Seat >= 0:
		return "seat " + strconv.Itoa(r.Seat)
	default:
		return "none"
	}
}

// String is the ref with its card's name.
func (r Ref) String() string {
	if r.Name == "" {
		return r.key()
	}
	return r.key() + " (" + r.Name + ")"
}
