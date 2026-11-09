package core

import (
	"cmp"
	"errors"
	"slices"

	"github.com/hypatia-tile/glossary/internal/model"
)

// ErrNotFound means show matched no entry and no morpheme.
var ErrNotFound = errors.New("not found")

// ShowResult is every record a show reference matched, with its links
// resolved.
type ShowResult struct {
	Entries   []EntryView    `json:"entries,omitempty"`
	Morphemes []MorphemeView `json:"morphemes,omitempty"`
}

// Link is one resolved reference. IDs empty means it does not resolve yet;
// more than one means it is ambiguous.
type Link struct {
	Text string   `json:"text"`
	IDs  []string `json:"ids,omitempty"`
	// Inbound marks a link stored on the other record, pointing here.
	Inbound bool `json:"inbound,omitempty"`
}

type EntryView struct {
	Entry    *model.Entry `json:"entry"`
	Parts    []Link       `json:"parts,omitempty"`
	Synonyms []Link       `json:"synonyms,omitempty"`
	Antonyms []Link       `json:"antonyms,omitempty"`
	Related  []Link       `json:"related,omitempty"`
	PhraseOf []Link       `json:"phrase_of,omitempty"`
	// Phrases are the entries whose phrase_of points at this one.
	Phrases   []Link            `json:"phrases,omitempty"`
	SameRoot  []SameRoot        `json:"same_root,omitempty"`
	Citations []*model.Citation `json:"citations,omitempty"`
	// Tags counts the tags of this entry's citations, most frequent first.
	Tags []TagCount `json:"tags,omitempty"`
}

// SameRoot lists the other entries built on one of this entry's morphemes.
type SameRoot struct {
	Morpheme string `json:"morpheme"`
	Entries  []Link `json:"entries"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type MorphemeView struct {
	Morpheme *model.Morpheme `json:"morpheme"`
	// Entries are those whose parts name this morpheme.
	Entries []Link `json:"entries,omitempty"`
}

// Show finds what ref names — an entry ID or headword, a morpheme ID or form —
// and resolves everything linked to it.
func (env Env) Show(ref string) (*ShowResult, error) {
	snap, err := env.Store.Load()
	if err != nil {
		return nil, err
	}
	ix := newIndex(snap)
	res := &ShowResult{}
	for _, id := range ix.ResolveEntry(ref) {
		res.Entries = append(res.Entries, ix.entryView(id))
	}
	for _, id := range ix.ResolveMorpheme(ref) {
		res.Morphemes = append(res.Morphemes, MorphemeView{
			Morpheme: ix.Morphemes[id],
			Entries:  ix.entriesWithPart(id, ""),
		})
	}
	if len(res.Entries) == 0 && len(res.Morphemes) == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}

func (ix *Index) entryView(id string) EntryView {
	e := ix.Entries[id]
	v := EntryView{Entry: e}
	for _, r := range e.Parts {
		v.Parts = append(v.Parts, Link{Text: r, IDs: ix.ResolveMorpheme(r)})
	}

	out := map[string][]Link{}
	for _, f := range linkFields(e) {
		for _, r := range f.refs {
			out[f.name] = append(out[f.name], Link{Text: r, IDs: ix.ResolveEntry(r)})
		}
	}
	// Links are stored one way: add those stored on other entries. For
	// synonyms, antonyms and related the reverse is the same relation; the
	// reverse of phrase_of is phrases.
	for _, oid := range sortedKeys(ix.Entries) {
		if oid == id {
			continue
		}
		other := ix.Entries[oid]
		for _, f := range linkFields(other) {
			if !ix.anyResolvesTo(f.refs, id) {
				continue
			}
			back := Link{Text: other.Headword, IDs: []string{oid}, Inbound: true}
			if f.name == "phrase_of" {
				v.Phrases = append(v.Phrases, back)
			} else if !linksTo(out[f.name], oid) {
				out[f.name] = append(out[f.name], back)
			}
		}
	}
	v.Synonyms, v.Antonyms, v.Related, v.PhraseOf = out["synonyms"], out["antonyms"], out["related"], out["phrase_of"]

	var seen []string
	for _, p := range v.Parts {
		if len(p.IDs) != 1 || slices.Contains(seen, p.IDs[0]) {
			continue
		}
		seen = append(seen, p.IDs[0])
		if others := ix.entriesWithPart(p.IDs[0], id); others != nil {
			v.SameRoot = append(v.SameRoot, SameRoot{Morpheme: p.IDs[0], Entries: others})
		}
	}

	counts := map[string]int{}
	for _, cid := range sortedKeys(ix.Citations) {
		c := ix.Citations[cid]
		if !slices.ContainsFunc(c.Entries, func(l model.CitationLink) bool { return l.Ref == id }) {
			continue
		}
		v.Citations = append(v.Citations, c)
		for _, t := range c.Tags {
			counts[t]++
		}
	}
	slices.SortStableFunc(v.Citations, func(a, b *model.Citation) int { return cmp.Compare(a.Met, b.Met) })
	for _, t := range sortedKeys(counts) {
		v.Tags = append(v.Tags, TagCount{t, counts[t]})
	}
	slices.SortStableFunc(v.Tags, func(a, b TagCount) int { return cmp.Compare(b.Count, a.Count) })
	return v
}

// entriesWithPart lists the entries whose parts may mean morpheme mid,
// leaving out the entry except.
func (ix *Index) entriesWithPart(mid, except string) []Link {
	var out []Link
	for _, id := range sortedKeys(ix.Entries) {
		if id == except {
			continue
		}
		e := ix.Entries[id]
		if slices.ContainsFunc(e.Parts, func(r string) bool { return slices.Contains(ix.ResolveMorpheme(r), mid) }) {
			out = append(out, Link{Text: e.Headword, IDs: []string{id}})
		}
	}
	return out
}

func (ix *Index) anyResolvesTo(refs []string, id string) bool {
	return slices.ContainsFunc(refs, func(r string) bool { return slices.Contains(ix.ResolveEntry(r), id) })
}

func linksTo(links []Link, id string) bool {
	return slices.ContainsFunc(links, func(l Link) bool { return slices.Contains(l.IDs, id) })
}
