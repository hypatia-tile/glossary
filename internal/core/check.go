package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hypatia-tile/glossary/internal/model"
)

// Problem is one thing check found. Errors are records the tool would not
// have written; warnings are references that do not resolve yet, which is
// allowed.
type Problem struct {
	Severity string `json:"severity"` // "error" or "warning"
	Where    string `json:"where"`    // a record ID, or a file the store could not read
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

const (
	SevError   = "error"
	SevWarning = "warning"
)

var quoteIDPattern = regexp.MustCompile(`^q-[0-9]{8}-[0-9a-f]{6}$`)

// Check validates every stored record and the links between them.
func (env Env) Check() ([]Problem, error) {
	snap, err := env.Store.Load()
	if err != nil {
		return nil, err
	}
	var ps []Problem
	add := func(sev, where, path, msg string) {
		ps = append(ps, Problem{sev, where, path, msg})
	}
	for _, p := range snap.Problems {
		add(SevError, p.Location, "", p.Message)
	}
	ix := newIndex(snap)

	for _, r := range snap.Records {
		id := r.RecordID()
		before, _ := json.Marshal(r)
		for _, e := range r.Clean() {
			add(SevError, id, e.Path, e.Message)
		}
		if after, _ := json.Marshal(r); string(after) != string(before) {
			add(SevWarning, id, "", "not stored in normal form; putting it again would rewrite it")
		}
		created := ""
		switch r := r.(type) {
		case *model.Entry:
			created = r.Created
			if stem, _ := splitEntryID(id); stem != model.Slug(r.Headword) {
				add(SevError, id, "id", fmt.Sprintf("does not fit the headword %q", r.Headword))
			}
			for _, w := range refWarnings(ix, r) {
				add(SevWarning, id, "", w)
			}
		case *model.Morpheme:
			created = r.Created
			if id != r.DerivedID() {
				add(SevError, id, "id", fmt.Sprintf("want %q, from type and form", r.DerivedID()))
			}
		case *model.Quote:
			created = r.Created
			if !quoteIDPattern.MatchString(id) {
				add(SevError, id, "id", "want an id like q-20261108-a3f09c")
			}
			for i, l := range r.Entries {
				path := fmt.Sprintf("entries[%d]", i)
				e, ok := ix.Entries[l.Ref]
				if !ok {
					add(SevError, id, path+".ref", fmt.Sprintf("no entry %q", l.Ref))
					continue
				}
				if msg := formProblem(r.Text, e.Headword, l.Form); msg != "" {
					add(SevError, id, path+".form", msg)
				}
			}
		}
		if created == "" {
			add(SevError, id, "created", "missing")
		}
	}
	slices.SortStableFunc(ps, func(a, b Problem) int {
		if a.Severity != b.Severity {
			return strings.Compare(a.Severity, b.Severity) // errors first
		}
		return strings.Compare(a.Where, b.Where)
	})
	return ps, nil
}

// formProblem checks that the linked word is in a quote's text: form if
// given, else the headword. The check is a plain substring match ignoring
// case, so "practitioner" is found in "practitioners".
func formProblem(text, headword, form string) string {
	text = strings.ToLower(text)
	if form == "" {
		if !strings.Contains(text, strings.ToLower(headword)) {
			return fmt.Sprintf("%q does not appear in the text; set form to the word as written there", headword)
		}
	} else if !strings.Contains(text, strings.ToLower(form)) {
		return fmt.Sprintf("%q does not appear in the text", form)
	}
	return ""
}
