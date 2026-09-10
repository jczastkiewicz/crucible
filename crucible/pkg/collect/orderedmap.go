package collect

// OrderedMap is a map that remembers insertion order.
//
// Go's map iteration order is deliberately random, which is the right default
// and the wrong one here: the engine's iteration order is load-bearing. Trigger
// ordering, counter reporting and event streams all have to come out the same
// way on every run, or a replay stops matching and a differential diff stops
// meaning anything (GO-12).
//
// Insertion order, not sorted order, because that is what Java's
// LinkedHashMap and FCollection give and parity is measured against them.
type OrderedMap[K comparable, V any] struct {
	index map[K]int
	keys  []K
	vals  []V
}

// NewOrderedMap returns an empty map with room for capacity entries.
func NewOrderedMap[K comparable, V any](capacity int) *OrderedMap[K, V] {
	return &OrderedMap[K, V]{index: make(map[K]int, capacity)}
}

// Set stores a value, and reports whether the key is new. An existing key
// keeps its position: re-setting a value is not a reinsertion, because a
// counter changing count must not jump to the end of the report.
func (m *OrderedMap[K, V]) Set(k K, v V) bool {
	if i, ok := m.index[k]; ok {
		m.vals[i] = v
		return false
	}
	m.index[k] = len(m.keys)
	m.keys = append(m.keys, k)
	m.vals = append(m.vals, v)
	return true
}

// Get returns a value and whether the key is present.
func (m *OrderedMap[K, V]) Get(k K) (V, bool) {
	if i, ok := m.index[k]; ok {
		return m.vals[i], true
	}
	var zero V
	return zero, false
}

// Delete removes a key and reports whether it was there.
//
// Order among the survivors is preserved, which costs a shift. The alternative
// -- swapping the last entry into the hole -- is O(1) and reorders, and this
// type exists because order matters.
func (m *OrderedMap[K, V]) Delete(k K) bool {
	i, ok := m.index[k]
	if !ok {
		return false
	}
	m.keys = append(m.keys[:i], m.keys[i+1:]...)
	m.vals = append(m.vals[:i], m.vals[i+1:]...)
	delete(m.index, k)
	for j := i; j < len(m.keys); j++ {
		m.index[m.keys[j]] = j
	}
	return true
}

// Len is how many entries the map holds.
func (m *OrderedMap[K, V]) Len() int { return len(m.keys) }

// Keys returns the keys in insertion order. The slice is the map's own.
func (m *OrderedMap[K, V]) Keys() []K { return m.keys }

// Values returns the values in the same order as [OrderedMap.Keys].
func (m *OrderedMap[K, V]) Values() []V { return m.vals }

// Clone returns an independent copy. Values are copied as they are, so a map
// of pointers clones the pointers and not what they point at.
func (m *OrderedMap[K, V]) Clone() *OrderedMap[K, V] {
	out := &OrderedMap[K, V]{
		index: make(map[K]int, len(m.index)),
		keys:  append([]K(nil), m.keys...),
		vals:  append([]V(nil), m.vals...),
	}
	for k, i := range m.index {
		out.index[k] = i
	}
	return out
}
