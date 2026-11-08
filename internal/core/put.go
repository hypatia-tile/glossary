package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hypatia-tile/glossary/internal/model"
	"github.com/hypatia-tile/glossary/internal/store"
)

// Env is what the commands run against. Now and Rand are fields so tests can
// fix the date and the quote IDs.
type Env struct {
	Store store.Store
	Now   func() time.Time
	Rand  io.Reader
}

// PutResult is the outcome of a put, written to stdout as JSON. With OK
// false, Errors says why and nothing was written.
type PutResult struct {
	OK     bool         `json:"ok"`
	DryRun bool         `json:"dry_run"`
	Action string       `json:"action,omitempty"` // "create" or "update"
	ID     string       `json:"id,omitempty"`
	Record model.Record `json:"record,omitempty"`
	// Stubs are entries created because a quote linked a headword that had
	// no entry yet. They hold only the headword.
	Stubs    []*model.Entry `json:"stubs,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Errors   model.Errors   `json:"errors,omitempty"`
}

// Put validates one record given as JSON and, unless dryRun, saves it. A
// record without an id is created and given one; a record with an id
// replaces the stored record with that id, which must exist.
func (env Env) Put(data []byte, dryRun bool) (*PutResult, error) {
	res := &PutResult{DryRun: dryRun}
	r, err := model.Decode(data)
	var errs model.Errors
	if errors.As(err, &errs) {
		res.Errors = errs
		return res, nil
	} else if err != nil {
		return nil, err
	}
	if errs := r.Clean(); errs != nil {
		res.Errors = errs
		return res, nil
	}
	snap, err := env.Store.Load()
	if err != nil {
		return nil, err
	}
	ix := newIndex(snap)
	switch r := r.(type) {
	case *model.Entry:
		errs = env.putEntry(ix, r, res)
	case *model.Morpheme:
		errs = env.putMorpheme(ix, r, res)
	case *model.Quote:
		errs = env.putQuote(ix, r, res)
	}
	if errs != nil {
		res.Errors = errs
		res.Action = ""
		res.Stubs = nil
		return res, nil
	}
	res.OK, res.ID, res.Record = true, r.RecordID(), r
	if dryRun {
		return res, nil
	}
	var recs []model.Record
	for _, s := range res.Stubs {
		recs = append(recs, s)
	}
	return res, env.Store.Save(append(recs, r)...)
}

func (env Env) today() string { return env.Now().Format(model.DateLayout) }

func (env Env) putEntry(ix *Index, e *model.Entry, res *PutResult) model.Errors {
	if e.ID == "" {
		if e.Created != "" {
			return model.Errors{{Path: "created", Message: "set by the tool; leave it out"}}
		}
		if same := ix.byHeadword[strings.ToLower(e.Headword)]; len(same) > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"entries with this headword already exist (%s); to add a sense to one of them, put that entry with its id instead",
				strings.Join(same, ", ")))
		}
		e.ID = ix.nextEntryID(e.Headword)
		e.Created = env.today()
		res.Action = "create"
	} else {
		old, ok := ix.Entries[e.ID]
		if !ok {
			return model.Errors{{Path: "id", Message: fmt.Sprintf("no entry %q; leave id out to create one", e.ID)}}
		}
		if stem, _ := splitEntryID(e.ID); model.Slug(e.Headword) != stem {
			return model.Errors{{Path: "headword", Message: fmt.Sprintf(
				"%q does not fit the id %q; renaming an entry is not supported yet", e.Headword, e.ID)}}
		}
		if errs := keepCreated(&e.Created, old.Created); errs != nil {
			return errs
		}
		res.Action = "update"
	}
	res.Warnings = append(res.Warnings, refWarnings(ix, e)...)
	return nil
}

func (env Env) putMorpheme(ix *Index, m *model.Morpheme, res *PutResult) model.Errors {
	derived := m.DerivedID()
	if m.ID == "" {
		if _, ok := ix.Morphemes[derived]; ok {
			return model.Errors{{Message: fmt.Sprintf("morpheme %q already exists; put it with \"id\" to update it", derived)}}
		}
		if m.Created != "" {
			return model.Errors{{Path: "created", Message: "set by the tool; leave it out"}}
		}
		m.ID = derived
		m.Created = env.today()
		res.Action = "create"
		return nil
	}
	if m.ID != derived {
		return model.Errors{{Path: "id", Message: fmt.Sprintf(
			"type and form make the id (%q), so they cannot change on an update", derived)}}
	}
	old, ok := ix.Morphemes[m.ID]
	if !ok {
		return model.Errors{{Path: "id", Message: fmt.Sprintf("no morpheme %q; leave id out to create one", m.ID)}}
	}
	if errs := keepCreated(&m.Created, old.Created); errs != nil {
		return errs
	}
	res.Action = "update"
	return nil
}

func (env Env) putQuote(ix *Index, q *model.Quote, res *PutResult) model.Errors {
	if q.ID == "" {
		if q.Created != "" {
			return model.Errors{{Path: "created", Message: "set by the tool; leave it out"}}
		}
		id, err := env.newQuoteID(ix)
		if err != nil {
			return model.Errors{{Message: err.Error()}}
		}
		q.ID = id
		q.Created = env.today()
		res.Action = "create"
	} else {
		old, ok := ix.Quotes[q.ID]
		if !ok {
			return model.Errors{{Path: "id", Message: fmt.Sprintf("no quote %q; leave id out to create one", q.ID)}}
		}
		if errs := keepCreated(&q.Created, old.Created); errs != nil {
			return errs
		}
		res.Action = "update"
	}
	if q.Met == "" {
		q.Met = q.Created
	}

	var errs model.Errors
	linked := map[string]bool{}
	for i := range q.Entries {
		l := &q.Entries[i]
		path := fmt.Sprintf("entries[%d]", i)
		var id string
		switch ids := ix.ResolveEntry(l.Ref); len(ids) {
		case 0:
			if looksLikeEntryID(l.Ref) {
				errs = append(errs, model.FieldError{Path: path + ".ref", Message: fmt.Sprintf(
					"no entry %q; to link a new headword, write the headword", l.Ref)})
				continue
			}
			stub := &model.Entry{Kind: model.KindEntry, Headword: l.Ref}
			if serrs := stub.Clean(); serrs != nil {
				errs = append(errs, model.FieldError{Path: path + ".ref", Message: serrs.Error()})
				continue
			}
			stub.ID = ix.nextEntryID(stub.Headword)
			stub.Created = env.today()
			ix.add(stub)
			res.Stubs = append(res.Stubs, stub)
			id = stub.ID
		case 1:
			id = ids[0]
		default:
			errs = append(errs, model.FieldError{Path: path + ".ref", Message: fmt.Sprintf(
				"%q matches %s; write the id to pick one", l.Ref, strings.Join(ids, ", "))})
			continue
		}
		if linked[id] {
			errs = append(errs, model.FieldError{Path: path + ".ref", Message: fmt.Sprintf("%s is linked twice", id)})
			continue
		}
		linked[id] = true
		// The word must be in the sentence; this catches a typo in either.
		if msg := formProblem(q.Text, ix.Entries[id].Headword, l.Form); msg != "" {
			errs = append(errs, model.FieldError{Path: path + ".form", Message: msg})
		}
		// Store the ID: a headword that is unique today may not stay unique.
		l.Ref = id
	}
	return errs
}

// keepCreated carries the stored creation date over to an update.
func keepCreated(created *string, old string) model.Errors {
	if *created == "" {
		*created = old
	} else if *created != old {
		return model.Errors{{Path: "created", Message: fmt.Sprintf("set by the tool; it is %q", old)}}
	}
	return nil
}

// nextEntryID gives a headword the ID <slug>-<n>, with n one past the
// highest already used for that slug.
func (ix *Index) nextEntryID(headword string) string {
	stem := model.Slug(headword)
	n := 0
	for id := range ix.Entries {
		if s, k := splitEntryID(id); s == stem && k > n {
			n = k
		}
	}
	return stem + "-" + strconv.Itoa(n+1)
}

var entryIDPattern = regexp.MustCompile(`^(.+)-([0-9]+)$`)

// splitEntryID splits "run-into-2" into "run-into" and 2.
func splitEntryID(id string) (string, int) {
	m := entryIDPattern.FindStringSubmatch(id)
	if m == nil {
		return "", 0
	}
	n, _ := strconv.Atoi(m[2])
	return m[1], n
}

func looksLikeEntryID(ref string) bool {
	return model.Slug(ref) == ref && entryIDPattern.MatchString(ref)
}

func (env Env) newQuoteID(ix *Index) (string, error) {
	prefix := "q-" + env.Now().Format("20060102") + "-"
	b := make([]byte, 3)
	for range 10 {
		if _, err := io.ReadFull(env.Rand, b); err != nil {
			return "", err
		}
		if id := prefix + hex.EncodeToString(b); ix.Quotes[id] == nil {
			return id, nil
		}
	}
	return "", errors.New("could not find a free quote id")
}

// refWarnings lists an entry's references that do not resolve to exactly one
// record. They are allowed — the record may be added later — but worth
// knowing about.
func refWarnings(ix *Index, e *model.Entry) []string {
	var out []string
	note := func(path, ref, what string, ids []string) {
		switch len(ids) {
		case 0:
			out = append(out, fmt.Sprintf("%s %q: no %s yet", path, ref, what))
		case 1:
		default:
			out = append(out, fmt.Sprintf("%s %q: matches %s; write the id to pick one", path, ref, strings.Join(ids, ", ")))
		}
	}
	for i, r := range e.Parts {
		note(fmt.Sprintf("parts[%d]", i), r, "morpheme", ix.ResolveMorpheme(r))
	}
	for _, f := range linkFields(e) {
		for i, r := range f.refs {
			note(fmt.Sprintf("%s[%d]", f.name, i), r, "entry", ix.ResolveEntry(r))
		}
	}
	return out
}

type linkField struct {
	name string
	refs []string
}

// linkFields lists an entry's references to other entries, in a fixed order.
func linkFields(e *model.Entry) []linkField {
	return []linkField{
		{"synonyms", e.Synonyms},
		{"antonyms", e.Antonyms},
		{"related", e.Related},
		{"phrase_of", e.PhraseOf},
	}
}
