package store_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/store"
)

func manifest() store.Manifest {
	return store.Manifest{RunID: "r1", GitSHA: "abc", SchemaVersion: 1, MetricsVersion: 1, BaseSeed: 5, Games: 3, Shards: 2,
		Decks: []store.DeckInfo{{Name: "A", Hash: "h"}}}
}

type rec struct {
	N int `json:"n"`
}

func TestRowsRoundTripThroughShards(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run, err := store.Create(dir, manifest())
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		sh, err := run.Shard(n)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			if err := sh.Write("thing", rec{N: n*10 + i}); err != nil {
				t.Fatal(err)
			}
		}
		if err := sh.Close(); err != nil {
			t.Fatal(err)
		}
	}
	r, err := store.Open(dir, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Manifest(); got.RunID != "r1" || got.Shards != 2 || got.Decks[0].Name != "A" {
		t.Errorf("manifest %+v", got)
	}
	var seen []string
	if err := r.Each(func(row store.Row) error {
		seen = append(seen, row.Kind+":"+string(row.Data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 6 || seen[0] != `thing:{"n":0}` || seen[3] != `thing:{"n":10}` {
		t.Errorf("rows %v, want shard 0 then shard 1 in written order", seen)
	}
	kinds, err := r.Kinds()
	if err != nil || len(kinds) != 1 || kinds[0] != "thing" {
		t.Errorf("kinds %v, %v", kinds, err)
	}
}

func TestSameRowsWriteTheSameBytes(t *testing.T) {
	t.Parallel()
	write := func() []byte {
		dir := t.TempDir()
		run, err := store.Create(dir, manifest())
		if err != nil {
			t.Fatal(err)
		}
		sh, _ := run.Shard(0)
		for i := 0; i < 50; i++ {
			_ = sh.Write("x", rec{N: i})
		}
		if err := sh.Close(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "shards", "shard-000.ndjson.gz"))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if !bytes.Equal(write(), write()) {
		t.Error("two writes of the same rows differ byte for byte")
	}
}

func TestOpenRefusesAnotherVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := store.Create(dir, manifest()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(dir, 2, 1); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("wrong schema: err = %v", err)
	}
	if _, err := store.Open(dir, 1, 2); err == nil || !strings.Contains(err.Error(), "metrics") {
		t.Errorf("wrong metrics: err = %v", err)
	}
	if _, err := store.Open(dir, 0, 0); err != nil {
		t.Errorf("0 accepts any: %v", err)
	}
	if _, err := store.Open(t.TempDir(), 0, 0); err == nil {
		t.Error("a directory with no manifest opened")
	}
}

func TestCreateRefusesWhatItWouldOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run, err := store.Create(dir, manifest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(dir, manifest()); err == nil {
		t.Error("a second Create over the same run did not fail")
	}
	bad := manifest()
	bad.Shards = 0
	if _, err := store.Create(t.TempDir(), bad); err == nil {
		t.Error("zero shards did not fail")
	}
	if _, err := run.Shard(2); err == nil {
		t.Error("a shard past the manifest's count opened")
	}
	if sh, err := run.Shard(0); err != nil {
		t.Fatal(err)
	} else {
		_ = sh.Close()
	}
	if _, err := run.Shard(0); err == nil {
		t.Error("a shard opened twice")
	}
}

func TestEachStopsWithTheCallbackError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run, _ := store.Create(dir, manifest())
	sh, _ := run.Shard(0)
	_ = sh.Write("x", rec{N: 1})
	_ = sh.Write("x", rec{N: 2})
	_ = sh.Close()
	r, _ := store.Open(dir, 0, 0)
	stop := errors.New("stop")
	calls := 0
	err := r.Each(func(store.Row) error { calls++; return stop })
	if !errors.Is(err, stop) || calls != 1 {
		t.Errorf("err %v after %d calls, want stop after 1", err, calls)
	}
}

func TestShardOfIsIndependentOfWorkers(t *testing.T) {
	t.Parallel()
	if store.ShardOf(7, 4) != 3 || store.ShardOf(8, 4) != 0 {
		t.Error("ShardOf is not index modulo shards")
	}
}

func TestReadingAShardThatIsNotNDJSONFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := store.Create(dir, manifest()); err != nil {
		t.Fatal(err)
	}
	r, _ := store.Open(dir, 0, 0)
	shard := filepath.Join(dir, "shards", "shard-000.ndjson.gz")
	if err := os.WriteFile(shard, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Each(func(store.Row) error { return nil }); err == nil || !strings.Contains(err.Error(), "shard-000") {
		t.Errorf("a bad gzip: err = %v, want it to name the shard", err)
	}
	if _, err := r.Kinds(); err == nil {
		t.Error("Kinds over a bad shard did not fail")
	}
	// A gzip holding a line that is not JSON.
	_ = os.Remove(shard)
	run, _ := store.Create(t.TempDir(), manifest())
	sh, _ := run.Shard(0)
	_ = sh.Close()
	var buf bytes.Buffer
	gz := gzipWriter(&buf)
	_, _ = gz.Write([]byte("{oops\n"))
	_ = gz.Close()
	if err := os.WriteFile(shard, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Each(func(store.Row) error { return nil }); err == nil {
		t.Error("a line that is not JSON did not fail")
	}
}

func gzipWriter(w *bytes.Buffer) *gzip.Writer { return gzip.NewWriter(w) }

type unencodable struct{ C chan int }

func TestWriteRefusesWhatJSONCannotEncode(t *testing.T) {
	t.Parallel()
	run, err := store.Create(t.TempDir(), manifest())
	if err != nil {
		t.Fatal(err)
	}
	if got := run.Manifest().RunID; got != "r1" {
		t.Errorf("run id %q", got)
	}
	sh, _ := run.Shard(0)
	defer func() { _ = sh.Close() }()
	if err := sh.Write("x", unencodable{C: make(chan int)}); err == nil {
		t.Error("a channel was written")
	}
}

func TestCreateFailsWhereTheDirectoryCannotBeMade(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(filepath.Join(file, "run"), manifest()); err == nil {
		t.Error("Create under a file did not fail")
	}
}

func TestOpenRefusesABrokenManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(dir, 0, 0); err == nil {
		t.Error("a manifest that is not JSON opened")
	}
}
