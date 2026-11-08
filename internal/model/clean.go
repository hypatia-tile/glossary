package model

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// DateLayout is the format of every date field.
const DateLayout = "2006-01-02"

func (e *Entry) Clean() Errors {
	var errs Errors
	e.Headword = squash(e.Headword)
	if e.Headword == "" {
		errs.add("headword", "required")
	} else if Slug(e.Headword) == "" {
		errs.add("headword", "needs at least one letter or digit")
	}
	for i := range e.Senses {
		s := &e.Senses[i]
		s.POS = strings.ToLower(squash(s.POS))
		s.JA = strings.TrimSpace(s.JA)
		s.EN = strings.TrimSpace(s.EN)
		s.Usage = strings.TrimSpace(s.Usage)
		if s.POS == "" && s.JA == "" && s.EN == "" {
			errs.add(fmt.Sprintf("senses[%d]", i), "needs at least one of pos, ja, en")
		}
	}
	e.Etymology = strings.TrimSpace(e.Etymology)
	e.Parts = cleanRefs(&errs, "parts", e.Parts)
	for _, f := range []struct {
		name string
		refs *[]string
	}{
		{"synonyms", &e.Synonyms},
		{"antonyms", &e.Antonyms},
		{"related", &e.Related},
		{"phrase_of", &e.PhraseOf},
	} {
		*f.refs = cleanRefs(&errs, f.name, *f.refs)
		for i, r := range *f.refs {
			if strings.EqualFold(r, e.Headword) || (e.ID != "" && r == e.ID) {
				errs.add(fmt.Sprintf("%s[%d]", f.name, i), "refers to the entry itself")
			}
		}
	}
	checkDate(&errs, "created", e.Created)
	return errs
}

func (m *Morpheme) Clean() Errors {
	var errs Errors
	switch m.Type {
	case Prefix, Suffix, Root:
	case "":
		errs.add("type", `required: one of "prefix", "suffix", "root"`)
	default:
		errs.add("type", `unknown type %q: want "prefix", "suffix" or "root"`, m.Type)
	}
	m.Form = cleanForm(m.Form)
	if m.Form == "" {
		errs.add("form", "required")
	} else if !isForm(m.Form) {
		errs.add("form", "%q: only lowercase letters", m.Form)
	}
	var variants []string
	for i, v := range m.Variants {
		v = cleanForm(v)
		switch {
		case v == "":
			errs.add(fmt.Sprintf("variants[%d]", i), "empty")
		case !isForm(v):
			errs.add(fmt.Sprintf("variants[%d]", i), "%q: only lowercase letters", v)
		case v != m.Form && !slices.Contains(variants, v):
			variants = append(variants, v)
		}
	}
	m.Variants = variants
	m.Meaning = strings.TrimSpace(m.Meaning)
	m.Origin = strings.TrimSpace(m.Origin)
	checkDate(&errs, "created", m.Created)
	return errs
}

func (q *Quote) Clean() Errors {
	var errs Errors
	// Copied text often carries line breaks and double spaces; a quote is
	// one line.
	q.Text = squash(q.Text)
	if q.Text == "" {
		errs.add("text", "required")
	}
	q.URL = strings.TrimSpace(q.URL)
	if q.URL != "" {
		if u, err := url.Parse(q.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs.add("url", "%q: want an http or https URL", q.URL)
		}
	}
	var tags []string
	for i, t := range q.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		switch {
		case !tagPattern.MatchString(t):
			errs.add(fmt.Sprintf("tags[%d]", i), "%q: want a tag like \"paper\" or \"genre:paper\" (lowercase, no spaces)", t)
		case !slices.Contains(tags, t):
			tags = append(tags, t)
		}
	}
	q.Tags = tags
	checkDate(&errs, "met", q.Met)
	if len(q.Entries) == 0 {
		errs.add("entries", "at least one entry is required")
	}
	var seen []string
	for i := range q.Entries {
		l := &q.Entries[i]
		l.Ref = squash(l.Ref)
		l.Form = squash(l.Form)
		path := fmt.Sprintf("entries[%d]", i)
		switch {
		case l.Ref == "":
			errs.add(path+".ref", "required")
		case slices.Contains(seen, strings.ToLower(l.Ref)):
			errs.add(path+".ref", "%q is linked twice", l.Ref)
		default:
			seen = append(seen, strings.ToLower(l.Ref))
		}
	}
	checkDate(&errs, "created", q.Created)
	return errs
}

// Slug turns a headword into the stem of an entry ID: lowercase letters and
// digits, with every other run of characters made one hyphen.
func Slug(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
		} else {
			hyphen = true
		}
	}
	return b.String()
}

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*(:[a-z0-9][a-z0-9._/-]*)?$`)

// squash trims s and collapses every run of whitespace to one space.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// cleanForm accepts a morpheme written as "in-", "-ion" or "spect" and keeps
// only the letters; the type field says where it goes.
func cleanForm(s string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(s)), "-")
}

func isForm(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

// cleanRefs squashes each reference, reports empty ones and drops repeats.
func cleanRefs(errs *Errors, field string, refs []string) []string {
	var out []string
	for i, r := range refs {
		r = squash(r)
		if r == "" {
			errs.add(fmt.Sprintf("%s[%d]", field, i), "empty")
			continue
		}
		if !containsFold(out, r) {
			out = append(out, r)
		}
	}
	return out
}

func checkDate(errs *Errors, path, s string) {
	if s == "" {
		return
	}
	if _, err := time.Parse(DateLayout, s); err != nil {
		errs.add(path, "%q: want a date like 2026-11-08", s)
	}
}

func containsFold(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
