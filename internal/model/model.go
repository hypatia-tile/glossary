// Package model defines the three record kinds the glossary stores — entries,
// morphemes and quotes — and how each is decoded, normalised and validated.
//
// Nothing here knows about other records: whether a reference resolves is
// decided by package core, which sees the whole store.
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Kind names a record kind. It is stored in every record as "kind".
type Kind string

const (
	KindEntry    Kind = "entry"
	KindMorpheme Kind = "morpheme"
	KindQuote    Kind = "quote"
)

// Record is implemented by *Entry, *Morpheme and *Quote.
type Record interface {
	RecordKind() Kind
	RecordID() string
	// Clean normalises the record in place and reports what is invalid.
	Clean() Errors
}

// Entry is one word, phrase or idiom with one etymology. A spelling with two
// unrelated origins ("bank" of a river, "bank" for money) is two entries.
type Entry struct {
	Kind      Kind    `json:"kind"`
	ID        string  `json:"id,omitempty"`
	Headword  string  `json:"headword"`
	Senses    []Sense `json:"senses,omitempty"`
	Etymology string  `json:"etymology,omitempty"`
	// Parts are references to morphemes: an ID ("root-spect") or a form
	// ("in-", "spect", "-ion").
	Parts []string `json:"parts,omitempty"`
	// Synonyms, Antonyms, Related and PhraseOf are references to entries: an
	// ID ("bank-2") or a headword ("omnipresent"). They are stored one way;
	// the reverse direction is worked out when shown.
	Synonyms []string `json:"synonyms,omitempty"`
	Antonyms []string `json:"antonyms,omitempty"`
	Related  []string `json:"related,omitempty"`
	PhraseOf []string `json:"phrase_of,omitempty"`
	Created  string   `json:"created,omitempty"`
}

// Sense is one meaning of an entry. Senses are shown in the order stored.
type Sense struct {
	POS   string `json:"pos,omitempty"`
	JA    string `json:"ja,omitempty"`
	EN    string `json:"en,omitempty"`
	Usage string `json:"usage,omitempty"`
}

// MorphemeType says where a morpheme sits in a word.
type MorphemeType string

const (
	Prefix MorphemeType = "prefix"
	Suffix MorphemeType = "suffix"
	Root   MorphemeType = "root"
)

// Morpheme is a prefix, suffix or root, defined once and referenced from the
// parts of entries. Its ID is derived from its type and form.
type Morpheme struct {
	Kind     Kind         `json:"kind"`
	ID       string       `json:"id,omitempty"`
	Type     MorphemeType `json:"type"`
	Form     string       `json:"form"`
	Variants []string     `json:"variants,omitempty"`
	Meaning  string       `json:"meaning,omitempty"`
	Origin   string       `json:"origin,omitempty"`
	Created  string       `json:"created,omitempty"`
}

// Quote is a sentence met somewhere, linked to the entries it was kept for.
type Quote struct {
	Kind    Kind        `json:"kind"`
	ID      string      `json:"id,omitempty"`
	Text    string      `json:"text"`
	URL     string      `json:"url,omitempty"`
	Tags    []string    `json:"tags,omitempty"`
	Met     string      `json:"met,omitempty"`
	Entries []QuoteLink `json:"entries"`
	Created string      `json:"created,omitempty"`
}

// QuoteLink ties a quote to one entry. Form is the word as written in the
// text ("ran into" for "run into"); empty means the headword itself.
type QuoteLink struct {
	Ref  string `json:"ref"`
	Form string `json:"form,omitempty"`
}

func (e *Entry) RecordKind() Kind    { return KindEntry }
func (e *Entry) RecordID() string    { return e.ID }
func (m *Morpheme) RecordKind() Kind { return KindMorpheme }
func (m *Morpheme) RecordID() string { return m.ID }
func (q *Quote) RecordKind() Kind    { return KindQuote }
func (q *Quote) RecordID() string    { return q.ID }

// DerivedID is the ID a morpheme must have: "<type>-<form>".
func (m *Morpheme) DerivedID() string { return string(m.Type) + "-" + m.Form }

// FieldError is one problem with a record, located by a JSON path such as
// "senses[1].pos". An empty path means the record as a whole.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Errors is a list of field errors; nil means valid.
type Errors []FieldError

func (es Errors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		if e.Path == "" {
			parts[i] = e.Message
		} else {
			parts[i] = e.Path + ": " + e.Message
		}
	}
	return strings.Join(parts, "; ")
}

func (es *Errors) add(path, format string, args ...any) {
	*es = append(*es, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
}

// Decode reads one record from JSON. The "kind" field picks the type, and
// any field that type does not have is an error.
func Decode(data []byte) (Record, error) {
	var head struct {
		Kind Kind `json:"kind"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, Errors{{Message: "not a JSON object: " + err.Error()}}
	}
	var r Record
	switch head.Kind {
	case KindEntry:
		r = &Entry{}
	case KindMorpheme:
		r = &Morpheme{}
	case KindQuote:
		r = &Quote{}
	case "":
		return nil, Errors{{Path: "kind", Message: `required: one of "entry", "morpheme", "quote"`}}
	default:
		return nil, Errors{{Path: "kind", Message: fmt.Sprintf(`unknown kind %q: want "entry", "morpheme" or "quote"`, head.Kind)}}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(r); err != nil {
		return nil, Errors{{Message: err.Error()}}
	}
	if dec.More() {
		return nil, Errors{{Message: "more than one JSON value"}}
	}
	return r, nil
}
