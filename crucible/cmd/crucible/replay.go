package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/replay"
)

// replayCmd is `crucible replay`: each recorded Java game (record-game.sh)
// replayed in Go, with the first unanswered question and the first event
// divergence (ADR-0010 L3, ADR-0063). It fails when any game differs.
func replayCmd(args []string) error {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	corpus := fs.String("corpus", "../forge-gui/res/cardsfolder", "card script directory")
	tokens := fs.String("tokens", "../forge-gui/res/tokenscripts", "token script directory")
	types := fs.String("types", "../forge-gui/res/lists/TypeLists.txt", "subtype vocabulary")
	turns := fs.Int("turns", 60, "turn cap")
	events := fs.Bool("events", false, "print the Go game's normalised events")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("replay: name one or more replay logs (.jsonl)")
	}
	db, err := compile.LoadDB(*corpus, *types)
	if err != nil {
		return err
	}
	tok, err := compile.LoadTokenScripts(*tokens, *types)
	if err != nil {
		return err
	}
	db = db.WithTokens(tok)
	differ := 0
	for _, path := range fs.Args() {
		res, err := replayOne(db, path, *turns)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %d turns\n", path, res.Turns)
		if *events {
			for i, e := range res.Go {
				fmt.Printf("  go %d: %s\n", i, e)
			}
		}
		if res.Same() {
			fmt.Println("  same")
			continue
		}
		differ++
		if res.Question != nil {
			fmt.Println("  unanswered: " + res.Question.String())
		}
		if res.Events != nil {
			fmt.Println("  events: " + res.Events.String())
		}
		if res.Err != nil {
			fmt.Printf("  error: %v\n", res.Err)
		}
	}
	if differ > 0 {
		return fmt.Errorf("replay: %d of %d games differ", differ, fs.NArg())
	}
	return nil
}

func replayOne(db *compile.DB, path string, turns int) (*replay.Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	log, err := replay.Read(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return replay.Run(db, log, turns)
}
