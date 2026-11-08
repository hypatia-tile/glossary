package main

import (
	"bytes"
	"strings"
	"testing"
)

// The exit code is part of the interface: a caller tells "invalid" (1) from
// "could not run" (2) without parsing output.
func TestExitCodes(t *testing.T) {
	t.Setenv("GLOSSARY_DIR", t.TempDir())
	for _, tc := range []struct {
		args  []string
		stdin string
		want  int
	}{
		{nil, "", exitError},
		{[]string{"nope"}, "", exitError},
		{[]string{"show"}, "", exitError},
		{[]string{"put"}, `{"kind":"entry"}`, exitNo},
		{[]string{"put", "--dry-run"}, `{"kind":"entry","headword":"bank"}`, 0},
		{[]string{"show", "bank"}, "", exitNo}, // the dry run wrote nothing
		{[]string{"put"}, `{"kind":"entry","headword":"bank","senses":[{"ja":"銀行"}]}`, 0},
		{[]string{"show", "bank"}, "", 0},
		{[]string{"show", "--json", "bank-1"}, "", 0},
		{[]string{"check"}, "", 0},
	} {
		var out, errOut bytes.Buffer
		if got := run(tc.args, strings.NewReader(tc.stdin), &out, &errOut); got != tc.want {
			t.Errorf("glossary %v: exit %d, want %d\nstdout: %s\nstderr: %s", tc.args, got, tc.want, &out, &errOut)
		}
	}
}

func TestShowText(t *testing.T) {
	t.Setenv("GLOSSARY_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	run([]string{"put"}, strings.NewReader(`{"kind":"entry","headword":"bank","senses":[{"pos":"noun","ja":"銀行","en":"a place for money"}],"synonyms":["lender"]}`), &out, &errOut)
	out.Reset()
	run([]string{"show", "bank"}, nil, &out, &errOut)
	want := "bank-1  bank\n  1. noun — 銀行 — a place for money\n  synonyms: lender (?)\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", &out, want)
	}
}
