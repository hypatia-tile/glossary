// Package core is what the commands do: put a record, show an entry, check
// the store. It resolves references between records, which package model
// cannot do on its own.
package core

import (
	"slices"
	"strings"

	"github.com/hypatia-tile/glossary/internal/model"
	"github.com/hypatia-tile/glossary/internal/store"
)

// Index holds every loaded record, with lookups for resolving references.
type Index struct {
	Entries   map[string]*model.Entry
	Morphemes map[string]*model.Morpheme
	Quotes    map[string]*model.Quote

	byHeadword map[string][]string // lowercased headword → entry IDs
	byForm     map[string][]string // form or variant → morpheme IDs
}

func newIndex(snap *store.Snapshot) *Index {
	ix := &Index{
		Entries:    map[string]*model.Entry{},
		Morphemes:  map[string]*model.Morpheme{},
		Quotes:     map[string]*model.Quote{},
		byHeadword: map[string][]string{},
		byForm:     map[string][]string{},
	}
	for _, r := range snap.Records {
		ix.add(r)
	}
	return ix
}

func (ix *Index) add(r model.Record) {
	switch r := r.(type) {
	case *model.Entry:
		ix.Entries[r.ID] = r
		key := strings.ToLower(r.Headword)
		ix.byHeadword[key] = sortedInsert(ix.byHeadword[key], r.ID)
	case *model.Morpheme:
		ix.Morphemes[r.ID] = r
		for _, f := range append([]string{r.Form}, r.Variants...) {
			ix.byForm[f] = sortedInsert(ix.byForm[f], r.ID)
		}
	case *model.Quote:
		ix.Quotes[r.ID] = r
	}
}

// ResolveEntry returns the IDs an entry reference may mean: the entry with
// that ID, or else every entry with that headword. None means unresolved;
// more than one means ambiguous.
func (ix *Index) ResolveEntry(ref string) []string {
	if _, ok := ix.Entries[ref]; ok {
		return []string{ref}
	}
	return ix.byHeadword[strings.ToLower(ref)]
}

// ResolveMorpheme does the same for a morpheme reference: an ID, or a form
// whose hyphens say its type ("in-" a prefix, "-ion" a suffix). A form with
// no hyphen matches any type.
func (ix *Index) ResolveMorpheme(ref string) []string {
	if _, ok := ix.Morphemes[ref]; ok {
		return []string{ref}
	}
	s := strings.ToLower(ref)
	var want model.MorphemeType
	switch {
	case strings.HasSuffix(s, "-") && !strings.HasPrefix(s, "-"):
		want = model.Prefix
	case strings.HasPrefix(s, "-") && !strings.HasSuffix(s, "-"):
		want = model.Suffix
	}
	var out []string
	for _, id := range ix.byForm[strings.Trim(s, "-")] {
		if want == "" || ix.Morphemes[id].Type == want {
			out = append(out, id)
		}
	}
	return out
}

func sortedInsert(xs []string, s string) []string {
	i, found := slices.BinarySearch(xs, s)
	if found {
		return xs
	}
	return slices.Insert(xs, i, s)
}

// sortedKeys returns a map's keys in order, so output never depends on map
// iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
