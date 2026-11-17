# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`GLOSSARY.md`** at the repo root: the Terms, their Avoided Words, and the Clusters they sit in.
- **`docs/adr/`**: read ADRs that touch the area you're about to work in.
- **`docs/server.md`**: the design of `ubiq serve`, when the work touches the Server.

How an entry or an ADR is written is set by the in-repo `domain-modeling` skill: `.claude/skills/domain-modeling/GLOSSARY-FORMAT.md` and `ADR-FORMAT.md`. ubiq itself reads those formats, so `go run . health` tells you when an edit broke them.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

Single-context repo, which this is:

```
/
├── GLOSSARY.md
├── docs/
│   ├── adr/
│   │   ├── 0001-references-are-bold-or-capitalised-mentions.md
│   │   └── ...
│   └── server.md
├── internal/
└── main.go
```

A multi-context repo would have a `GLOSSARY-MAP.md` at the root pointing at one `GLOSSARY.md` per context, each with its own `docs/adr/` beside it. ubiq reads that layout too; this repo does not use it.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `GLOSSARY.md`. Don't drift to synonyms the glossary explicitly avoids. `go run . check` on your text lists the Terms it uses and the Avoided Words to replace.

If the concept you need isn't in the glossary yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0005 (Changes land in a numbered Batch branch), but worth reopening because…_
