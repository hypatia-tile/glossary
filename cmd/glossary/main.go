// Command glossary stores words, phrases and the sentences they were met in.
//
//	glossary put [--dry-run] < record.json
//	glossary show [--json] <headword|id|morpheme>
//	glossary check
//
// Records live in $GLOSSARY_DIR, by default $XDG_DATA_HOME/glossary
// (~/.local/share/glossary).
package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hypatia-tile/glossary/internal/core"
	"github.com/hypatia-tile/glossary/internal/store"
)

const usage = `usage:
  glossary put [--dry-run] < record.json   validate a record and save it
  glossary show [--json] <ref>             show an entry or morpheme with its links
  glossary check                           validate everything stored

Records are kept in $GLOSSARY_DIR (default: ~/.local/share/glossary).
`

// Exit codes: 1 means the command ran and the answer is "no" (invalid
// record, nothing found, check found errors); 2 means it could not run.
const (
	exitNo    = 1
	exitError = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitError
	}
	dir, err := dataDir()
	if err != nil {
		fmt.Fprintln(stderr, "glossary:", err)
		return exitError
	}
	env := core.Env{Store: store.Files{Dir: dir}, Now: time.Now, Rand: rand.Reader}

	cmd, args := args[0], args[1:]
	fs := flag.NewFlagSet("glossary "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	switch cmd {
	case "put":
		dryRun := fs.Bool("dry-run", false, "validate and report, but write nothing")
		if fs.Parse(args) != nil || fs.NArg() != 0 {
			fmt.Fprint(stderr, usage)
			return exitError
		}
		return put(env, *dryRun, stdin, stdout, stderr)
	case "show":
		asJSON := fs.Bool("json", false, "print JSON instead of text")
		if fs.Parse(args) != nil || fs.NArg() != 1 {
			fmt.Fprint(stderr, usage)
			return exitError
		}
		return show(env, fs.Arg(0), *asJSON, stdout, stderr)
	case "check":
		if fs.Parse(args) != nil || fs.NArg() != 0 {
			fmt.Fprint(stderr, usage)
			return exitError
		}
		return check(env, stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "glossary: unknown command %q\n%s", cmd, usage)
		return exitError
	}
}

func dataDir() (string, error) {
	if d := os.Getenv("GLOSSARY_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "glossary"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "glossary"), nil
}

func put(env core.Env, dryRun bool, stdin io.Reader, stdout, stderr io.Writer) int {
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "glossary:", err)
		return exitError
	}
	res, err := env.Put(data, dryRun)
	if err != nil {
		fmt.Fprintln(stderr, "glossary:", err)
		return exitError
	}
	writeJSON(stdout, res)
	if !res.OK {
		return exitNo
	}
	return 0
}

func show(env core.Env, ref string, asJSON bool, stdout, stderr io.Writer) int {
	res, err := env.Show(ref)
	if errors.Is(err, core.ErrNotFound) {
		fmt.Fprintf(stderr, "glossary: nothing matches %q\n", ref)
		return exitNo
	}
	if err != nil {
		fmt.Fprintln(stderr, "glossary:", err)
		return exitError
	}
	if asJSON {
		writeJSON(stdout, res)
	} else {
		renderShow(stdout, res)
	}
	return 0
}

func check(env core.Env, stdout, stderr io.Writer) int {
	ps, err := env.Check()
	if err != nil {
		fmt.Fprintln(stderr, "glossary:", err)
		return exitError
	}
	errs := 0
	for _, p := range ps {
		where := p.Where
		if p.Path != "" {
			where += " " + p.Path
		}
		fmt.Fprintf(stdout, "%s: %s: %s\n", p.Severity, where, p.Message)
		if p.Severity == core.SevError {
			errs++
		}
	}
	if errs > 0 {
		return exitNo
	}
	return 0
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}
