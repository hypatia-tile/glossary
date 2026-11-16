package store

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hypatia-tile/glossary/internal/model"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestJSONFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b-1.json"), "{}")
	write(t, filepath.Join(dir, "a-1.json"), "{}")
	write(t, filepath.Join(dir, "notes.txt"), "")
	write(t, filepath.Join(dir, "sub", "c-1.json"), "{}")                   // not descended into
	if err := os.Mkdir(filepath.Join(dir, "d-1.json"), 0o755); err != nil { // a directory, not a file
		t.Fatal(err)
	}

	got, err := jsonFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a-1.json"), filepath.Join(dir, "b-1.json")}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if got, err := jsonFiles(filepath.Join(dir, "missing")); got != nil || err != nil {
		t.Errorf("missing directory: got %v, %v; want an empty list", got, err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "entries", "bank-1.json"),
		`{"kind":"entry","id":"bank-1","headword":"bank","created":"2026-11-16"}`)
	write(t, filepath.Join(dir, "entries", "broken-1.json"), `{"kind":"entry",`)
	write(t, filepath.Join(dir, "entries", "root-spect.json"),
		`{"kind":"morpheme","id":"root-spect","type":"root","form":"spect"}`)
	write(t, filepath.Join(dir, "entries", "other-1.json"),
		`{"kind":"entry","id":"bank-1","headword":"bank"}`)
	write(t, filepath.Join(dir, "morphemes", "README.md"), "not a record")
	// No citations/ directory at all: that is an empty kind, not an error.

	snap, err := Files{Dir: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Records) != 1 || snap.Records[0].RecordID() != "bank-1" {
		t.Errorf("records: got %v, want only bank-1", snap.Records)
	}
	if _, ok := snap.Records[0].(*model.Entry); !ok {
		t.Errorf("bank-1 loaded as %T", snap.Records[0])
	}

	want := map[string]string{
		"broken-1.json":   "", // any decode error
		"other-1.json":    "does not match the file name",
		"root-spect.json": "a morpheme record in entries/",
	}
	if len(snap.Problems) != len(want) {
		t.Fatalf("problems: got %v, want %d", snap.Problems, len(want))
	}
	for _, p := range snap.Problems {
		substr, ok := want[filepath.Base(p.Location)]
		if !ok || !strings.Contains(p.Message, substr) {
			t.Errorf("unexpected problem %+v", p)
		}
	}
}

func TestSaveThenLoad(t *testing.T) {
	f := Files{Dir: t.TempDir()}
	e := &model.Entry{Kind: model.KindEntry, ID: "bank-1", Headword: "bank", Created: "2026-11-16"}
	if err := f.Save(e); err != nil {
		t.Fatal(err)
	}
	snap, err := f.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Problems) != 0 || len(snap.Records) != 1 || snap.Records[0].(*model.Entry).Headword != "bank" {
		t.Errorf("got %+v", snap)
	}
	if err := f.Save(&model.Entry{Kind: model.KindEntry, ID: "../escape", Headword: "x"}); err == nil {
		t.Error("an id with a path in it was saved")
	}
}
