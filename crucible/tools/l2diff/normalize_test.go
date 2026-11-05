package main

import "testing"

// Each case pins one row of l2-verbs.md's "Normaliser" table: a Java dump and
// a Go dump of the same post-state normalise to the same text.
func TestNormalizeRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		rule, java, goText, want string
	}{
		{"N1 player prefixes", "p0life=20\np1life=18\nactiveplayer=p1\n", "humanlife=20\nailife=18\nactiveplayer=ai\n",
			"activeplayer=ai\nailife=18\nhumanlife=20\n"},
		{"N1 player tokens", "p0battlefield=Bears|Set:A|Art:1|Owner:P1\nmonarch=p1\n", "humanbattlefield=Bears|Owner:ai\nmonarch=ai\n",
			"humanbattlefield=Bears|Owner:ai\nmonarch=ai\n"},
		{"N2 phase constant", "activephase=END_OF_TURN\n", "activephase=End of Turn\n", "activephase=END_OF_TURN\n"},
		{"N3 printing tokens", "p0battlefield=Plains|Set:MAR|Art:1|Tapped\n", "humanbattlefield=Plains|Tapped\n",
			"humanbattlefield=Plains|Tapped\n"},
		{"N4 empty and zero keys", "p0hand=\np0landsplayed=0\np0speed=0\np0numringtemptedyou=0\np0landsplayedlastturn=0\np0life=20\n",
			"humanlife=20\n", "humanlife=20\n"},
		{"N4 a non-zero count stays", "p0landsplayed=1\n", "humanlandsplayed=1\n", "humanlandsplayed=1\n"},
		{"N5 ids renumbered and references rewritten",
			"p0battlefield=Forest|Set:A|Art:1;Rancor|Set:B|Art:1|AttachedTo:301;Bears|Set:C|Art:1|Id:301\n",
			"humanbattlefield=Forest|Id:1;Rancor|Id:2|AttachedTo:3;Bears|Id:3\n",
			"humanbattlefield=Forest;Rancor|AttachedTo:1;Bears|Id:1\n"},
		{"N5 a reference list", "p1exile=Bears|Id:7;Elves|Id:9\np0battlefield=Ring|RememberedCards:9,7\n",
			"aiexile=Bears|Id:2;Elves|Id:3\nhumanbattlefield=Ring|Id:1|RememberedCards:3,2\n",
			"aiexile=Bears|Id:1;Elves|Id:2\nhumanbattlefield=Ring|RememberedCards:2,1\n"},
		{"N6 The Monarch", "p0command=The Monarch|Set:X|Art:1\nmonarch=human\n", "monarch=human\n", "monarch=human\n"},
		{"N6 The Initiative", "p1command=The Initiative|Set:X|Art:1\n", "", "\n"},
		{"N6 The Ring", "p0command=The Ring|Set:X|Art:1\n", "", "\n"},
		{"N6 another effect card stays", "p0command=Emblem|Set:X|Art:1\n", "humancommand=Emblem\n", "humancommand=Emblem\n"},
		{"N9 a lost player keeps only lost=", "p1life=20\np0lost=true\nover=true\n",
			"humanlife=0\nhumanlost=true\nhumangraveyard=Bears\nailife=20\nover=true\n",
			"ailife=20\nhumanlost=true\nover=true\n"},
		{"N7 token order", "p0battlefield=Bears|Tapped|Counters:P1P1=1|SummonSick\n",
			"humanbattlefield=Bears|SummonSick|Counters:P1P1=1|Tapped\n", "humanbattlefield=Bears|Counters:P1P1=1|SummonSick|Tapped\n"},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			t.Parallel()
			j, err := Normalize(c.java, JavaSide)
			if err != nil {
				t.Fatal(err)
			}
			g, err := Normalize(c.goText, GoSide)
			if err != nil {
				t.Fatal(err)
			}
			if j != c.want || g != c.want {
				t.Errorf("java %q, go %q, want %q", j, g, c.want)
			}
		})
	}
}

// N6 is Java-only: a Go dump never writes a designation card, so one there is
// a difference, not noise.
func TestNormalizeKeepsADesignationCardOnTheGoSide(t *testing.T) {
	t.Parallel()
	g, err := Normalize("humancommand=The Monarch\n", GoSide)
	if err != nil {
		t.Fatal(err)
	}
	if g != "humancommand=The Monarch\n" {
		t.Errorf("got %q", g)
	}
}

func TestNormalizeRejectsALineWithoutAKey(t *testing.T) {
	t.Parallel()
	if _, err := Normalize("humanlife\n", GoSide); err == nil {
		t.Error("no error for a line without '='")
	}
}

func TestJavaPhaseLeavesAConstantAlone(t *testing.T) {
	t.Parallel()
	if got := JavaPhase("MAIN2"); got != "MAIN2" {
		t.Errorf("got %q", got)
	}
}
