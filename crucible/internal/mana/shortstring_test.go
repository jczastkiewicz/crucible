package mana_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ShortString is ManaCost.getShortString: generic first, every shard after a
// space with its braces, "0" for a zero cost, "-1" for no cost.
func TestShortString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cost mana.Cost
		want string
	}{
		{"no cost", mana.NoCost(), "-1"},
		{"zero cost", mana.GenericCost(0), "0"},
		{"generic only", mana.GenericCost(3), "3"},
		{"generic and colors", mana.MustParse("2 W W"), "2 {W} {W}"},
		{"colors only", mana.MustParse("R"), "{R}"},
		{"hybrid keeps its slash", mana.MustParse("1 BG"), "1 {B/G}"},
		{"x is a shard", mana.MustParse("X R"), "{X} {R}"},
		{"reduced below zero shows the reduction", mana.FromShards([]mana.Shard{mana.ShardX}, -2), "{X} -2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cost.ShortString(); got != tt.want {
				t.Errorf("ShortString() = %q, want %q", got, tt.want)
			}
		})
	}
}
