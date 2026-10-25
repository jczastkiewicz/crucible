package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Tectonic Split's RaiseCost Cost$ Sac<X/Land/land(s)> reads the host's SVar X
// (Count$Valid Land.YouCtrl/HalfUp) as the part is paid, not an announced X
// (PlaySpellAbility.announceValuesLikeX asks only for Count$xPaid;
// CostPart.getAbilityAmount): half the lands rounded up are sacrificed.
func TestStaticRaiseCostSacrificeXFromTheHostsSVar(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ lands, sacrificed int }{{5, 3}, {4, 2}} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		var lands []engine.CardID
		for range tc.lands {
			lands = append(lands, g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield))
		}
		split := g.NewCard(corpusCard(t, "Tectonic Split"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 6)
		c := engine.NewScriptedController()
		for range 6 {
			c.QueuePayGeneric(mana.ShardG)
		}
		c.QueueSacrificeChoice(lands[:tc.sacrificed])
		if !g.CastSpell(p, split, c) {
			t.Fatalf("%d lands: CastSpell failed: %v", tc.lands, g.TakePendingError())
		}
		gone := 0
		for _, id := range lands {
			if g.Card(id).Zone == engine.Graveyard {
				gone++
			}
		}
		if gone != tc.sacrificed {
			t.Errorf("%d lands: %d sacrificed, want %d", tc.lands, gone, tc.sacrificed)
		}
	}
}

// Drought's RaiseCost line with ForEachShard$ Black adds "Sacrifice a Swamp" to
// a spell once per black mana symbol in its printed mana cost
// (CostAdjustment.java:118-131); a spell with none pays nothing extra.
func TestStaticRaiseCostForEachShardCountsBlackSymbols(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		spell   string
		color   mana.Colors
		shard   mana.Shard
		paid    int
		swamps  int
		burned  int // Swamps sacrificed
		castsOK bool
	}{
		{"one black symbol", "Dark Ritual", mana.Black, mana.ShardB, 1, 2, 1, true},
		{"two black symbols", "Gray Merchant of Asphodel", mana.Black, mana.ShardB, 5, 3, 2, true},
		{"no black symbol", "Grizzly Bears", mana.Green, mana.ShardG, 2, 2, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, "Drought"), p, engine.Battlefield)
			var swamps []engine.CardID
			for range tc.swamps {
				swamps = append(swamps, g.NewCard(corpusCard(t, "Swamp"), p, engine.Battlefield))
			}
			spell := g.NewCard(corpusCard(t, tc.spell), p, engine.Hand)
			g.Player(p).ManaPool.Add(tc.color, tc.paid)
			c := engine.NewScriptedController()
			for range tc.paid {
				c.QueuePayGeneric(tc.shard)
			}
			if tc.burned > 0 {
				c.QueueSacrificeChoice(swamps[:tc.burned])
			}
			if got := g.CastSpell(p, spell, c); got != tc.castsOK {
				t.Fatalf("CastSpell = %v, want %v (%v)", got, tc.castsOK, g.TakePendingError())
			}
			gone := 0
			for _, id := range swamps {
				if g.Card(id).Zone == engine.Graveyard {
					gone++
				}
			}
			if gone != tc.burned {
				t.Errorf("%d Swamps sacrificed, want %d", gone, tc.burned)
			}
		})
	}
}
