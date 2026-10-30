package boosters

import (
	"errors"
	"slices"
	"testing"
)

// The Java collections whose order a sheet depends on, checked against values
// computed by hand from the JDK sources (the oracle checks the same code
// end to end in oracle_test.go).

func TestStringHashIsStringHashCode(t *testing.T) {
	t.Parallel()

	for s, want := range map[string]int32{"": 0, "a": 97, "hello": 99162322, "Hello, World!": 1498789909} {
		if got := stringHash(s); got != want {
			t.Errorf("stringHash(%q) = %d, want %d", s, got, want)
		}
	}
	// U+1F600 is the pair D83D DE00: 31*0xD83D + 0xDE00.
	if got, want := stringHash("\U0001F600"), int32(31*0xD83D+0xDE00); got != want {
		t.Errorf("stringHash of a surrogate pair = %d, want %d", got, want)
	}
}

func TestHashMapOrderIsBucketThenInsertion(t *testing.T) {
	t.Parallel()

	// "cards" is in bucket 3 and "promo" in 16 of a table of 32, "buy a box" and
	// "rebalanced" share bucket 2 and keep their insertion order.
	keys := []string{"promo", "cards", "rebalanced", "buy a box"}
	got := hashMapOrder(keys)
	want := []int{2, 3, 1, 0}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	// More keys than 0.75 of the table double it: 25 keys make the table 64.
	many := make([]string, 25)
	for i := range many {
		many[i] = string(rune('a' + i))
	}
	if got := hashMapOrder(many); len(got) != 25 {
		t.Errorf("order of 25 keys has %d entries", len(got))
	}
}

// chmFill puts keys with the given hashes into a map, key i being hash[i], all
// distinct.
func chmFill(t *testing.T, hashes []int32) *chmMap {
	t.Helper()
	m := &chmMap{}
	for i, h := range hashes {
		if err := m.merge(h, i, func(int) bool { return false }); err != nil {
			t.Fatalf("merge %d: %v", i, err)
		}
	}
	return m
}

func TestChmOrderIsTableIndexThenBin(t *testing.T) {
	t.Parallel()

	m := chmFill(t, []int32{3, 1, 17, 2, 33})
	// Table 16: bucket 1 holds hashes 1, 17, 33 in insertion order.
	if got, want := m.order(), []int{1, 2, 4, 3, 0}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestChmMergeFindsTheEqualKey(t *testing.T) {
	t.Parallel()

	m := &chmMap{}
	same := func(other int) bool { return other == 0 }
	for i := range 3 {
		if err := m.merge(5, i, same); err != nil {
			t.Fatal(err)
		}
	}
	if got := m.order(); !slices.Equal(got, []int{0}) {
		t.Errorf("order = %v, want just the first key", got)
	}
}

func TestChmTransferReversesTheNodesBeforeTheLastRun(t *testing.T) {
	t.Parallel()

	// Thirteen distinct hashes grow the 16-table (threshold 12) to 32. Bin 1
	// holds 1, 17, 33(=1 mod 16), ...: after the split the low bin keeps the
	// last run as it is and the earlier nodes come before it reversed.
	hashes := []int32{1, 17, 33, 49, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	m := chmFill(t, hashes)
	if len(m.table) != 32 {
		t.Fatalf("table has %d bins, want 32", len(m.table))
	}
	// Bin 1 held hashes 1, 17, 33, 49 when the table grew. Bit 16 splits it: the
	// last run is [49] for the high bin; 1 and 33 go low, each put in front, so
	// 33 comes before 1; 17 is put in front of 49 in the high bin.
	got := m.order()
	idx1, idx33 := slices.Index(got, 0), slices.Index(got, 2)
	idx17, idx49 := slices.Index(got, 1), slices.Index(got, 3)
	if idx33 > idx1 {
		t.Errorf("hash 33 at %d after hash 1 at %d: the nodes before the last run are reversed", idx33, idx1)
	}
	if idx17 > idx49 {
		t.Errorf("hash 17 at %d after hash 49 at %d", idx17, idx49)
	}
}

func TestChmBinOfEightGrowsASmallTableAndRefusesALargeOne(t *testing.T) {
	t.Parallel()

	// Nine hashes in one bin of the 16-table: tryPresize doubles it.
	var same []int32
	for i := range 9 {
		same = append(same, int32(i*16))
	}
	m := chmFill(t, same)
	if len(m.table) < 32 {
		t.Errorf("table has %d bins after nine collisions, want it grown", len(m.table))
	}

	// In a table of 64 or more the bin would be treeified, which is refused.
	big := &chmMap{}
	var err error
	for i := range 4000 {
		// 4000 hashes spread over the table, then nine more in one bin.
		_ = big.merge(int32(i), i, func(int) bool { return false })
	}
	for i := range 40 {
		if err = big.merge(int32(100000+i*8192), 10000+i, func(int) bool { return false }); err != nil {
			break
		}
	}
	if !errors.Is(err, ErrNotModelled) {
		t.Errorf("a treeified bin gave err = %v, want ErrNotModelled", err)
	}
}

func TestTableSizeForIsThePowerOfTwoAbove(t *testing.T) {
	t.Parallel()

	for in, want := range map[int]int{1: 1, 2: 2, 3: 4, 17: 32, 64: 64, 65: 128} {
		if got := tableSizeFor(in); got != want {
			t.Errorf("tableSizeFor(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestSplitHelpersKeepParenthesesAndQuotesTogether(t *testing.T) {
	t.Parallel()

	if got := splitParen(`Common:fromSheet("A B") DOM`, ' ', 2); !slices.Equal(got, []string{`Common:fromSheet("A B")`, "DOM"}) {
		t.Errorf("splitParen = %q", got)
	}
	if got := splitParen("a::b", ':', 1<<30); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("splitParen drops empty parts: %q", got)
	}
	if got := splitQuoted(`"Name, One","Two"`); !slices.Equal(got, []string{"Name, One", "Two"}) {
		t.Errorf("splitQuoted = %q", got)
	}
}

func TestEditionTextHelpers(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]Rarity{"C": Common, "uncommon": Uncommon, "Mythic Rare": MythicRare, "L": BasicLand, "?": Unknown, "zzz": Unknown} {
		if got := rarityOf(in); got != want {
			t.Errorf("rarityOf(%q) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[string]string{"": "50000", "7": "00007", "12a": "00012a", "WS6": "WS00006", "S": "S00000", "x1": "x00001"} {
		if in == "" {
			in = ""
		}
		if got := sortableCollector(in); got != want {
			t.Errorf("sortableCollector(%q) = %q, want %q", in, got, want)
		}
	}
	if compareIgnoreCase("abc", "ABD") >= 0 || compareIgnoreCase("B", "a") <= 0 || compareIgnoreCase("x", "X") != 0 || compareIgnoreCase("a", "ab") >= 0 {
		t.Error("compareIgnoreCase does not order strings without case")
	}
	if !parseDate("2020-05").Equal(parseDate("2020-05-01")) {
		t.Error("a short date is the first of the month")
	}
	if got := extraParam(`"variant": "C", "flavorname": "Z"`, "variant"); got != "C" {
		t.Errorf("extraParam variant = %q", got)
	}
	if extraParam("", "variant") != "" || extraParam("junk", "variant") != "" {
		t.Error("extraParam of nothing is empty")
	}
}

const sampleEdition = `[metadata]
Code=TST
Code2=TS
Date=2020-05-01
Name=Test Set
BoosterCovers=3
BoosterSlots=Common,Wild
Booster=2 Common, 1 Wild+
FoilChanceInBooster=10
ChanceReplaceCommonWith=0.5F Rare
foil=oldstyle
AdditionalSheetForFoils=fromSheet("TST foils")

[cards]
# a comment
2 C Second Card @Artist
1 R First Card @Artist ${"variant": "A"}
1 C first card

[promo]
5 S Promo Card

[Common]
Base=Common

[Wild]
Base=Common
Replace=0.25F Rare
Replace=0.25F Uncommon
Replace=0.5F Mythic

[Custom Sheet]
1 Some Card|TST
`

func TestEditionFileReading(t *testing.T) {
	t.Parallel()

	e, err := parseEdition(sampleEdition)
	if err != nil {
		t.Fatal(err)
	}
	if e.Code != "TST" || e.Code2 != "TS" || e.Covers != 3 || !e.slotted || e.foilType != "OLD_STYLE" {
		t.Errorf("edition = %+v", e)
	}
	if e.foilChance != 0.1 || e.replaceProb != 0.5 || e.replaceSlot != "Rare" {
		t.Errorf("foil chance %v, replace %v %q", e.foilChance, e.replaceProb, e.replaceSlot)
	}
	// Sorted by name without case, then collector number, then rarity: the two
	// first cards (the common before the rare), the promo, the second card.
	var names []string
	for _, en := range e.cards {
		names = append(names, en.name)
	}
	if want := []string{"first card", "First Card", "Promo Card", "Second Card"}; !slices.Equal(names, want) {
		t.Errorf("cards = %v, want %v", names, want)
	}
	if e.cards[1].variant != "A" || e.cards[0].variant != "" {
		t.Errorf("variants = %q, %q", e.cards[0].variant, e.cards[1].variant)
	}
	if len(e.slots) != 2 || len(e.slots[1].replaces) != 3 {
		t.Fatalf("slots = %+v", e.slots)
	}
	// Cumulative ranges: .25, .5, 1.0 in ascending order.
	if got := e.slots[1].replaces; got[0].upTo != 0.25 || got[1].upTo != 0.5 || got[2].upTo != 1 {
		t.Errorf("ranges = %+v", got)
	}
	if len(e.custom) != 1 || e.custom[0].name != "Custom Sheet" {
		t.Errorf("custom sections = %+v", e.custom)
	}
	if !e.isTraditionalFrame("1") || e.isTraditionalFrame("5") || e.isTraditionalFrame("") {
		t.Error("only the cards section's collector numbers are traditional")
	}
}

func TestEditionFileErrors(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{
		"[metadata]\nCode=X\nBooster=x Common\n",
		"[metadata]\nCode=X\nChanceReplaceCommonWith=zzz Common\n",
		"[metadata]\nCode=X\nFoilChanceInBooster=zzz\n",
		"[metadata]\nCode=X\nBoosterSlots=A\nBooster=1 A\n\n[A]\nReplace=zzz Sheet\n",
	} {
		if _, err := parseEdition(bad); err == nil {
			t.Errorf("edition %q read without error", bad)
		}
	}
}

func TestBoosterSlotPutReplacesAnEqualKey(t *testing.T) {
	t.Parallel()

	var b boosterSlot
	b.put(0.5, "a")
	b.put(0.25, "b")
	b.put(0.5, "c")
	if len(b.replaces) != 2 || b.replaces[0].sheet != "b" || b.replaces[1].sheet != "c" {
		t.Errorf("replaces = %+v", b.replaces)
	}
}

func TestSlotTypeIsTheWordBeforeASeparator(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{"Common": "Common", `Common:fromSheet("X")`: "Common", "BasicLand DOM": "BasicLand", "!Land": ""} {
		if got := slotType(in); got != want {
			t.Errorf("slotType(%q) = %q, want %q", in, got, want)
		}
	}
	tpl := &Template{Slots: []slotSpec{{spec: "Rare:x", count: 1}, {spec: "dfc", count: 1}}}
	if !tpl.hasSlot("Rare") || !tpl.hasSlot("dfc") || tpl.hasSlot("Common") {
		t.Error("hasSlot reads the slot types")
	}
}

func TestParseRequestReadsTheSegments(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]request{
		"Name":           {name: "Name", art: 1, collector: noCollectorNumber},
		"Name|SET":       {name: "Name", edition: "SET", art: 1, collector: noCollectorNumber},
		"Name|SET|3":     {name: "Name", edition: "SET", art: 3, collector: noCollectorNumber},
		"Name|SET|[12a]": {name: "Name", edition: "SET", art: -1, collector: "12a"},
		"Name|SET|2|[7]": {name: "Name", edition: "SET", art: 2, collector: "7"},
	} {
		if got := parseRequest(in); got != want {
			t.Errorf("parseRequest(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestPrintFoilAndEquality(t *testing.T) {
	t.Parallel()

	p := &Print{Name: "A", Edition: "X", Collector: "1", Art: 1}
	p.hash = p.hashOf()
	f := p.Foiled()
	if f == p || !f.Foil || f.Foiled() != f || f.equal(p) || !p.equal(&Print{Name: "A", Edition: "X", Collector: "1", Art: 1}) {
		t.Error("a foil is a different print, and foiling it again is itself")
	}
	if f.hash == p.hash {
		t.Error("the foil hashes differently")
	}
}

func TestReplaceCardKeepsTheFoilStateAndRarity(t *testing.T) {
	t.Parallel()

	mk := func(name string, r Rarity, foil bool) *Print {
		p := &Print{Name: name, Edition: "X", Collector: "1", Art: 1, Rarity: r, Foil: foil}
		p.hash = p.hashOf()
		return p
	}
	pack := []*Print{mk("c", Common, false), mk("r", Rare, true), mk("u", Uncommon, false)}
	got := replaceCard(pack, mk("m", MythicRare, false))
	if len(got) != 3 || got[2].Name != "m" || !got[2].Foil || got[1].Name != "u" {
		t.Errorf("replaced pack = %v", packLines(got))
	}
	same := replaceCard(pack, mk("s", Special, false))
	if len(same) != 3 || same[0].Name != "c" {
		t.Errorf("a card of a rarity the pack lacks changed it: %v", packLines(same))
	}
	for _, r := range []Rarity{BasicLand, Common, Uncommon} {
		if got := replaceCard([]*Print{mk("x", r, false)}, mk("y", r, false)); got[0].Name != "y" {
			t.Errorf("rarity %v not replaced", r)
		}
	}
}

func packLines(cards []*Print) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Describe()
	}
	return out
}
