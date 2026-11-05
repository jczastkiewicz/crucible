package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diff sorts each scenario by what the Java runner wrote for it: an unmapped
// line or a Java error is not runnable, equal normalised states are same, and
// a mismatch is random when Go drew from its generator, differs otherwise.
func TestDiffSortsScenariosIntoTheReportLists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"same", "differs", "random", "unmapped", "error", "missing"} {
		write("go/"+name+".state", "humanlife=20\nactivephase=Main1\n")
	}
	write("java/same.state", "p0life=20\nactivephase=MAIN1\n")
	write("java/differs.state", "p0life=19\nactivephase=MAIN1\n")
	write("java/random.state", "p0life=18\nactivephase=MAIN1\n")
	write("java/unmapped.unmapped", "castspell human 1\nresolvestack\n")
	write("java/error.error", "java.lang.IllegalStateException: boom\n\tat x\n")
	write("random.txt", "random\n")

	if err := diff(dir); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"same: 1\n", "differs: 1\n", "random: 1\n", "not runnable: 3\n",
		"## differs\ndiffers\n",
		"## random\nrandom\n",
		"error\terror: java.lang.IllegalStateException: boom\n",
		"missing\tno Java output\n",
		"unmapped\tunmapped: castspell human 1\n",
	} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	d, err := os.ReadFile(filepath.Join(dir, "diffs", "differs.diff"))
	if err != nil {
		t.Fatal(err)
	}
	if string(d) != "- go   humanlife=20\n+ java humanlife=19\n" {
		t.Errorf("differs.diff = %q", d)
	}
}
