# glossary

A personal store for English words, phrases and idioms met while reading,
with the sentences they were met in, their etymology and word parts, and
links between them.

**Status:** v0, the core only. The design and what comes next are in
[`docs/design.md`](docs/design.md).

## Use

```sh
glossary put [--dry-run] < record.json   # validate a record and save it
glossary show [--json] <ref>             # an entry or morpheme, links resolved
glossary check                           # validate everything stored
```

`put` reads one record as JSON and prints the result as JSON: the record as
it will be stored, or the errors. Exit status is 0 on success, 1 when the
answer is "no" (invalid record, nothing found, `check` found errors) and 2
when the command could not run.

```sh
echo '{"kind":"citation","text":"She ran into an old friend.",
       "url":"https://example.com","tags":["genre:novel"],
       "entries":[{"ref":"run into","form":"ran into"}]}' | glossary put
glossary show "run into"
```

Records are kept in `$GLOSSARY_DIR`, by default `~/.local/share/glossary`.

## Development

```sh
nix develop      # or: direnv allow
go test ./...
nix build        # ./result/bin/glossary
```
