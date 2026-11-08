// Package store keeps records. The rest of the program sees only the Store
// interface, so the files used today can be replaced by a database later.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hypatia-tile/glossary/internal/model"
)

// Store loads every record at once and saves records one by one.
type Store interface {
	Load() (*Snapshot, error)
	// Save writes each record, replacing any record with the same kind and ID.
	Save(recs ...model.Record) error
}

// Snapshot is everything a Load found: the records it could read, and a
// problem for each one it could not.
type Snapshot struct {
	Records  []model.Record
	Problems []Problem
}

// Problem is a stored record that could not be loaded.
type Problem struct {
	Location string // a file path, for the file store
	Message  string
}

// Files stores one JSON file per record: <Dir>/entries/<id>.json,
// <Dir>/morphemes/<id>.json and <Dir>/quotes/<id>.json.
type Files struct {
	Dir string
}

var dirs = map[model.Kind]string{
	model.KindEntry:    "entries",
	model.KindMorpheme: "morphemes",
	model.KindQuote:    "quotes",
}

// idPattern keeps IDs usable as file names on every system.
var idPattern = regexp.MustCompile(`^[\p{Ll}\p{Lo}\p{N}][\p{Ll}\p{Lo}\p{N}-]*$`)

func (f Files) Load() (*Snapshot, error) {
	snap := &Snapshot{}
	for _, kind := range []model.Kind{model.KindEntry, model.KindMorpheme, model.KindQuote} {
		dir := filepath.Join(f.Dir, dirs[kind])
		names, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, de := range names {
			if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, de.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			r, err := model.Decode(data)
			switch {
			case err != nil:
				snap.Problems = append(snap.Problems, Problem{path, err.Error()})
			case r.RecordKind() != kind:
				snap.Problems = append(snap.Problems, Problem{path, fmt.Sprintf("a %s record in %s/", r.RecordKind(), dirs[kind])})
			case r.RecordID()+".json" != de.Name():
				snap.Problems = append(snap.Problems, Problem{path, fmt.Sprintf("id %q does not match the file name", r.RecordID())})
			default:
				snap.Records = append(snap.Records, r)
			}
		}
	}
	return snap, nil
}

func (f Files) Save(recs ...model.Record) error {
	for _, r := range recs {
		if !idPattern.MatchString(r.RecordID()) {
			return fmt.Errorf("store: %q cannot be used as an id", r.RecordID())
		}
	}
	for _, r := range recs {
		if err := f.save(r); err != nil {
			return err
		}
	}
	return nil
}

func (f Files) save(r model.Record) error {
	dir := filepath.Join(f.Dir, dirs[r.RecordKind()])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// Write beside the target and rename, so a crash never leaves half a file.
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, r.RecordID()+".json"))
}
