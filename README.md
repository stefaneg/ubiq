# ubiq

Tooling for a domain-driven design ubiquitous language kept as Markdown in the repository.

ubiq reads the `GLOSSARY.md` glossaries, the `GLOSSARY-MAP.md` context map and the `docs/adr` decision records,
and:

- `ubiq html [-root dir] [-out glossary.html]` renders them as one self-contained, searchable HTML page.
- `ubiq check [-root dir] [file ...]` checks free text (stdin when no file is given) against the vocabulary,
  listing recognised terms and Capitalised Phrases that have no entry.
- `ubiq health [-root dir]` reports glossary health findings as `path:line: rule: name: note` and exits 1 when
  there are any.

Every command exits 2 on a usage or read error.

## Install

```sh
go install github.com/stefaneg/ubiq@latest
```
