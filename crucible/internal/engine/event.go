// The event stream: what the engine tells the recorder.

package engine

// SchemaVersion is the version of the event record's shape.
//
// Stamped on every row and every shard, and separate from the metrics version:
// the schema is what was emitted, the metrics version is how it was
// interpreted, and the two move independently. Adding a kind or a field
// increments this; changing what a metric means does not (ADR-0013, MET-1).
//
// Tooling refuses to merge shards with differing versions, because a reader
// that silently unions two schemas produces a column meaning one thing for
// half its rows.
const SchemaVersion = 1

// EventKind is what happened.
type EventKind uint8

// The kinds. Appending is a schema change; reordering is a breaking one,
// because the number is what a shard stores.
const (
	// EventNone is the zero value and is never emitted. It exists so a
	// zero-valued Event is obviously not a real one.
	EventNone EventKind = iota
	TurnBegan
	PhaseBegan
	ZoneChanged
	SpellCast
	AbilityActivated
	AbilityResolved
	DamageDealt
	LifeChanged
	CardDrawn
	CounterChanged
	GameEnded

	numEventKinds = int(GameEnded) + 1
)

var eventKindNames = [numEventKinds]string{
	EventNone: "None", TurnBegan: "TurnBegan", PhaseBegan: "PhaseBegan",
	ZoneChanged: "ZoneChanged", SpellCast: "SpellCast",
	AbilityActivated: "AbilityActivated", AbilityResolved: "AbilityResolved",
	DamageDealt: "DamageDealt", LifeChanged: "LifeChanged", CardDrawn: "CardDrawn",
	CounterChanged: "CounterChanged", GameEnded: "GameEnded",
}

// String returns the kind's name, for reports and debugging.
func (k EventKind) String() string {
	if int(k) >= numEventKinds {
		return "None"
	}
	return eventKindNames[k]
}

// EventFlags are the booleans an event carries, packed so the record stays one
// fixed-size value.
type EventFlags uint16

// The flags.
const (
	// FlagDeathtouch marks damage dealt by a deathtouch source.
	FlagDeathtouch EventFlags = 1 << iota
	// FlagCombat marks damage dealt in combat rather than by an effect.
	FlagCombat
	// FlagOptional marks an action the player could have declined.
	FlagOptional
	// FlagReplaced marks an event that a replacement effect produced in place
	// of another.
	FlagReplaced
)

// Has reports whether every flag in mask is set.
func (f EventFlags) Has(mask EventFlags) bool { return f&mask == mask }

// Event is one thing that happened, as a fixed-size record.
//
// Flat and fixed-size on purpose: at 10^5 games the stream is the largest
// thing the runner produces, and a struct with a pointer in it would be an
// allocation per event and a pointer chase per read. `Detail` carries the
// kind-specific payload as a number rather than a field per kind.
type Event struct {
	Kind   EventKind
	Phase  PhaseType
	Active PlayerID
	Actor  PlayerID
	Turn   uint16
	Flags  EventFlags
	// Source is the card the event came from. ADR-0013 sketches this field as
	// `Card`; it is `Source` here for the same reason EntityID's accessors are
	// AsCard and AsPlayer -- a field named Card in a package that declares a
	// Card type reads as one, and it matches Ability.Source.
	Source CardID
	Target EntityID
	From   ZoneType
	To     ZoneType
	Amount int32
	// Detail is a kind-specific enum payload: the counter type for
	// CounterChanged, the reason for GameEnded. Its meaning is the kind's.
	Detail uint32
}

// Sink receives events.
//
// The recorder folds synchronously inside the game's goroutine. A game already
// owns its state exclusively, so its recorder can too, and folding in place
// means there is no queue to fill, no drop policy to get wrong and no
// scheduling input to the output (ADR-0005, ADR-0013).
type Sink interface {
	Emit(Event)
}

// DiscardSink drops every event.
//
// This is what a cloned game gets. The AI explores lines that never happened,
// and a clone holding the real game's sink would record imagined casts as
// real.
type DiscardSink struct{}

// Emit does nothing.
func (DiscardSink) Emit(Event) {}
