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
// <Dir>/morphemes/<id>.json and <Dir>/citations/<id>.json.
type Files struct {
	Dir string
}

var dirs = map[model.Kind]string{
	model.KindEntry:    "entries",
	model.KindMorpheme: "morphemes",
	model.KindCitation: "citations",
}

// idPattern keeps IDs usable as file names on every system.
var idPattern = regexp.MustCompile(`^[\p{Ll}\p{Lo}\p{N}][\p{Ll}\p{Lo}\p{N}-]*$`)

// kinds is the order Load reads the kind directories in.
var kinds = []model.Kind{model.KindEntry, model.KindMorpheme, model.KindCitation}

func (f Files) Load() (*Snapshot, error) {
	snap := &Snapshot{}
	for _, kind := range kinds {
		paths, err := jsonFiles(filepath.Join(f.Dir, dirs[kind]))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			r, p, err := loadFile(path, kind)
			switch {
			case err != nil:
				return nil, err
			case p != nil:
				snap.Problems = append(snap.Problems, *p)
			default:
				snap.Records = append(snap.Records, r)
			}
		}
	}
	return snap, nil
}

// jsonFiles lists the record files in one kind's directory, in name order.
// The layout is flat, so subdirectories are skipped, not descended into. A
// missing directory is an empty list.
func jsonFiles(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, de := range des {
		if !de.IsDir() && strings.HasSuffix(de.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, de.Name()))
		}
	}
	return paths, nil
}

// loadFile reads one record file expected to hold a record of kind. A file
// that cannot be read is a fatal error; one that reads but is not a valid
// record of that kind, under its own ID, is a Problem.
func loadFile(path string, kind model.Kind) (model.Record, *Problem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	r, err := model.Decode(data)
	switch {
	case err != nil:
		return nil, &Problem{path, err.Error()}, nil
	case r.RecordKind() != kind:
		return nil, &Problem{path, fmt.Sprintf("a %s record in %s/", r.RecordKind(), dirs[kind])}, nil
	case r.RecordID()+".json" != filepath.Base(path):
		return nil, &Problem{path, fmt.Sprintf("id %q does not match the file name", r.RecordID())}, nil
	}
	return r, nil, nil
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
