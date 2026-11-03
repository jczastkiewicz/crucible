package compile_test

import "testing"

// K:Awaken compiles a second copy of the first spell whose chain ends with
// the land's PutCounter and Animate (CardFactoryUtil.java:2636-2652); the
// card's own spell is left as written. A line without a count, or a face
// without a spell, gets no awaken spell.
func TestAwakenCompilesItsOwnSpellChain(t *testing.T) {
	t.Parallel()

	out := compileScript(t, "Name:X\nManaCost:3 U\nTypes:Sorcery\nK:Awaken:4:5 U\n"+
		"A:SP$ Draw | NumCards$ 2 | SpellDescription$ Draw two cards.\n")
	face := out.Faces[0]
	if len(face.Abilities[0].Subs) != 0 {
		t.Errorf("the card's own spell gained %d subs", len(face.Abilities[0].Subs))
	}
	awaken := face.AwakenSpell
	if awaken == nil || awaken == face.Abilities[0] || awaken.Name != "Draw" {
		t.Fatalf("AwakenSpell %+v, want a fresh Draw spell", awaken)
	}
	var names []string
	for a := awaken; ; {
		names = append(names, a.Name)
		if len(a.Subs) == 0 {
			break
		}
		a = a.Subs[0].Ability
	}
	if len(names) != 3 || names[1] != "PutCounter" || names[2] != "Animate" {
		t.Errorf("chain %v, want Draw, PutCounter, Animate", names)
	}

	for _, script := range []string{
		"Name:X\nManaCost:3 U\nTypes:Sorcery\nK:Awaken\nA:SP$ Draw | NumCards$ 2\n",
		"Name:X\nManaCost:3\nTypes:Artifact\nK:Awaken:4:5 U\n",
	} {
		if got := compileScript(t, script).Faces[0].AwakenSpell; got != nil {
			t.Errorf("%q: AwakenSpell %+v, want none", script, got)
		}
	}
}
