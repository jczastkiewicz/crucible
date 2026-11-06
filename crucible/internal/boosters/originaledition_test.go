package boosters_test

import "testing"

// OriginalEdition is the print ORIGINAL_ART_ALL_EDITIONS picks: the oldest
// edition of the name, whatever reprinted it later.
func TestOriginalEditionIsTheOldestPrint(t *testing.T) {
	t.Parallel()

	tab := loadTable(t)
	for name, want := range map[string]string{"Grizzly Bears": "LEA", "Serendib Efreet": "ARN", "Mishra's Factory": "ATQ"} {
		if got, ok := tab.OriginalEdition(name); !ok || got != want {
			t.Errorf("OriginalEdition(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	if code, ok := tab.OriginalEdition("No Such Card"); ok {
		t.Errorf("OriginalEdition of an unprinted name = %q, want none", code)
	}
}
