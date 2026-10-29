# ubiq

Tooling for a domain-driven design ubiquitous language kept as Markdown in the repository.

ubiq reads the `GLOSSARY.md` glossaries, the `GLOSSARY-MAP.md` context map, and the `docs/adr` decision records.
It renders them as one HTML page, and it checks both the glossary and other text against the vocabulary.

## Usage

Every command reads the repository in the current directory. To read another one, pass `-root dir`. Every command
exits 2 on a usage or read error.

### Render the glossary as an HTML page

```sh
ubiq html
```

`html` writes `glossary.html`: one self-contained page with every term, context, and ADR, and a search box. The page
also has the context map, the vocabulary check, and the health report. It needs no server, so open it straight from
disk. To write it somewhere else, pass `-out file`. The command reports what
went into the page:

```
wrote glossary.html: 122 terms, 56 ADRs, 60 unresolved phrases
```

### Find problems in the glossary

```sh
ubiq health
```

`health` prints one finding per line as `path:line: rule: name: note`, then a summary on stderr:

```
GLOSSARY.md:21: long: Learning Goal: 7 sentences
GLOSSARY.md:35: orphan: Lineage: refers to 4, referenced by none
GLOSSARY.md: unresolved: Beat Warning: Course Video Manager: Beat
179 findings over 122 terms: 92 long, 27 orphan, 0 unmapped, 60 unresolved
```

It applies four rules:

- `long`: the definition is longer than two sentences.
- `orphan`: no other term's definition and no context relationship refers to the term. A reference is the term's
  name in bold, or written as defined when the name starts with a capital letter. ADRs do not count.
- `unmapped`: two contexts define the same name, and no context relationship between them mentions it.
- `unresolved`: a bold phrase matches no term, or a relationship label names no context. The note names the context
  and the term or relationship where the phrase appears.
  An unresolved finding has no line number.

`health` exits 1 when it finds anything, so you can run it in CI to keep the glossary clean.

### Check text against the vocabulary

```sh
ubiq check docs/notes.md
```

`check` reads the named files, or stdin when you name none. It lists the glossary terms it recognised and the
Capitalised Phrases that have no entry:

```
terms recognised: Clip (Course Video Manager); Lesson (Course Video Manager)
capitalised phrases without an entry: Upload Queue
```

A phrase without an entry is either a term the glossary is missing or a word to replace with an existing term.
Only phrases of two or more capitalised words count. `check` always exits 0.

## Examples

[ubiq's own glossary](https://stefaneg.github.io/ubiq/examples/ubiq.html) is the page rendered from this
repository's glossary and ADRs. [docs/examples/README.md](docs/examples/README.md) has more.

## Layout

ubiq reads the layout that Matt Pocock's
[domain-modeling skill](https://github.com/mattpocock/skills/tree/main/skills/engineering/domain-modeling) writes:

- **One context:** a `GLOSSARY.md` at the repository root.
- **Several contexts:** a `GLOSSARY-MAP.md` at the root whose `## Contexts` section links each context's glossary,
  one bullet per context:

  ```md
  ## Contexts

  - [Ordering](./src/ordering/GLOSSARY.md): receives and tracks customer orders
  - [Billing](./src/billing/GLOSSARY.md): generates invoices and processes payments
  ```

  The map's `## Relationships` section relates contexts, one bullet per relationship, such as
  `- **Ordering → Billing**: each **Order** becomes an **Invoice**`. The bold label names contexts by their heading
  or by their link text in `## Contexts`. A bold phrase resolves in its own context first, then in the others in map
  order. In a relationship, its own context is the first one the label names.

ADRs are read from `docs/adr/*.md` at the root and from `docs/adr/*.md` beside each context's glossary. With one
context, the root ADRs belong to it. With several, the root ADRs are system-wide.

## Install

```sh
go install github.com/stefaneg/ubiq@latest
```

## License

MIT, see [LICENSE](LICENSE).

The glossary, context map, and ADR formats come from the
[domain-modeling skill](https://github.com/mattpocock/skills/tree/main/skills/engineering/domain-modeling) in
[mattpocock/skills](https://github.com/mattpocock/skills), copyright (c) 2026 Matt Pocock, MIT licensed. See
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
