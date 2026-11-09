# Design

What glossary is for, what it stores, and why it is shaped this way. The
decisions were settled in a design interview on 2026-11-08; each is listed
with its reason so it can be revisited once the tool has been used.

## Purpose

A personal store for English words, phrases and idioms met while reading,
built up from the bottom: a word goes in when it is met, with the sentence it
was met in, and gains meanings, etymology and links over time.

## Versions

| Version | Scope |
|---|---|
| **v0 (this)** | The deterministic core: schema, validation, a file store, and the commands `put`, `show`, `check`. |
| next | A conversation loop in this program on the Claude API: rough input is analysed, choices are offered, and only what the owner approves is `put`. Tool definitions come from the same Go types. |
| later, if wanted | Spaced repetition, collocation extraction, search, sources as records, an MCP server, Neovim. |

Installing through `nix-packages` / `dotfiles-mac` is a separate piece of
work.

**Why the core first:** the schema is the part that is expensive to change
once data exists, and a conversation layer needs finished, tested handlers to
call. Features are added after the tool has been used and refactored.

**Why the conversation will live in this program**, not in a Claude Code
skill: it must not interfere with the owner's user-scope skills, and a
session started from Claude Code cannot be fully isolated from them without
an API key anyway.

## Records

Three kinds, each one JSON file. Every record has `kind` and `id`; `created`
is set by the tool.

### Entry

One per spelling **and** etymology: "bank" of a river and "bank" for money
are `bank-1` and `bank-2`. Senses and parts of speech sit inside the entry.
A phrase ("run into") is its own entry, linked to its head word by
`phrase_of`.

| Field | |
|---|---|
| `headword` | Required. The only required field. |
| `senses[]` | `{pos, ja, en, usage}`, in display order. |
| `etymology` | Free text. |
| `parts[]` | References to morphemes. |
| `synonyms[]`, `antonyms[]`, `related[]`, `phrase_of[]` | References to entries. |

ID: `<slug of headword>-<n>`, e.g. `run-into-1`.

**Why split on etymology:** etymology and word parts belong to a word's
origin, not to one of its senses.

### Morpheme

A prefix, suffix or root, defined once: `type` (`prefix` | `suffix` |
`root`), `form`, `variants[]`, `meaning`, `origin`. ID: `<type>-<form>`, e.g.
`root-spect`; type and form cannot change after creation.

**Why its own record:** the meaning is written once instead of in every
entry, variants (`spic` for `spect`) can match, and "other words with this
root" comes from looking up which entries name it.

### Citation

A sentence met somewhere: `text`, `url`, `tags[]`, `met` (date),
`entries[]` as `{ref, form}`. ID: `c-<YYYYMMDD>-<6 hex>`.

**Why "citation":** it is the lexicographers' term for an example sentence
collected with its source and date. It was called "quote" at first, which
did not read as "quoted from a source".

**Why its own record:** one sentence often holds several new words; it is
stored once and linked to each.

`form` is the word as written in the text ("ran into" for "run into"); empty
means the headword. **Why not character offsets:** offsets break when the
text is corrected, and a form is enough to find the word again.

### Tags

Lowercase strings on citations, optionally `key:value` (`genre:paper`,
`domain:ml`). An entry's tags are counted from its citations and never stored on
the entry. **Why the convention instead of fixed fields:** a new kind of tag
needs no schema change.

## References

- **Entry references** are an ID (`bank-2`) or a headword (`omnipresent`).
  A headword that has no entry yet is allowed and shows as unresolved; it
  resolves once the entry is added. A headword shared by several entries is
  ambiguous and shows every candidate.
- **Morpheme references** are an ID or a form, whose hyphens give the type:
  `in-` is a prefix, `-ion` a suffix, `spect` any type.
- **Links are stored one way.** `show` adds the reverse: a synonym stored on
  A appears on B too, and `phrase_of` appears in reverse as `phrases`.
- **Citation links are stored as IDs.** `put` resolves each `ref` and writes the
  entry's ID, because a headword that is unique today may not stay unique.
  A headword with no entry gets a stub entry holding only the headword.

## Validation

`put` is the only way records are written; the files belong to the tool and
are not edited by hand.

- Unknown fields, unknown kinds and types, malformed dates, URLs and tags are
  rejected.
- Whitespace is normalised; citation text becomes one line, so text copied with
  line breaks is accepted.
- **A citation's linked word must appear in its text** (the form if given, else
  the headword, ignoring case). This catches a typo in either the sentence or
  the form.
- An update must name an existing ID; `created` cannot change; an entry's
  headword must still fit its ID (renaming is not supported yet).
- `--dry-run` runs every check and reports exactly what would be written,
  so what is approved and what is stored are the same data.

`check` re-validates every stored record and reports links that do not
resolve. Unresolved entry and morpheme references are warnings, not errors.

## Storage

One file per record under `$GLOSSARY_DIR` (default `~/.local/share/glossary`):

```
entries/bank-1.json
morphemes/root-spect.json
citations/c-20261108-a3f09c.json
```

Every read and write goes through the `store.Store` interface (`Load`
everything, `Save` records). **Why files:** they are plain, diffable and need
no server; at a few thousand records, reading them all per command is fast.
**Why the interface:** the data may move to an external database once the
tool is in real use; that would be a second implementation of `Store`.

## Code

| Package | |
|---|---|
| `internal/model` | Record types, decoding, normalising and validating one record. |
| `internal/store` | The `Store` interface and the file store. |
| `internal/core` | `Put`, `Show`, `Check`: resolving references across records. |
| `cmd/glossary` | Flags, exit codes, text output. |

Go, standard library only.
