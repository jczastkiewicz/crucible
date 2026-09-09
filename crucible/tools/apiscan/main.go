// Command apiscan extracts the parameter vocabulary of every ability API from
// Forge's own source.
//
// Nothing in Forge declares which params an effect accepts: each class reaches
// for `sa.getParam("...")` where it needs one. So the vocabulary is recovered
// by reading those call sites — per effect class for the API's own params, and
// per shared base class for the ones every ability may carry.
//
// The output is the input to M3's P2 gate: a param a card writes that no effect
// reads is either a hole in this scan or a dead param on the card, and both are
// worth knowing (ADR-0008).
//
//	go run ./tools/apiscan                 # the vocabulary, as TSV
//	go run ./tools/apiscan -check          # fail on any param key nothing reads
//
// -check is the gate. It compiles every card and reports each param key that no
// Java code reads, with the card and the line that writes it, unless the key is
// listed in the deliberate-exclusion table of
// docs/crucible/porting/parity-matrix.md.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	forge := flag.String("forge", "..", "path to the Forge checkout")
	corpus := flag.String("corpus", "../forge-gui/res/cardsfolder", "card script directory")
	types := flag.String("types", "../forge-gui/res/lists/TypeLists.txt", "subtype vocabulary")
	allow := flag.String("allow", "../docs/crucible/porting/parity-matrix.md", "deliberate-exclusion table")
	gate := flag.Bool("check", false, "fail on any param key nothing reads")
	flag.Parse()

	var err error
	if *gate {
		err = check(*forge, *corpus, *types, *allow)
	} else {
		err = run(*forge)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "apiscan: %v\n", err)
		os.Exit(1)
	}
}
