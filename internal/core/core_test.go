package core

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hypatia-tile/glossary/internal/model"
	"github.com/hypatia-tile/glossary/internal/store"
)

func newEnv(t *testing.T) (Env, string) {
	t.Helper()
	dir := t.TempDir()
	return Env{
		Store: store.Files{Dir: dir},
		Now:   func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.Local) },
		// Enough bytes for several citation IDs, all different.
		Rand: bytes.NewReader([]byte{0xa3, 0xf0, 0x9c, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}),
	}, dir
}

// mustPut puts a record and fails the test unless it was accepted.
func mustPut(t *testing.T, env Env, js string) *PutResult {
	t.Helper()
	res, err := env.Put([]byte(js), false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("put %s: %v", js, res.Errors)
	}
	return res
}

func putErrors(t *testing.T, env Env, js string) model.Errors {
	t.Helper()
	res, err := env.Put([]byte(js), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatalf("put %s: accepted, want errors", js)
	}
	return res.Errors
}

func hasError(errs model.Errors, path, substr string) bool {
	return slices.ContainsFunc(errs, func(e model.FieldError) bool {
		return e.Path == path && strings.Contains(e.Message, substr)
	})
}

func TestPutEntryAssignsIDs(t *testing.T) {
	env, dir := newEnv(t)
	res := mustPut(t, env, `{"kind":"entry","headword":"  bank ","senses":[{"pos":"Noun","ja":"銀行"}]}`)
	if res.ID != "bank-1" || res.Action != "create" {
		t.Fatalf("got %s %s, want create bank-1", res.Action, res.ID)
	}
	e := res.Record.(*model.Entry)
	if e.Headword != "bank" || e.Senses[0].POS != "noun" || e.Created != "2026-11-08" {
		t.Errorf("not normalised: %+v", e)
	}
	if _, err := os.Stat(filepath.Join(dir, "entries", "bank-1.json")); err != nil {
		t.Error(err)
	}

	res = mustPut(t, env, `{"kind":"entry","headword":"Bank","etymology":"Old Norse bakki"}`)
	if res.ID != "bank-2" {
		t.Errorf("second bank: got %s, want bank-2", res.ID)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "bank-1") {
		t.Errorf("want a warning naming bank-1, got %v", res.Warnings)
	}
}

func TestPutEntryUpdate(t *testing.T) {
	env, _ := newEnv(t)
	mustPut(t, env, `{"kind":"entry","headword":"bank"}`)
	res := mustPut(t, env, `{"kind":"entry","id":"bank-1","headword":"Bank","etymology":"Italian banca"}`)
	if res.Action != "update" || res.Record.(*model.Entry).Created != "2026-11-08" {
		t.Errorf("got %+v", res)
	}
	if errs := putErrors(t, env, `{"kind":"entry","id":"bank-1","headword":"bench"}`); !hasError(errs, "headword", "renaming") {
		t.Errorf("rename: got %v", errs)
	}
	if errs := putErrors(t, env, `{"kind":"entry","id":"bank-9","headword":"bank"}`); !hasError(errs, "id", "no entry") {
		t.Errorf("missing id: got %v", errs)
	}
	if errs := putErrors(t, env, `{"kind":"entry","id":"bank-1","headword":"bank","created":"2020-01-01"}`); !hasError(errs, "created", "") {
		t.Errorf("changed created: got %v", errs)
	}
}

func TestPutRejects(t *testing.T) {
	env, _ := newEnv(t)
	for _, tc := range []struct{ js, path, substr string }{
		{`{"headword":"x"}`, "kind", "required"},
		{`{"kind":"word","headword":"x"}`, "kind", "unknown kind"},
		{`{"kind":"entry","headword":"x","meaning":"y"}`, "", "unknown field"},
		{`{"kind":"entry","headword":""}`, "headword", "required"},
		{`{"kind":"entry","headword":"x","created":"2026-11-08"}`, "created", "set by the tool"},
		{`{"kind":"entry","headword":"x","senses":[{}]}`, "senses[0]", "at least one"},
		{`{"kind":"entry","headword":"x","synonyms":["X"]}`, "synonyms[0]", "itself"},
		{`{"kind":"morpheme","type":"stem","form":"spect"}`, "type", "unknown type"},
		{`{"kind":"citation","text":"hi","entries":[]}`, "entries", "at least one"},
		{`{"kind":"citation","text":"hi","url":"example.com","entries":[{"ref":"hi"}]}`, "url", "http"},
		{`{"kind":"citation","text":"hi","tags":["two words"],"entries":[{"ref":"hi"}]}`, "tags[0]", "no spaces"},
	} {
		if errs := putErrors(t, env, tc.js); !hasError(errs, tc.path, tc.substr) {
			t.Errorf("%s: got %v, want %s: …%s…", tc.js, errs, tc.path, tc.substr)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	env, dir := newEnv(t)
	res, err := env.Put([]byte(`{"kind":"citation","text":"It was ubiquitous.","entries":[{"ref":"ubiquitous"}]}`), true)
	if err != nil || !res.OK || len(res.Stubs) != 1 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if _, err := os.Stat(dir + "/entries"); !os.IsNotExist(err) {
		t.Errorf("dry run wrote to the store")
	}
}

func TestPutCitation(t *testing.T) {
	env, dir := newEnv(t)
	mustPut(t, env, `{"kind":"entry","headword":"practitioner"}`)
	res := mustPut(t, env, `{"kind":"citation",
		"text":"The findings were  ubiquitous\nyet ignored by practitioners.",
		"url":"https://example.com/paper","tags":["Genre:Paper","genre:paper"],
		"entries":[{"ref":"ubiquitous"},{"ref":"practitioner"}]}`)
	c := res.Record.(*model.Citation)
	if c.ID != "c-20261108-a3f09c" || c.Met != "2026-11-08" {
		t.Errorf("got id %s met %s", c.ID, c.Met)
	}
	if c.Text != "The findings were ubiquitous yet ignored by practitioners." {
		t.Errorf("text not squashed: %q", c.Text)
	}
	if !slices.Equal(c.Tags, []string{"genre:paper"}) {
		t.Errorf("tags: %v", c.Tags)
	}
	if c.Entries[0].Ref != "ubiquitous-1" || c.Entries[1].Ref != "practitioner-1" {
		t.Errorf("refs not stored as ids: %+v", c.Entries)
	}
	if len(res.Stubs) != 1 || res.Stubs[0].ID != "ubiquitous-1" {
		t.Errorf("stubs: %+v", res.Stubs)
	}
	if _, err := os.Stat(filepath.Join(dir, "entries", "ubiquitous-1.json")); err != nil {
		t.Error("stub not saved:", err)
	}
}

func TestPutCitationChecksTheWordIsInTheText(t *testing.T) {
	env, _ := newEnv(t)
	mustPut(t, env, `{"kind":"entry","headword":"run into"}`)
	if errs := putErrors(t, env, `{"kind":"citation","text":"She ran into a friend.","entries":[{"ref":"run into"}]}`); !hasError(errs, "entries[0].form", "set form") {
		t.Errorf("got %v", errs)
	}
	if errs := putErrors(t, env, `{"kind":"citation","text":"She ran into a friend.","entries":[{"ref":"run into","form":"rna into"}]}`); !hasError(errs, "entries[0].form", "does not appear") {
		t.Errorf("typo in form: got %v", errs)
	}
	mustPut(t, env, `{"kind":"citation","text":"She ran into a friend.","entries":[{"ref":"run into","form":"ran into"}]}`)
}

func TestPutCitationAmbiguousRef(t *testing.T) {
	env, _ := newEnv(t)
	mustPut(t, env, `{"kind":"entry","headword":"bank"}`)
	mustPut(t, env, `{"kind":"entry","headword":"bank"}`)
	errs := putErrors(t, env, `{"kind":"citation","text":"by the bank","entries":[{"ref":"bank"}]}`)
	if !hasError(errs, "entries[0].ref", "bank-1, bank-2") {
		t.Errorf("got %v", errs)
	}
	if errs := putErrors(t, env, `{"kind":"citation","text":"by the bank","entries":[{"ref":"bank-7"}]}`); !hasError(errs, "entries[0].ref", "no entry") {
		t.Errorf("missing id: got %v", errs)
	}
	mustPut(t, env, `{"kind":"citation","text":"by the bank","entries":[{"ref":"bank-2"}]}`)
}

func TestPutMorpheme(t *testing.T) {
	env, _ := newEnv(t)
	res := mustPut(t, env, `{"kind":"morpheme","type":"prefix","form":"In-","meaning":"into"}`)
	if res.ID != "prefix-in" {
		t.Errorf("got %s", res.ID)
	}
	if errs := putErrors(t, env, `{"kind":"morpheme","type":"prefix","form":"in"}`); !hasError(errs, "", "already exists") {
		t.Errorf("duplicate: got %v", errs)
	}
	if errs := putErrors(t, env, `{"kind":"morpheme","id":"prefix-in","type":"root","form":"in"}`); !hasError(errs, "id", "cannot change") {
		t.Errorf("type change: got %v", errs)
	}
	mustPut(t, env, `{"kind":"morpheme","id":"prefix-in","type":"prefix","form":"in","meaning":"in, into"}`)
}

func TestShow(t *testing.T) {
	env, _ := newEnv(t)
	mustPut(t, env, `{"kind":"morpheme","type":"root","form":"spect","variants":["spic"],"meaning":"look"}`)
	mustPut(t, env, `{"kind":"entry","headword":"inspect","parts":["in-","spect"],"synonyms":["examine"]}`)
	mustPut(t, env, `{"kind":"entry","headword":"conspicuous","parts":["con-","spic"]}`)
	mustPut(t, env, `{"kind":"entry","headword":"scrutinize","synonyms":["inspect"]}`)
	mustPut(t, env, `{"kind":"citation","text":"Inspect the logs.","tags":["genre:docs"],"entries":[{"ref":"inspect"}]}`)
	mustPut(t, env, `{"kind":"citation","text":"They inspected it.","tags":["genre:docs","domain:ops"],"entries":[{"ref":"inspect"}]}`)

	res, err := env.Show("inspect")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("got %d entries", len(res.Entries))
	}
	v := res.Entries[0]
	wantSyn := []Link{{Text: "examine"}, {Text: "scrutinize", IDs: []string{"scrutinize-1"}, Inbound: true}}
	if !slices.EqualFunc(v.Synonyms, wantSyn, linkEqual) {
		t.Errorf("synonyms: got %+v", v.Synonyms)
	}
	if len(v.SameRoot) != 1 || v.SameRoot[0].Morpheme != "root-spect" || v.SameRoot[0].Entries[0].Text != "conspicuous" {
		t.Errorf("same root: got %+v", v.SameRoot)
	}
	if len(v.Citations) != 2 {
		t.Errorf("citations: got %d", len(v.Citations))
	}
	if want := []TagCount{{"genre:docs", 2}, {"domain:ops", 1}}; !slices.Equal(v.Tags, want) {
		t.Errorf("tags: got %v", v.Tags)
	}

	res, err = env.Show("spic")
	if err != nil || len(res.Morphemes) != 1 || len(res.Morphemes[0].Entries) != 2 {
		t.Errorf("morpheme by variant: got %+v, %v", res, err)
	}
	if _, err := env.Show("nothing"); err != ErrNotFound {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func linkEqual(a, b Link) bool {
	return a.Text == b.Text && a.Inbound == b.Inbound && slices.Equal(a.IDs, b.IDs)
}

func TestCheck(t *testing.T) {
	env, dir := newEnv(t)
	mustPut(t, env, `{"kind":"entry","headword":"inspect","parts":["spect"]}`)
	mustPut(t, env, `{"kind":"citation","text":"Inspect it.","entries":[{"ref":"inspect"}]}`)
	ps, err := env.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Severity != SevWarning || !strings.Contains(ps[0].Message, "no morpheme") {
		t.Errorf("got %+v, want one warning about spect", ps)
	}

	os.WriteFile(filepath.Join(dir, "entries", "broken-1.json"), []byte(`{"kind":"entry",`), 0o644)
	os.WriteFile(filepath.Join(dir, "entries", "other-1.json"), []byte(`{"kind":"entry","id":"inspect-1","headword":"inspect","created":"2026-11-08"}`), 0o644)
	ps, _ = env.Check()
	var errs []string
	for _, p := range ps {
		if p.Severity == SevError {
			errs = append(errs, filepath.Base(p.Where)+": "+p.Message)
		}
	}
	if len(errs) != 2 || !strings.Contains(errs[1], "does not match the file name") {
		t.Errorf("got %v", errs)
	}
}
