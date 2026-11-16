# ubiq

Tooling for a domain-driven design ubiquitous language kept as Markdown in the repository.

ubiq reads the `GLOSSARY.md` glossaries, the `GLOSSARY-MAP.md` context map, and the `docs/adr` decision records.
It renders them as one HTML page, looks up what a word means, and checks both the glossary and other text against
the vocabulary. It also serves the lookup, the check and the health report to coding agents over MCP.

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
- `orphan`: no other term's definition and no statement (see [Layout](#layout)) refers to the term. A reference is
  the term's name in bold, or written as defined when the name starts with a capital letter. ADRs do not count.
- `unmapped`: two contexts define the same name, and no heading has statements under it that mention the name and
  name both contexts.
- `unresolved`: a bold phrase matches no term or context, or a name in a bold label such as `Ordering → Billing`
  matches no context. The note names the context and the term or headings where the phrase appears.
  An unresolved finding has no line number.

`health` exits 1 when it finds anything, so you can run it in CI to keep the glossary clean.

### Look up a word

```sh
ubiq lookup Clip "upload queue" VideoFile
```

`lookup` answers, for each word, what the glossary says about it. The word may be written as prose or as an
identifier: `StackImage`, `stack_image`, `stack-image` and `stack images` all find the term Stack Image. When the word
names a term, `lookup` prints the term with its context, its cluster when it sits under one, its place, and its
definition. It also prints:

- what the term refers to, and what refers to it
- the terms that statements mention alongside it
- up to five of those statements
- the ADRs that mention it

```
Clip (Course Video Manager › Editing), GLOSSARY.md:12
  A cut of one Recording that belongs to a Lesson.
  avoid: video, snippet
  refers to: Recording, Lesson
  referred to by: Lesson
  co-mentioned with: Recording, Timeline
  statement: Relationships: A Clip sits on one Timeline and plays part of a Recording. (GLOSSARY.md:40)
  ADR: Cut clips at keyframes (docs/adr/0003-cut-clips-at-keyframes.md)
```

When a term lists the word under `_Avoid_`, `lookup` says so and prints the term to use instead.

When neither holds for the whole word, `lookup` splits it into parts, so that a new name such as
`StackImageRefResolver` shows the terms it is built from. Scanning from the left, each part is the longest run of
words that names a term or that a term avoids; a word that is neither is a part on its own. Each part gets one line:

```
"StackImageRefResolver" names no Term, and no Term avoids it. Its parts:
  stack image ref: Stack Image Ref (Stacks), internal/stacks/GLOSSARY.md:34
  resolver: names no Term, and no Term avoids it
```

Look up a part to get its full entry. When no part matches either, `lookup` lists up to ten terms whose name,
`_Avoid_` line or definition has every word, and those whose name has more of the words come first. `lookup` always
exits 0.

### Check text against the vocabulary

```sh
ubiq check docs/notes.md
```

`check` reads the named files, or stdin when you name none. It lists the glossary terms it recognised, the
Capitalised Phrases that have no entry, and the words a term's `_Avoid_` line rules out, each with the term to use
instead:

```
terms recognised: Clip (Course Video Manager); Lesson (Course Video Manager)
capitalised phrases without an entry: Upload Queue
avoided words: video → Clip (Course Video Manager)
```

A phrase without an entry is either a term the glossary is missing or a word to replace with an existing term.
Only phrases of two or more capitalised words count. An avoided word that only shortens the term's own name, such as
"name" under Image Base Name, is not reported. Where a term is part of a longer avoided word, as Image is part of
"container image", the longer one wins. `check` always exits 0.

### Serve agents over MCP

```sh
ubiq mcp
```

`mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io) server on stdin and stdout, so that a coding
agent can ask about the glossary while it works. It has four read-only tools:

- `lookup` takes a list of words and answers as `ubiq lookup` does.
- `check` takes a text and answers as `ubiq check` does.
- `health` answers as `ubiq health` does, findings and summary line together.
- `whoami` gives the git identity of the working tree, `user.name` and `user.email`, with role Maintainer.

Each call reads the glossary again, so an agent that edits `GLOSSARY.md` gets answers from the edited file. A call
takes milliseconds even for a glossary of a thousand terms. To add the server to Claude Code for a repository, run
this in the repository:

```sh
claude mcp add --scope project ubiq -- ubiq mcp
```

The command writes it to `.mcp.json`. Other agents take the same command, `ubiq mcp`, started in the repository.

For a small glossary, an agent can as well read `GLOSSARY.md` itself. The server pays off once the glossary and ADRs
are too long to read in every session, or once the repository has several contexts.

### Serve the page and the tools over HTTP

```sh
ubiq serve -root /var/lib/ubiq/repo -origin https://github.com/example/handbook.git \
  -assertion-keys https://authenticate.glossary.example.com/.well-known/pomerium/jwks.json -writers glossary-writers
```

`serve` runs the Server: one long-running process for one repository, behind an identity-aware proxy, that serves
the page `ubiq html` renders at `/`, with the Person's name and role at the top, and the four tools of `ubiq mcp` over
the MCP streamable transport at `/mcp`. Every request is on behalf of a Person the proxy vouches for; a request with
none is answered `401`. A Writer's Agent may also call `define_term`: the Term lands at once as a commit authored by
the Person in the open Batch, a `ubiq/<n>` branch on origin that a Maintainer merges by hand. The Server clones
origin into `-root` on first start and reuses the clone after; it reads the open Batch, and its instructions name
the branch. It logs one line per request and per Change to stderr. The design is [docs/server.md](docs/server.md).

| Flag | Default | Meaning |
|---|---|---|
| `-root <dir>` | `.` | The clone the Server owns: cloned from `-origin` when it holds no `.git`, reused otherwise. |
| `-origin <url>` | required | The repository, by HTTPS; SSH is refused. A local path is accepted for development. |
| `-listen <addr>` | `127.0.0.1:8766` | Loopback; the proxy sits in front. |
| `-assertion-keys <url>` | required unless `-person` | Where the proxy publishes the keys that sign its assertion. |
| `-ca <file>` | system roots | Extra CA to trust when fetching the keys. |
| `-writer-claim <name>` | `groups` | The Writer Rule's claim. |
| `-writers <v,v,...>` | required | The Writer Rule's accepted values. |
| `-committer "Name <email>"` | `ubiq serve <ubiq@localhost>` | The Server's git identity, committer of every Change. |
| `-person "Name <email>"` | off | Dev direct mode: every request is this Person, a Writer, with no proxy; loopback only. |

It refuses to start when `-writers` is missing, when it has neither or both of `-assertion-keys` and `-person`, when
`-person` is given with a `-listen` that is not loopback, or when `-origin` is missing, SSH or another scheme than
HTTPS. To try it alone:

```sh
ubiq serve -root /tmp/handbook -origin /path/to/handbook.git -writers dev -person "Ada Lovelace <ada@example.com>"
```

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

ubiq reads every section of every glossary and of the map. Only two things in them are structure: term definitions
and bold phrases.

A **term** is a bold name followed by a colon on a line of its own, then its definition and an optional `_Avoid_`
line. It may sit under any heading. Its cluster is the nearest heading above it, skipping one named Language, or
General when there is none. The text between that heading and the cluster's first term is the cluster's intro.

```md
## Language

### Orders              ← cluster Orders

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

## Billing              ← cluster Billing

### Language

**Invoice**:
…

### Relationships

- An **Order** becomes one **Invoice**.
```

Every other block is a **statement**: a paragraph, a bullet with its indented continuation lines, a table row, or a
quote. Fenced code is skipped. Each statement is labelled by the headings above it, such as
`Billing › Relationships`. It mentions the terms named in it, in bold or capitalised as defined, plus those its
nearest heading names. Terms one statement mentions are **co-mentioned**, and `lookup` and the page show them next to
each term.

A statement names a context when it writes the context's heading or its link text from `## Contexts`, in bold or
capitalised as written. A bold label such as `**Ordering → Billing**` names each context it joins, and a name in it
that matches no context is unresolved. A statement that names two or more contexts is a **context relationship**:

```md
## Relationships

- **Ordering → Billing**: each **Order** becomes an **Invoice**.

## Cross-context vocabulary

#### Customer

- Ordering: the person who places an **Order**.
- Billing: the party an **Invoice** is sent to.
```

A bold phrase resolves in the statement's own context first, then in the others in map order. A statement in the map
has the first context it names as its own.

`unmapped` reads the statements under one heading together. In the example above, the two bullets under
`#### Customer` together mention Customer and name both Ordering and Billing, so Customer is not unmapped.

ADRs are read from `docs/adr/*.md` at the root and from `docs/adr/*.md` beside each context's glossary. With one
context, the root ADRs belong to it. With several, the root ADRs are system-wide.

## Install

From GitHub:

```sh
go install github.com/stefaneg/ubiq@latest
```

From a local clone, in its root:

```sh
go install .
```

## License

MIT, see [LICENSE](LICENSE).

The glossary, context map, and ADR formats come from the
[domain-modeling skill](https://github.com/mattpocock/skills/tree/main/skills/engineering/domain-modeling) in
[mattpocock/skills](https://github.com/mattpocock/skills), copyright (c) 2026 Matt Pocock, MIT licensed. See
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
