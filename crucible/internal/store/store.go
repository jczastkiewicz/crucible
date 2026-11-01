// Package store writes and reads a run's telemetry on disk (ADR-0014):
// manifest.json plus gzipped NDJSON shards, standard library only.
//
//	runs/<run-id>/manifest.json
//	runs/<run-id>/shards/shard-000.ndjson.gz
//
// A shard has one writer and is immutable once closed. gzip headers carry no
// timestamp, so the same rows written in the same order give the same bytes.
package store

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// DeckInfo identifies one deck of a run.
type DeckInfo struct {
	Name string `json:"name"`
	// Hash is a digest of the decklist, so a manifest says which list was run.
	Hash string `json:"hash"`
}

// Manifest is what a run records about itself (ADR-0014): enough to re-run it
// and to refuse to compare it with an incompatible one.
type Manifest struct {
	RunID          string         `json:"run_id"`
	GitSHA         string         `json:"git_sha"`
	SchemaVersion  int            `json:"schema_version"`
	MetricsVersion int            `json:"metrics_version"`
	BaseSeed       int64          `json:"base_seed"`
	Games          int            `json:"games"`
	Shards         int            `json:"shards"`
	Gauntlet       string         `json:"gauntlet,omitempty"`
	Decks          []DeckInfo     `json:"decks"`
	Thresholds     map[string]int `json:"thresholds,omitempty"`
}

// Row is one stored record: its kind and the kind's JSON.
type Row struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// Run is a run directory being written.
type Run struct {
	dir      string
	manifest Manifest
}

// Create makes the run directory and writes its manifest. An existing
// manifest is an error: a run is never overwritten.
func Create(dir string, m Manifest) (*Run, error) {
	if m.Shards < 1 {
		return nil, errors.New("store: a run needs at least one shard")
	}
	if err := os.MkdirAll(filepath.Join(dir, "shards"), 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "manifest.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &Run{dir: dir, manifest: m}, nil
}

// Manifest is the manifest the run was created with.
func (r *Run) Manifest() Manifest { return r.manifest }

func shardPath(dir string, n int) string {
	return filepath.Join(dir, "shards", fmt.Sprintf("shard-%03d.ndjson.gz", n))
}

// Shard opens shard n for writing; n counts from 0 below Manifest.Shards.
func (r *Run) Shard(n int) (*Shard, error) {
	if n < 0 || n >= r.manifest.Shards {
		return nil, fmt.Errorf("store: shard %d is outside 0..%d", n, r.manifest.Shards-1)
	}
	f, err := os.OpenFile(shardPath(r.dir, n), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	gz := gzip.NewWriter(f)
	return &Shard{f: f, gz: gz, w: bufio.NewWriter(gz)}, nil
}

// Shard is one open shard. It is not safe for concurrent use: one writer owns
// it.
type Shard struct {
	f  *os.File
	gz *gzip.Writer
	w  *bufio.Writer
}

// Write appends one row of kind, its data the JSON of v.
func (s *Shard) Write(kind string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	line, err := json.Marshal(Row{Kind: kind, Data: data})
	if err != nil {
		return err
	}
	if _, err := s.w.Write(line); err != nil {
		return err
	}
	return s.w.WriteByte('\n')
}

// Close flushes and closes the shard.
func (s *Shard) Close() error {
	err := s.w.Flush()
	if e := s.gz.Close(); err == nil {
		err = e
	}
	if e := s.f.Close(); err == nil {
		err = e
	}
	return err
}

// Reader reads a finished run.
type Reader struct {
	dir      string
	manifest Manifest
}

// Open reads dir's manifest. A run whose schema or metrics version differs
// from the wanted ones is refused (ADR-0013, MET-1); pass 0 to accept any.
func Open(dir string, schema, metrics int) (*Reader, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("store: manifest: %w", err)
	}
	if schema != 0 && m.SchemaVersion != schema {
		return nil, fmt.Errorf("store: run has schema version %d, want %d", m.SchemaVersion, schema)
	}
	if metrics != 0 && m.MetricsVersion != metrics {
		return nil, fmt.Errorf("store: run has metrics version %d, want %d", m.MetricsVersion, metrics)
	}
	return &Reader{dir: dir, manifest: m}, nil
}

// Manifest is the run's manifest.
func (r *Reader) Manifest() Manifest { return r.manifest }

// Each streams every row of every shard, shards in name order and rows in the
// order written, holding one row in memory at a time. fn returning an error
// stops the walk with it.
func (r *Reader) Each(fn func(Row) error) error {
	names, err := filepath.Glob(filepath.Join(r.dir, "shards", "*.ndjson.gz"))
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		if err := eachRow(name, fn); err != nil {
			return fmt.Errorf("store: %s: %w", filepath.Base(name), err)
		}
	}
	return nil
}

func eachRow(name string, fn func(Row) error) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	br := bufio.NewReader(gz)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			var row Row
			if jerr := json.Unmarshal(line, &row); jerr != nil {
				return jerr
			}
			if ferr := fn(row); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// ShardOf is the shard a game's rows go to: its index modulo the shard count,
// independent of how many workers play.
func ShardOf(game, shards int) int { return game % shards }

// Kinds lists the distinct row kinds in a run, sorted; a convenience for
// tools that print what a run holds.
func (r *Reader) Kinds() ([]string, error) {
	seen := map[string]bool{}
	if err := r.Each(func(row Row) error { seen[row.Kind] = true; return nil }); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}
