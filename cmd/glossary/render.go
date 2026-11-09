package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/hypatia-tile/glossary/internal/core"
)

// renderShow prints a show result for a person to read. The JSON form
// (--json) carries the same data for programs.
func renderShow(w io.Writer, res *core.ShowResult) {
	first := true
	sep := func() {
		if !first {
			fmt.Fprintln(w)
		}
		first = false
	}
	for _, v := range res.Entries {
		sep()
		renderEntry(w, v)
	}
	for _, v := range res.Morphemes {
		sep()
		m := v.Morpheme
		fmt.Fprintf(w, "%s  %s (%s)\n", m.ID, displayForm(string(m.Type), m.Form), m.Type)
		field(w, "variants", strings.Join(m.Variants, ", "))
		field(w, "meaning", m.Meaning)
		field(w, "origin", m.Origin)
		field(w, "words", links(v.Entries))
	}
}

func renderEntry(w io.Writer, v core.EntryView) {
	e := v.Entry
	fmt.Fprintf(w, "%s  %s\n", e.ID, e.Headword)
	for i, s := range v.Entry.Senses {
		var parts []string
		for _, p := range []string{s.POS, s.JA, s.EN} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		line := fmt.Sprintf("  %d. %s", i+1, strings.Join(parts, " — "))
		if s.Usage != "" {
			line += "  (" + s.Usage + ")"
		}
		fmt.Fprintln(w, line)
	}
	field(w, "etymology", e.Etymology)
	field(w, "parts", strings.Join(mapLinks(v.Parts), " + "))
	field(w, "phrase of", links(v.PhraseOf))
	field(w, "phrases", links(v.Phrases))
	field(w, "synonyms", links(v.Synonyms))
	field(w, "antonyms", links(v.Antonyms))
	field(w, "related", links(v.Related))
	for _, sr := range v.SameRoot {
		field(w, "same "+sr.Morpheme, links(sr.Entries))
	}
	if len(v.Citations) > 0 {
		fmt.Fprintln(w, "  citations:")
		for _, c := range v.Citations {
			fmt.Fprintf(w, "    %s  %s\n", c.Met, c.Text)
			var meta []string
			if c.URL != "" {
				meta = append(meta, c.URL)
			}
			if len(c.Tags) > 0 {
				meta = append(meta, "["+strings.Join(c.Tags, " ")+"]")
			}
			if len(meta) > 0 {
				fmt.Fprintf(w, "                %s\n", strings.Join(meta, "  "))
			}
		}
	}
	var tags []string
	for _, t := range v.Tags {
		tags = append(tags, fmt.Sprintf("%s ×%d", t.Tag, t.Count))
	}
	field(w, "tags", strings.Join(tags, ", "))
}

func field(w io.Writer, name, value string) {
	if value != "" {
		fmt.Fprintf(w, "  %s: %s\n", name, value)
	}
}

func links(ls []core.Link) string { return strings.Join(mapLinks(ls), ", ") }

// mapLinks writes each link as its text plus where it leads: "(id)" when it
// resolves, "(?)" when it does not yet, "(a | b)" when ambiguous. "←" marks
// a link stored on the other record.
func mapLinks(ls []core.Link) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		s := l.Text
		switch len(l.IDs) {
		case 0:
			s += " (?)"
		case 1:
			if l.IDs[0] != l.Text {
				s += " (" + l.IDs[0] + ")"
			}
		default:
			s += " (" + strings.Join(l.IDs, " | ") + ")"
		}
		if l.Inbound {
			s = "← " + s
		}
		out[i] = s
	}
	return out
}

func displayForm(typ, form string) string {
	switch typ {
	case "prefix":
		return form + "-"
	case "suffix":
		return "-" + form
	}
	return form
}
