# ubiq

ubiq reads the **Domain Model** a repository keeps as Markdown, renders it as one page, answers what its words mean,
and reports where its language is unclear or broken.

## Language

### Domain model

**Domain Model**:
All of a repository's **Glossaries**, its **Context Map**, and its **ADRs**, read together.
_Avoid_: model, ubiquitous language, glossary

**Context**:
An area of the system in which each **Term** has one meaning. Each Context has exactly one **Glossary**.
_Avoid_: bounded context, module, glossary file

**Glossary**:
The written language of one **Context**: a short description of the **Context** and its **Terms**, which may be
grouped into **Clusters**.
_Avoid_: dictionary, vocabulary file

**Context Map**:
The document that lists a repository's **Contexts** and where each one's **Glossary** lives.
_Avoid_: glossary map, index

**Context Relationship**:
A **Statement** that names two or more **Contexts**.
_Avoid_: integration, dependency

**ADR**:
A short record of one hard-to-reverse decision and the reason for it.
_Avoid_: decision record, decision doc

### Glossary contents

**Term**:
A word or phrase a **Glossary** defines, with its definition and any **Avoided Words**.
_Avoid_: entry, word

**Avoided Word**:
A word that a **Term** rules out in favour of itself.
_Avoid_: synonym, alias, redirect

**Cluster**:
A named group of **Terms** under one subheading of a **Glossary**.
_Avoid_: group, section, category

**Statement**:
A paragraph, bullet, table row or quote of a **Glossary** or the **Context Map** that defines no **Term**, labelled by
the headings above it.
_Avoid_: passage

**Reference**:
A mention of a **Term** by its exact name, in bold or capitalised, in another **Term**'s definition or in a
**Statement**.
_Avoid_: link

**Co-mention**:
Two **Terms** that one **Statement** mentions, or that it and the heading just above it mention.
_Avoid_: association, related term

### Lookups

**Lookup**:
A search of a **Domain Model** for the **Terms** a word names or avoids, else for those the parts of a compound word
name or avoid, else for the **Terms** that mention it.
_Avoid_: search, query, find

### Checks

**Vocabulary Check**:
A comparison of a text with the **Terms** of a **Domain Model**, which lists the **Terms** and **Avoided Words** the
text uses, and its **Candidate Phrases**.
_Avoid_: check, lint, spellcheck

**Candidate Phrase**:
A run of two or more capitalised words in the text of a **Vocabulary Check** that matches no **Term**. It is either a
missing **Term** or a word to replace with an existing one.
_Avoid_: capitalised phrase, unknown word

**Health Check**:
An application of every **Health Rule** to a **Domain Model**, which reports each **Finding**.
_Avoid_: check, lint, validation

**Health Rule**:
One test of a **Glossary**'s quality: **Long Definition**, **Orphan**, **Unmapped Name**, or **Unresolved Phrase**.
_Avoid_: lint rule, check

**Finding**:
One **Term** or phrase that a **Health Check** reports because it breaks a **Health Rule**.
_Avoid_: issue, warning, error

**Long Definition**:
A **Term** whose definition runs longer than two sentences.
_Avoid_: verbose term

**Orphan**:
A **Term** that has no **Reference**.
_Avoid_: unused term, dead term

**Unmapped Name**:
A name that two **Contexts** both define as a **Term**, where no heading has **Statements** under it that mention
the name and name both **Contexts**.
_Avoid_: duplicate term

**Unresolved Phrase**:
A bold phrase that names neither a **Term** nor a **Context**, or a name in a bold label such as "Ordering →
Billing" that names no **Context**.
_Avoid_: broken link, dangling reference

## Relationships

- A **Lookup** of a **Term** gives its **References** both ways, its **Co-mentions**, and the **Statements** that
  mention it.
- The **Statements** under one heading that mention a name and name both **Contexts** that define it, typically
  **Context Relationships**, keep it from being an **Unmapped Name**.
