// Package replay reads a replay log recorded from a Java AI-vs-AI game and
// compares that game's events with a Go game's (ADR-0010 L3, ADR-0059,
// ADR-0062, ADR-0063).
//
// The log is JSON Lines written by crucible/oracle-java's GameRecorder: a
// header, then one record per controller call, play, payment, random outcome
// and normalised game event, in Java's order.
package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// FormatVersion is the log version this package reads. Version 1 has no event
// records (ADR-0063), so a diff against one would compare nothing and pass.
const FormatVersion = 2

// Header is a log's first line.
type Header struct {
	Version int    `json:"version"`
	Forge   string `json:"forge"`
	Seed    int64  `json:"seed"`
	Seats   []Seat `json:"seats"`
	// Winner is the winning seat, -1 for a draw.
	Winner int `json:"winner"`
	Turns  int `json:"turns"`
	// StartingLibraries is each seat's library, top first, as its first
	// shuffle left it: Java card ids.
	StartingLibraries    [][]int `json:"startingLibraries"`
	UnresolvedIdentities int     `json:"unresolvedIdentities"`
	UnattributedDraws    int     `json:"unattributedDraws"`
	// Records is how many records follow, so a truncated log is refused.
	Records int `json:"records"`
}

// Seat is one player's deck: every card it owns, by Java id in deck order,
// which ADR-0059 pairs with Go's cards by position.
type Seat struct {
	Seat  int    `json:"seat"`
	Deck  string `json:"deck"`
	Cards []Ref  `json:"cards"`
}

// Record is one line after the header.
type Record struct {
	Seq  int    `json:"seq"`
	Kind string `json:"kind"`
	// Turn and Phase are Java's when the record was written.
	Turn  int    `json:"turn"`
	Phase string `json:"phase"`
	// Event is set on an "event" record.
	Event *Event `json:"event"`
	// Raw is the whole line, for the record kinds the replay controller
	// decodes itself (call, play, payment, shuffle, die, planarDie, coin,
	// draw).
	Raw json.RawMessage `json:"-"`
}

// Log is a decoded replay log.
type Log struct {
	Header  Header
	Records []Record
}

// maxLine bounds one record. The header holds every seat's cards and is the
// longest line, a few kilobytes per seat.
const maxLine = 64 << 20

// Read decodes a log and refuses any version but [FormatVersion], a log
// shorter or longer than its header says, and a record out of sequence.
func Read(r io.Reader) (*Log, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<16), maxLine)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("replay: header: %w", err)
		}
		return nil, errors.New("replay: empty log")
	}
	var log Log
	if err := json.Unmarshal(sc.Bytes(), &log.Header); err != nil {
		return nil, fmt.Errorf("replay: header: %w", err)
	}
	if log.Header.Version != FormatVersion {
		return nil, fmt.Errorf("replay: log version %d, this reader needs %d: re-record it with record-game.sh",
			log.Header.Version, FormatVersion)
	}
	log.Records = make([]Record, 0, log.Header.Records)
	for line := 2; sc.Scan(); line++ {
		raw := bytes.Clone(sc.Bytes())
		var rec Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("replay: line %d: %w", line, err)
		}
		if rec.Seq != len(log.Records) {
			return nil, fmt.Errorf("replay: line %d: record %d out of sequence, want %d", line, rec.Seq, len(log.Records))
		}
		if rec.Kind == "event" && rec.Event == nil {
			return nil, fmt.Errorf("replay: line %d: event record with no event", line)
		}
		rec.Raw = raw
		log.Records = append(log.Records, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("replay: line %d: %w", len(log.Records)+2, err)
	}
	if len(log.Records) != log.Header.Records {
		return nil, fmt.Errorf("replay: %d records, the header says %d", len(log.Records), log.Header.Records)
	}
	return &log, nil
}

// Events is the log's normalised events in Java's order.
func (l *Log) Events() []Event {
	var out []Event
	for _, r := range l.Records {
		if r.Event != nil {
			out = append(out, *r.Event)
		}
	}
	return out
}
