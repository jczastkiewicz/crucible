package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// attackWith declares attackers for the active player, failing the test if
// the declaration is rejected.
func declareLegalAttack(t *testing.T, g *engine.Game, attackers ...engine.CardID) {
	t.Helper()
	if _, err := declareAttack(g, attackers...); err != nil {
		t.Fatalf("declare attackers %v: %v", attackers, err)
	}
}

// declareBlocks declares blocks and returns the result and the error the
// declaration left, if any.
func declareBlocks(g *engine.Game, blocks ...engine.Block) ([]engine.Block, error) {
	c := engine.NewScriptedController()
	c.QueueBlocks(blocks)
	return g.DeclareCombatBlockers(c)
}

func wantBlockError(t *testing.T, g *engine.Game, rule string, blocks ...engine.Block) {
	t.Helper()
	got, err := declareBlocks(g, blocks...)
	if err == nil || !strings.Contains(err.Error(), rule) {
		t.Fatalf("block %v: error = %v, want one containing %q", blocks, err, rule)
	}
	if got != nil || len(g.Blocks()) != 0 {
		t.Fatalf("rejected declaration blocked %v, combat %v", got, g.Blocks())
	}
}

func wantBlockLegal(t *testing.T, g *engine.Game, blocks ...engine.Block) {
	t.Helper()
	got, err := declareBlocks(g, blocks...)
	if err != nil {
		t.Fatalf("block %v: %v", blocks, err)
	}
	if len(got) != len(blocks) {
		t.Fatalf("blocks = %v, want %v", got, blocks)
	}
}

func blk(blocker, attacker engine.CardID) engine.Block {
	return engine.Block{Blocker: blocker, Attacker: attacker}
}

// resolveMustBlock resolves a MustBlock line on a fresh 1/1 creature of p's
// (resolveNow's host), targeting targets, and returns that creature.
func resolveMustBlock(t *testing.T, g *engine.Game, p engine.PlayerID, targets []engine.CardID, line string, svars ...string) engine.CardID {
	t.Helper()
	var ents []engine.EntityID
	for _, id := range targets {
		ents = append(ents, engine.CardEntity(id))
	}
	host, err := resolveNow(t, g, p, engine.NewScriptedController(), ents, line, svars...)
	if err != nil {
		t.Fatalf("resolve %q: %v", line, err)
	}
	return host
}

func TestMustBlockEffectRequiresTheTargetToBlockTheHost(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(combatCreature(t, "Target", "2", "2"), other, engine.Battlefield)
	bystander := g.NewCard(combatCreature(t, "Bystander", "2", "2"), p, engine.Battlefield)
	host := resolveMustBlock(t, g, p, []engine.CardID{target}, "DB$ MustBlock | ValidTgts$ Creature")
	g.SetTurnState(1, p, engine.DeclareAttackers)
	declareLegalAttack(t, g, host, bystander)

	wantBlockError(t, g, "must still block")
	wantBlockError(t, g, "509.1c", blk(target, bystander))
	wantBlockLegal(t, g, blk(target, host))
}

// Without BlockAllDefined$ only DefinedAttacker$'s first card is required
// (MustBlockEffect.java:78); with it, every card it names is, and blocking
// any one of them is as much as a single blocker can do.
func TestMustBlockEffectDefinedAttackers(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		line          string
		secondIsLegal bool
	}{
		"first only": {"DB$ MustBlock | ValidTgts$ Creature | DefinedAttacker$ Remembered", false},
		"all":        {"DB$ MustBlock | ValidTgts$ Creature | DefinedAttacker$ Remembered | BlockAllDefined$ True", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			g.SetTurnState(1, p, engine.Main1)
			target := g.NewCard(combatCreature(t, "Target", "2", "2"), other, engine.Battlefield)
			a1 := g.NewCard(combatCreature(t, "A1", "2", "2"), p, engine.Battlefield)
			a2 := g.NewCard(combatCreature(t, "A2", "2", "2"), p, engine.Battlefield)
			host := g.NewCard(combatCreature(t, "Host", "1", "1"), p, engine.Battlefield)
			g.Card(host).Memory.Remember(engine.CardEntity(a1))
			g.Card(host).Memory.Remember(engine.CardEntity(a2))
			resolveAgain(t, g, p, host, target, tc.line)
			g.SetTurnState(1, p, engine.DeclareAttackers)
			declareLegalAttack(t, g, a1, a2)

			wantBlockError(t, g, "must")
			if tc.secondIsLegal {
				wantBlockLegal(t, g, blk(target, a2))
			} else {
				wantBlockError(t, g, "509.1c", blk(target, a2))
				wantBlockLegal(t, g, blk(target, a1))
			}
		})
	}
}

// resolveAgain pushes line as an ability of host targeting target and
// resolves it.
func resolveAgain(t *testing.T, g *engine.Game, p engine.PlayerID, host, target engine.CardID, line string) {
	t.Helper()
	def := etbChainDef(t, "Again", line)
	ab := def.Faces[0].Triggers[0].Subs[0].Ability
	api, ok := engine.APIByName(ab.Name)
	if !ok {
		t.Fatalf("unknown API %q", ab.Name)
	}
	g.PushAbility(engine.Ability{API: api, Source: host, Controller: p, Params: ab, Amounts: def.Faces[0].Amounts,
		Targets: []engine.EntityID{engine.CardEntity(target)}})
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("resolve %q: %v", line, err)
	}
}

func TestMustBlockEffectEndsWhenTheBlockerLeaves(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(combatCreature(t, "Target", "2", "2"), other, engine.Battlefield)
	host := resolveMustBlock(t, g, p, []engine.CardID{target}, "DB$ MustBlock | ValidTgts$ Creature")
	g.Move(target, engine.Hand, other)
	g.Move(target, engine.Battlefield, other)
	g.Card(target).SummonSick = false
	g.SetTurnState(1, p, engine.DeclareAttackers)
	declareLegalAttack(t, g, host)
	wantBlockLegal(t, g)
}

func TestMustBlockEffectSkipsATargetOffTheBattlefield(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(combatCreature(t, "Target", "2", "2"), other, engine.Hand)
	host := resolveMustBlock(t, g, p, []engine.CardID{target}, "DB$ MustBlock | ValidTgts$ Creature")
	g.Move(target, engine.Battlefield, other)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	declareLegalAttack(t, g, host)
	wantBlockLegal(t, g)
}

// An empty DefinedAttacker$ resolves to nothing (MustBlockEffect.java:32).
func TestMustBlockEffectWithNoDefinedAttackerDoesNothing(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(combatCreature(t, "Target", "2", "2"), other, engine.Battlefield)
	host := resolveMustBlock(t, g, p, []engine.CardID{target}, "DB$ MustBlock | ValidTgts$ Creature | DefinedAttacker$ Remembered")
	g.SetTurnState(1, p, engine.DeclareAttackers)
	declareLegalAttack(t, g, host)
	wantBlockLegal(t, g)
}

// A Menace attacker the lone must-block creature cannot block alone leaves
// no requirement (validateBlocks' minimum-blockers exception).
func TestMustBlockStaticExcusedByMenace(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	attacker := g.NewCard(combatCreature(t, "Sneak", "2", "2", "K:Menace"), p, engine.Battlefield)
	g.NewCard(combatCreature(t, "Eager", "2", "2", "S:Mode$ MustBlock | ValidCreature$ Card.Self"), other, engine.Battlefield)
	declareLegalAttack(t, g, attacker)
	wantBlockLegal(t, g)
}

func TestMustBlockStaticUnresolvableIsAnError(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	attacker := g.NewCard(combatCreature(t, "Attacker", "2", "2"), p, engine.Battlefield)
	g.NewCard(combatCreature(t, "Eager", "2", "2", "S:Mode$ MustBlock | ValidCreature$ Card.Self | CheckSVar$ X"), other, engine.Battlefield)
	declareLegalAttack(t, g, attacker)
	wantBlockError(t, g, "not resolvable yet")
}

func TestLureKeywords(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		keyword      string
		noneLegal    bool
		oneLegal     bool
		twoLegal     bool
		elsewhereLeg bool
	}{
		"all able":        {"All creatures able to block CARDNAME do so.", false, false, true, false},
		"must be blocked": {"CARDNAME must be blocked if able.", false, true, true, true},
		"exactly one":     {"CARDNAME must be blocked by exactly one creature if able.", false, true, true, true},
		"two or more":     {"CARDNAME must be blocked by two or more creatures if able.", false, false, true, false},
		"by a wall":       {"MustBeBlockedBy Creature.Wall", false, true, true, true},
		"by every wall":   {"MustBeBlockedByAll:Creature.Wall:All Walls able to block CARDNAME do so.", false, false, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setup := func() (*engine.Game, engine.CardID, engine.CardID, engine.CardID, engine.CardID) {
				g, p, other := newTwoPlayerGame(t)
				g.SetTurnState(1, p, engine.DeclareAttackers)
				lure := g.NewCard(combatCreature(t, "Lure", "2", "2", "K:"+tc.keyword), p, engine.Battlefield)
				plain := g.NewCard(combatCreature(t, "Plain", "2", "2"), p, engine.Battlefield)
				w1 := g.NewCard(copyTestDef(t, "Wall1", "Creature Wall", "0", "4"), other, engine.Battlefield)
				w2 := g.NewCard(copyTestDef(t, "Wall2", "Creature Wall", "0", "4"), other, engine.Battlefield)
				declareLegalAttack(t, g, lure, plain)
				return g, lure, plain, w1, w2
			}
			check := func(legal bool, blocks func(lure, plain, w1, w2 engine.CardID) []engine.Block) {
				t.Helper()
				g, lure, plain, w1, w2 := setup()
				_, err := declareBlocks(g, blocks(lure, plain, w1, w2)...)
				if (err == nil) != legal {
					t.Fatalf("legal = %v, error %v", legal, err)
				}
			}
			check(tc.noneLegal, func(_, _, _, _ engine.CardID) []engine.Block { return nil })
			check(tc.oneLegal, func(lure, _, w1, _ engine.CardID) []engine.Block { return []engine.Block{blk(w1, lure)} })
			check(tc.twoLegal, func(lure, _, w1, w2 engine.CardID) []engine.Block {
				if tc.keyword == "CARDNAME must be blocked by exactly one creature if able." {
					return []engine.Block{blk(w1, lure)}
				}
				return []engine.Block{blk(w1, lure), blk(w2, lure)}
			})
			check(tc.elsewhereLeg, func(lure, plain, w1, w2 engine.CardID) []engine.Block {
				return []engine.Block{blk(w1, lure), blk(w2, plain)}
			})
		})
	}
}

func TestBlockRestrictionKeywords(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		keyword   string
		power     string
		alone     bool
		withOther bool
	}{
		"cant block":           {"CARDNAME can't block.", "2", false, false},
		"cant block alone":     {"CARDNAME can't block alone.", "2", false, true},
		"two others":           {"CARDNAME can't block unless at least two other creatures block.", "2", false, false},
		"greater power":        {"CARDNAME can't block unless a creature with greater power also blocks.", "1", false, true},
		"greater power absent": {"CARDNAME can't block unless a creature with greater power also blocks.", "5", false, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, withOther := range []bool{false, true} {
				g, p, other := newTwoPlayerGame(t)
				g.SetTurnState(1, p, engine.DeclareAttackers)
				attacker := g.NewCard(combatCreature(t, "Attacker", "2", "2"), p, engine.Battlefield)
				shy := g.NewCard(combatCreature(t, "Shy", tc.power, "2", "K:"+tc.keyword), other, engine.Battlefield)
				friend := g.NewCard(combatCreature(t, "Friend", "3", "3"), other, engine.Battlefield)
				declareLegalAttack(t, g, attacker)
				blocks := []engine.Block{blk(shy, attacker)}
				want := tc.alone
				if withOther {
					blocks = append(blocks, blk(friend, attacker))
					want = tc.withOther
				}
				_, err := declareBlocks(g, blocks...)
				if (err == nil) != want {
					t.Fatalf("with other %v: error %v, want legal=%v", withOther, err, want)
				}
			}
		})
	}
}

// Without a block requirement the unmodeled restrictions change nothing.
func TestUnmodeledBlockRestrictionWithoutRequirementIsNoError(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	attacker := g.NewCard(combatCreature(t, "Attacker", "2", "2"), p, engine.Battlefield)
	b := g.NewCard(combatCreature(t, "B", "2", "2"), other, engine.Battlefield)
	g.NewCard(continuousDef(t, "Rule", "Mode$ BlockRestrict | MaxBlockers$ 1"), other, engine.Battlefield)
	declareLegalAttack(t, g, attacker)
	wantBlockLegal(t, g, blk(b, attacker))
}

func TestCanBlockReadsCantBlockKeywords(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	attacker := g.NewCard(combatCreature(t, "Attacker", "2", "2"), p, engine.Battlefield)
	wall := g.NewCard(combatCreature(t, "Wall", "0", "4", "K:CARDNAME can't attack or block."), other, engine.Battlefield)
	loner := g.NewCard(combatCreature(t, "Loner", "1", "1", "K:CARDNAME can't attack or block alone."), p, engine.Battlefield)
	if g.CanBlock(attacker, wall) {
		t.Error("a creature that can't block can block")
	}
	if !g.CanBlock(attacker, loner) {
		t.Error("a creature that can't block alone, with company, can't block")
	}
	g.Move(attacker, engine.Graveyard, p)
	if g.CanBlock(wall, loner) {
		t.Error("a lone creature that can't block alone can block")
	}
}
