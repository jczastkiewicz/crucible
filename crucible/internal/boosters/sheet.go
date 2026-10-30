// Print sheets (card/PrintSheet.java): a weighted pool of prints whose entries
// iterate in ConcurrentHashMap order, drawn from by index (ADR-0049).

package boosters

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Sheet is a PrintSheet.
type Sheet struct {
	name    string
	prints  []*Print
	weights []int
	m       chmMap
}

func newSheet(name string) *Sheet { return &Sheet{name: name} }

// add is PrintSheet.add(card, weight): ItemPool.add is ConcurrentHashMap.merge
// with a sum. A nil print or a weight of 0 or less adds nothing.
func (s *Sheet) add(p *Print, weight int) error {
	if p == nil || weight <= 0 {
		return nil
	}
	idx := len(s.prints)
	found := -1
	err := s.m.merge(p.hash, idx, func(other int) bool {
		if s.prints[other].equal(p) {
			found = other
			return true
		}
		return false
	})
	if err != nil {
		return fmt.Errorf("sheet %q: %w", s.name, err)
	}
	if found >= 0 {
		s.weights[found] += weight
		return nil
	}
	s.prints = append(s.prints, p)
	s.weights = append(s.weights, weight)
	return nil
}

// entries is the indexes of the sheet's prints in iteration order.
func (s *Sheet) entries() []int { return s.m.order() }

// flat is PrintSheet.toFlatList: each print as many times as its weight, in
// iteration order.
func (s *Sheet) flat() []*Print {
	var out []*Print
	for _, i := range s.entries() {
		for range s.weights[i] {
			out = append(out, s.prints[i])
		}
	}
	return out
}

func (s *Sheet) countAll() int {
	n := 0
	for _, w := range s.weights {
		n += w
	}
	return n
}

func (s *Sheet) distinct() int { return len(s.prints) }

// random is PrintSheet.random(n, wantUnique) (PrintSheet.java:116-141): while
// n reaches the distinct count every print comes out once with no draw;
// the rest are weighted draws, each skipping the prints already picked when
// wantUnique, by fetchRoulette's walk (not a re-weighted draw).
func (s *Sheet) random(r *javarand.Rand, n int, wantUnique bool) ([]*Print, error) {
	total := s.countAll()
	if total == 0 {
		return nil, nil
	}
	order := s.entries()
	var out []*Print
	u := s.distinct()
	for n >= u {
		for _, i := range order {
			out = append(out, s.prints[i])
		}
		n -= u
	}
	picked := map[int]bool{}
	for range n {
		idx := int(r.Int32n(int32(total)))
		i, err := s.fetchRoulette(order, 0, idx, picked, wantUnique)
		if err != nil {
			return nil, err
		}
		out = append(out, s.prints[i])
		if wantUnique {
			picked[i] = true
		}
	}
	return out, nil
}

// fetchRoulette is PrintSheet.fetchRoulette (:95-110): the first entry whose
// running weight passes roulette and which is not skipped; none that does
// restarts once from the sum plus one, which takes the first unskipped entry.
func (s *Sheet) fetchRoulette(order []int, start, roulette int, skip map[int]bool, unique bool) (int, error) {
	sum := start
	for _, i := range order {
		sum += s.weights[i]
		if sum > roulette {
			if unique && skip[i] {
				continue
			}
			return i, nil
		}
	}
	if start > 0 {
		return 0, fmt.Errorf("sheet %q: roulette ran out of cards", s.name)
	}
	return s.fetchRoulette(order, sum+1, roulette, skip, unique)
}
