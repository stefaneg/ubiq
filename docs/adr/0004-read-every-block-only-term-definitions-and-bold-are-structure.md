# Read every block; only Term definitions and bold are structure

ubiq reads every block of a Glossary and the Context Map, not only the sections the domain-modeling skill defines. A
`**Name**:` line and its definition, under any heading, define a Term. Every other paragraph, bullet, table row or
quote is a Statement,
labelled by the headings above it, and its bold and capitalised mentions are References. The Terms one Statement
mentions are Co-mentions, and a Statement that names two Contexts is a Context Relationship. This supersedes ADR 0002
on which sections to read, though its dropping of the Shared and Home Context stands. It also widens ADR 0001 from
Context Relationships to every Statement.

Reading only the latest format's sections dropped the Relationships, Flagged ambiguities and Cross-context vocabulary
that older Glossaries hold. That left most of their Terms as Orphans or Unmapped Names, and a parser per
format would have to chase every variant. The price is that bold anywhere in a Glossary now claims a Term. Also, a
heading whose Statements name every Context relates every name mentioned under it.
