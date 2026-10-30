// Iteration order of the Java collections whose order Forge's booster code
// depends on, reproduced so a pack built here draws the same card as Java for
// the same seed (ADR-0049). The draw is an index into a sheet, and a sheet is
// a java.util.concurrent.ConcurrentHashMap, so the order of its entries is
// part of the contract.
//
// Ported from the JDK 17 sources of String.hashCode, HashMap and
// ConcurrentHashMap (putVal/merge, addCount, tryPresize and transfer, single
// threaded: a sheet is built by one goroutine at load).

package boosters

import "fmt"

// stringHash is String.hashCode over the UTF-16 code units.
func stringHash(s string) int32 {
	var h int32
	for _, r := range s {
		if r >= 0x10000 {
			r -= 0x10000
			h = 31*h + (0xD800 + (r >> 10))
			h = 31*h + (0xDC00 + (r & 0x3ff))
			continue
		}
		h = 31*h + r
	}
	return h
}

// hashMapOrder is the iteration order of Guava's ArrayListMultimap key map (a
// HashMap created with expected size 12, whose table starts at 32) holding keys,
// inserted in this order and never removed: bucket by bucket in the final
// table, insertion order inside a bucket (a resize split keeps relative
// order, and an insertion appends to its bucket's tail).
func hashMapOrder(keys []string) []int { return hashMapOrderFrom(keys, 32) }

// hashMapOrderFrom is hashMapOrder for a HashMap whose table starts at capacity.
func hashMapOrderFrom(keys []string, capacity int) []int {
	for len(keys) > capacity*3/4 {
		capacity *= 2
	}
	buckets := make([][]int, capacity)
	for i, k := range keys {
		h := uint32(stringHash(k))
		h ^= h >> 16
		b := int(h) & (capacity - 1)
		buckets[b] = append(buckets[b], i)
	}
	out := make([]int, 0, len(keys))
	for _, b := range buckets {
		out = append(out, b...)
	}
	return out
}

const (
	chmTreeifyThreshold   = 8
	chmMinTreeifyCapacity = 64
	chmMaximumCapacity    = 1 << 30
)

// chmNode is a bin entry of the emulated ConcurrentHashMap.
type chmNode struct {
	hash int32
	key  int // index into the caller's key list
	next *chmNode
}

// chmMap is ConcurrentHashMap's table, enough of it to put keys in and read
// them back in iteration order. Keys are caller indexes with a caller hash;
// equal keys are the caller's business (merge reports whether it found one).
type chmMap struct {
	table     []*chmNode
	sizeCtl   int
	baseCount int
}

func chmSpread(h int32) int32 {
	u := uint32(h)
	return int32((u ^ (u >> 16)) & 0x7fffffff)
}

func (m *chmMap) initTable() {
	n := 16
	if m.sizeCtl > 0 {
		n = m.sizeCtl
	}
	m.table = make([]*chmNode, n)
	m.sizeCtl = n - (n >> 2)
}

// merge is ConcurrentHashMap.merge's placement: key with this hash, equal to
// an earlier key when same reports it. It returns an error where Java would
// treeify a bin of a table of 64 or more, an order this port does not model.
func (m *chmMap) merge(hash int32, key int, same func(other int) bool) error {
	h := chmSpread(hash)
	for {
		if m.table == nil {
			m.initTable()
			continue
		}
		n := len(m.table)
		i := int(h) & (n - 1)
		f := m.table[i]
		if f == nil {
			m.table[i] = &chmNode{hash: h, key: key}
			m.addCount(0)
			return nil
		}
		binCount := 1
		var pred *chmNode
		delta := 0
		for e := f; ; binCount++ {
			if e.hash == h && same(e.key) {
				break
			}
			pred = e
			if e = e.next; e == nil {
				delta = 1
				pred.next = &chmNode{hash: h, key: key}
				break
			}
		}
		if binCount >= chmTreeifyThreshold {
			if n < chmMinTreeifyCapacity {
				m.tryPresize(n << 1)
			} else {
				return fmt.Errorf("%w: a bin of %d entries would be treeified", ErrNotModelled, binCount)
			}
		}
		if delta != 0 {
			m.addCount(binCount)
		}
		return nil
	}
}

// addCount is ConcurrentHashMap.addCount after the base count took the
// element: grow while the count has reached sizeCtl.
func (m *chmMap) addCount(check int) {
	m.baseCount++
	if check < 0 {
		return
	}
	for m.baseCount >= m.sizeCtl && m.table != nil && len(m.table) < chmMaximumCapacity {
		m.transfer()
	}
}

func tableSizeFor(c int) int {
	n := 1
	for n < c {
		n <<= 1
	}
	return n
}

// tryPresize is ConcurrentHashMap.tryPresize for a table that exists.
func (m *chmMap) tryPresize(size int) {
	c := tableSizeFor(size + (size >> 1) + 1)
	for m.sizeCtl >= 0 {
		n := len(m.table)
		if n == 0 {
			m.initTable()
			continue
		}
		if c <= m.sizeCtl || n >= chmMaximumCapacity {
			return
		}
		m.transfer()
	}
}

// transfer doubles the table, splitting each bin into a low and a high one
// the way ConcurrentHashMap.transfer does: the run of nodes at the tail that
// share a destination is reused as it is, and every node before it is put in
// front of its list, which reverses their order.
func (m *chmMap) transfer() {
	n := len(m.table)
	next := make([]*chmNode, n<<1)
	for i := 0; i < n; i++ {
		f := m.table[i]
		if f == nil {
			continue
		}
		runBit := f.hash & int32(n)
		lastRun := f
		for p := f.next; p != nil; p = p.next {
			if b := p.hash & int32(n); b != runBit {
				runBit = b
				lastRun = p
			}
		}
		var ln, hn *chmNode
		if runBit == 0 {
			ln = lastRun
		} else {
			hn = lastRun
		}
		for p := f; p != lastRun; p = p.next {
			c := &chmNode{hash: p.hash, key: p.key}
			if p.hash&int32(n) == 0 {
				c.next = ln
				ln = c
			} else {
				c.next = hn
				hn = c
			}
		}
		next[i] = ln
		next[i+n] = hn
	}
	m.table = next
	m.sizeCtl = (n << 1) - (n >> 1)
}

// order is the keys in iteration order: table index ascending, each bin from
// its first node.
func (m *chmMap) order() []int {
	var out []int
	for _, f := range m.table {
		for e := f; e != nil; e = e.next {
			out = append(out, e.key)
		}
	}
	return out
}
