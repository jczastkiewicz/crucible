// Canonical text form of a compiled card, for the golden AST diff.

package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
)

// WriteCanonical writes a card's compiled AST as deterministic text.
//
// This is the subject of M3's golden diff. The compiler is rewritten
// constantly during a port, and most of those rewrites are meant to change
// nothing; a golden makes "nothing" checkable, and turns a silent change
// across 33,689 cards into a diff someone has to accept.
//
// The format is line-oriented and indentation-nested so the diff is readable:
//
//	face 0
//	  Spell Draw
//	    param SP$ "Draw"
//	    param NumCards$ "3"
//	    sub SubAbility DBCleanup
//	      SubAbility Cleanup svar=DBCleanup
//	        param ClearRemembered$ "True"
//
// Values are quoted with strconv.Quote, so a value holding a tab, a quote or a
// non-printing byte cannot shift a column or forge a line.
//
// Faces with no abilities are written as `face N empty` rather than skipped. A
// face that silently stops being compiled is exactly what this is here to
// catch, and omitting empty faces would hide it.
func WriteCanonical(w io.Writer, c *Card) error {
	_, err := w.Write(appendCanonical(nil, c))
	return err
}

// Fingerprint is the SHA-256 of a card's canonical form, hex-encoded.
//
// The corpus golden is one line per card rather than the text of 33,689 ASTs,
// which would be neither reviewable nor reasonable to commit. A hash keeps the
// locality that matters -- the diff names the cards that changed -- and the
// pinned shapes golden carries readable text for the forms worth reading.
func Fingerprint(c *Card) string {
	sum := sha256.Sum256(appendCanonical(nil, c))
	return hex.EncodeToString(sum[:])
}

// appendCanonical builds the text with append rather than a bufio.Writer, so
// there is no per-call error to ignore 20 times over.
func appendCanonical(dst []byte, c *Card) []byte {
	for i := range c.Faces {
		face := &c.Faces[i]
		groups := [][]*Ability{face.Abilities, face.Triggers, face.Statics, face.Replacements}

		empty := true
		for _, list := range groups {
			if len(list) > 0 {
				empty = false
				break
			}
		}
		dst = append(dst, "face "...)
		dst = strconv.AppendInt(dst, int64(i), 10)
		if empty {
			dst = append(dst, " empty\n"...)
			continue
		}
		dst = append(dst, '\n')
		for _, list := range groups {
			for _, a := range list {
				dst = appendAbility(dst, a, 1)
			}
		}
	}
	return dst
}

// appendAbility writes one ability and everything it names, depth-first in the
// script's own order. Compile rejects cycles, so the walk terminates; an
// ability named twice is written twice, because the two references are two
// facts about the card and collapsing them would hide one of them going away.
func appendAbility(dst []byte, a *Ability, depth int) []byte {
	indent := func(dst []byte, n int) []byte {
		for i := 0; i < n; i++ {
			dst = append(dst, ' ', ' ')
		}
		return dst
	}

	dst = indent(dst, depth)
	dst = append(dst, a.Record.String()...)
	dst = append(dst, ' ')
	dst = append(dst, a.Name...)
	if a.SVar != "" {
		dst = append(dst, " svar="...)
		dst = append(dst, a.SVar...)
	}
	dst = append(dst, '\n')

	for _, p := range a.Params {
		dst = indent(dst, depth+1)
		dst = append(dst, "param "...)
		dst = append(dst, p.Key...)
		dst = append(dst, "$ "...)
		dst = strconv.AppendQuote(dst, p.Value)
		dst = append(dst, '\n')
	}
	for _, s := range a.Subs {
		dst = indent(dst, depth+1)
		dst = append(dst, "sub "...)
		dst = append(dst, s.Key...)
		dst = append(dst, ' ')
		dst = append(dst, s.SVar...)
		dst = append(dst, '\n')
		dst = appendAbility(dst, s.Ability, depth+2)
	}
	return dst
}
