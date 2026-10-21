package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): abilityListIDs and listHeadCards answer per head, and what
// a head records is private to the resolving ability.

import (
	"slices"
	"testing"
)

func TestAbilityListIDsReadTheResolvingAbilitysRecords(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p := g.Players()[0]
	def := targetCandidatesTestCreature(t)
	src := g.NewCard(def, p, Battlefield)
	c1 := g.NewCard(def, p, Battlefield)
	c2 := g.NewCard(def, p, Battlefield)

	if _, ok := g.abilityListIDs(src, "Sacrificed"); ok {
		t.Error("a head read with no resolving ability resolved")
	}
	a := &Ability{Source: src, Controller: p}
	g.resolving = a
	a.TriggerRemembered = []EntityID{CardEntity(c1), PlayerEntity(p)}
	a.paid = paidLists{recorded: true, tapped: []CardID{c1}, sacrificed: []CardID{c2}, exiled: []CardID{c1, c2}, discarded: []CardID{c2}}
	a.triggered.source = CardEntity(c1)
	a.triggered.target = CardEntity(c2)
	a.triggered.exploited, a.triggered.devoured = c1, c2
	a.triggered.attackers, a.triggered.attackersRecorded = []CardID{c1}, true

	for _, tc := range []struct {
		head string
		want []CardID
	}{
		{"TriggerRemembered", []CardID{c1}},
		{"Tapped", []CardID{c1}},
		{"Sacrificed", []CardID{c2}},
		{"Exiled", []CardID{c1, c2}},
		{"Discarded", []CardID{c2}},
		{"TriggeredSource", []CardID{c1}},
		{"CardTriggeredTarget", []CardID{c2}},
		{"TriggeredExploited", []CardID{c1}},
		{"TriggeredDevoured", []CardID{c2}},
		{"TriggerObjectsAttackers", []CardID{c1}},
	} {
		got, ok := g.abilityListIDs(src, tc.head)
		if !ok || !slices.Equal(got, tc.want) {
			t.Errorf("abilityListIDs(%q) = (%v, %v), want (%v, true)", tc.head, got, ok, tc.want)
		}
	}

	*a = Ability{Source: src, Controller: p}
	for _, head := range []string{"TriggeredExploited", "TriggeredDevoured", "TriggerObjectsAttackers", "TriggeredSource", "CardTriggeredTarget", "Sacrificed", "ParentTargeted", "Bogus"} {
		if got, ok := g.abilityListIDs(src, head); ok {
			t.Errorf("abilityListIDs(%q) with nothing recorded = (%v, true), want unresolved", head, got)
		}
	}
	a.triggered.source = PlayerEntity(p)
	if got, ok := g.abilityListIDs(src, "TriggeredSource"); !ok || len(got) != 0 {
		t.Errorf("a player source = (%v, %v), want no cards", got, ok)
	}
	a.ancestorTargeting = true
	if got, ok := g.abilityListIDs(src, "ParentTargeted"); !ok || len(got) != 0 {
		t.Errorf("a targeting ancestor with no card targets = (%v, %v), want none", got, ok)
	}
}

func TestListHeadCardsWithoutAHostOrOnAnEnchantedHost(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p := g.Players()[0]
	def := targetCandidatesTestCreature(t)
	aura := g.NewCard(def, p, Battlefield)
	host := g.NewCard(def, p, Battlefield)

	for _, head := range []string{"ExiledWith", "Emerged", "Convoked", "ChosenCard", "Remembered", "Imprinted", "Enchanted", "Equipped"} {
		if _, ok, handled := g.listHeadCards(p, NoCard, head); ok || !handled {
			t.Errorf("listHeadCards(%q) with no source = ok %v handled %v, want unresolved and handled", head, ok, handled)
		}
	}
	if _, ok, handled := g.listHeadCards(NoPlayer, aura, "DungeonsCompleted"); ok || !handled {
		t.Errorf("DungeonsCompleted with no controller = ok %v handled %v", ok, handled)
	}
	if _, ok, handled := g.listHeadCards(p, aura, "Crewed"); ok || !handled {
		t.Errorf("Crewed = ok %v handled %v, want unresolved and handled", ok, handled)
	}
	if cards, ok, handled := g.listHeadCards(p, aura, "Enchanted"); !ok || !handled || len(cards) != 0 {
		t.Errorf("unattached Enchanted = (%v, %v, %v), want an empty list", cards, ok, handled)
	}
	g.Attach(aura, host)
	if cards, ok, _ := g.listHeadCards(p, aura, "Enchanted"); !ok || len(cards) != 1 || cards[0].ID != host {
		t.Errorf("attached Enchanted = (%v, %v), want the host", cards, ok)
	}
	if _, _, handled := g.listHeadCards(p, aura, "Bogus"); handled {
		t.Error("an unknown head was reported handled")
	}
}
