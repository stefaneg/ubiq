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

## Layout

ubiq reads the layout the domain-modeling skill writes:

- **One context:** a `GLOSSARY.md` at the repository root.
- **Several contexts:** a `GLOSSARY-MAP.md` at the root whose `## Contexts` section links each context's glossary,
  one bullet per context:

  ```md
  ## Contexts

  - [Ordering](./src/ordering/GLOSSARY.md): receives and tracks customer orders
  - [Billing](./src/billing/GLOSSARY.md): generates invoices and processes payments
  ```

  A context listed with the root `GLOSSARY.md` is the shared one: a bold phrase resolves in its own context first,
  then in the shared context, then in the others in map order.

ADRs are read from `docs/adr/*.md` at the root and from `docs/adr/*.md` beside each context's glossary.

## Install

```sh
go install github.com/stefaneg/ubiq@latest
```

## License

MIT, see [LICENSE](LICENSE).
