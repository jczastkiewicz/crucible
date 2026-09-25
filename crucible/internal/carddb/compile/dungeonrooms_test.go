package compile_test

import (
	"errors"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// A dungeon's K:Dungeon keyword becomes one RoomEntered trigger per room, in
// keyword order, each executing its room's ability, which learns the names
// of the rooms it leads to (CardFactoryUtil.java:1955-1989). Checked on the
// real Undercity and Lost Mine of Phandelver scripts.
func TestDungeonKeywordCompilesRoomTriggers(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "forge-gui", "res", "tokenscripts")
	typeList := filepath.Join(repoRoot(t), "forge-gui", "res", "lists", "TypeLists.txt")
	tokens, err := compile.LoadTokenScripts(root, typeList)
	if err != nil {
		t.Fatalf("LoadTokenScripts: %v", err)
	}
	for _, tt := range []struct {
		script      string
		rooms       int
		first, next string
		last        string
	}{
		{"undercity", 9, "Secret Entrance", "Forge,Lost Well", "Throne of the Dead Three"},
		{"lost_mine_of_phandelver", 7, "Cave Entrance", "Goblin Lair,Mine Tunnels", "Temple of Dumathoin"},
	} {
		face := tokens[tt.script].Faces[0]
		if len(face.Triggers) != tt.rooms {
			t.Fatalf("%s: %d triggers, want %d rooms", tt.script, len(face.Triggers), tt.rooms)
		}
		first := face.Triggers[0]
		if first.Name != "RoomEntered" || first.Record != compile.Trigger {
			t.Errorf("%s: first trigger %s %v, want a RoomEntered trigger", tt.script, first.Name, first.Record)
		}
		if v, _ := first.Param("ValidRoom"); v != tt.first {
			t.Errorf("%s: first ValidRoom$ %q, want %q", tt.script, v, tt.first)
		}
		if v, _ := first.Param("ValidCard"); v != "Card.Self" {
			t.Errorf("%s: ValidCard$ %q, want Card.Self", tt.script, v)
		}
		exec := first.Subs[0]
		if exec.Key != "Execute" {
			t.Fatalf("%s: sub key %q, want Execute", tt.script, exec.Key)
		}
		if v, _ := exec.Ability.Param("NextRoomName"); v != tt.next {
			t.Errorf("%s: NextRoomName$ %q, want %q", tt.script, v, tt.next)
		}
		lastExec := face.Triggers[len(face.Triggers)-1].Subs[0].Ability
		if v, _ := lastExec.Param("RoomName"); v != tt.last {
			t.Errorf("%s: last room %q, want %q", tt.script, v, tt.last)
		}
		if _, ok := lastExec.Param("NextRoomName"); ok {
			t.Errorf("%s: the last room leads somewhere", tt.script)
		}
	}

	db := compile.NewDB(nil).WithTokens(tokens)
	names := db.TokenScripts()
	if len(names) != len(tokens) || !sort.StringsAreSorted(names) {
		t.Errorf("TokenScripts: %d names sorted=%v, want all %d sorted", len(names), sort.StringsAreSorted(names), len(tokens))
	}
	var nilDB *compile.DB
	if nilDB.TokenScripts() != nil {
		t.Error("a nil DB listed token scripts")
	}
}

// A room with no RoomName$, or leading to an SVar its keyword does not list,
// is a script to fix, not a room to guess at: Java dereferences null.
func TestDungeonKeywordRejectsBadRooms(t *testing.T) {
	t.Parallel()

	for name, script := range map[string]string{
		"no room name": "Name:D\nTypes:Dungeon\nK:Dungeon:A\nSVar:A:DB$ Draw\n",
		"unlisted next": "Name:D\nTypes:Dungeon\nK:Dungeon:A\n" +
			"SVar:A:DB$ Draw | RoomName$ A | NextRoom$ B\nSVar:B:DB$ Draw | RoomName$ B\n",
		"missing svar": "Name:D\nTypes:Dungeon\nK:Dungeon:A,Z\nSVar:A:DB$ Draw | RoomName$ A\n",
	} {
		card, err := carddb.ParseScript(testRegistry(t), "fixture", []byte(script))
		if err != nil {
			t.Fatalf("%s: ParseScript: %v", name, err)
		}
		_, err = compile.Compile(card)
		if err == nil {
			t.Errorf("%s: Compile succeeded, want an error", name)
			continue
		}
		if name != "missing svar" && !errors.Is(err, compile.ErrBadRoom) {
			t.Errorf("%s: err = %v, want ErrBadRoom", name, err)
		}
	}
}
