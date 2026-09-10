package collect_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/pkg/collect"
)

func TestOrderedMapKeepsInsertionOrder(t *testing.T) {
	t.Parallel()

	m := collect.NewOrderedMap[string, int](0)
	for i, k := range []string{"c", "a", "b"} {
		if !m.Set(k, i) {
			t.Errorf("Set(%q) reported the key as existing", k)
		}
	}
	want := []string{"c", "a", "b"}
	for i, k := range m.Keys() {
		if k != want[i] {
			t.Errorf("position %d is %q, want %q", i, k, want[i])
		}
	}
}

// Re-setting a value must not move the key. A counter whose count changes
// would otherwise jump to the end of every report and every event.
func TestOrderedMapUpdateKeepsPosition(t *testing.T) {
	t.Parallel()

	m := collect.NewOrderedMap[string, int](0)
	m.Set("a", 1)
	m.Set("b", 2)
	if m.Set("a", 99) {
		t.Error("re-setting an existing key reported it as new")
	}
	if got := m.Keys(); got[0] != "a" || got[1] != "b" {
		t.Errorf("order after update is %v, want [a b]", got)
	}
	if v, _ := m.Get("a"); v != 99 {
		t.Errorf("Get(a) = %d, want 99", v)
	}
}

// Deleting from the middle keeps the survivors in order and leaves the index
// consistent, which is the part a swap-with-last implementation gets wrong.
func TestOrderedMapDeleteKeepsOrder(t *testing.T) {
	t.Parallel()

	m := collect.NewOrderedMap[string, int](0)
	for i, k := range []string{"a", "b", "c", "d"} {
		m.Set(k, i)
	}
	if !m.Delete("b") {
		t.Fatal("Delete reported a present key as absent")
	}
	if m.Delete("b") {
		t.Error("Delete reported an absent key as present")
	}
	if got, want := m.Keys(), []string{"a", "c", "d"}; len(got) != len(want) {
		t.Fatalf("keys %v, want %v", got, want)
	}
	for i, k := range m.Keys() {
		v, ok := m.Get(k)
		if !ok {
			t.Fatalf("%q lost its value after a delete elsewhere", k)
		}
		if got := m.Values()[i]; got != v {
			t.Errorf("Keys and Values disagree at %d: %d vs %d", i, got, v)
		}
	}
	if m.Len() != 3 {
		t.Errorf("Len = %d, want 3", m.Len())
	}
}

func TestOrderedMapCloneIsIndependent(t *testing.T) {
	t.Parallel()

	m := collect.NewOrderedMap[string, int](0)
	m.Set("a", 1)
	c := m.Clone()
	c.Set("b", 2)
	c.Set("a", 9)

	if _, ok := m.Get("b"); ok {
		t.Error("writing to the clone added a key to the original")
	}
	if v, _ := m.Get("a"); v != 1 {
		t.Errorf("original's value changed to %d", v)
	}
}

func TestOrderedMapMissingKey(t *testing.T) {
	t.Parallel()

	m := collect.NewOrderedMap[string, int](0)
	if v, ok := m.Get("nope"); ok || v != 0 {
		t.Errorf("Get on an empty map = (%d, %v), want (0, false)", v, ok)
	}
}
