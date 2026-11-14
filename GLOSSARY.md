# ubiq

ubiq reads the **Domain Model** a repository keeps as Markdown, renders it as one page, and reports where its language
is unclear or broken.

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
The document that lists a repository's **Contexts**, where each one's **Glossary** lives, and each
**Context Relationship**.
_Avoid_: glossary map, index

**Context Relationship**:
A statement in the **Context Map** of how two **Contexts** relate.
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

**Reference**:
A mention of a **Term** by its exact name, in bold or capitalised, in another **Term**'s definition or in a
**Context Relationship**.
_Avoid_: link

### Checks

**Vocabulary Check**:
A comparison of a text with the **Terms** of a **Domain Model**, which lists the **Terms** the text uses and its
**Candidate Phrases**.
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
A name that two **Contexts** both define as a **Term** and that no **Context Relationship** between them
mentions.
_Avoid_: duplicate term

**Unresolved Phrase**:
A bold phrase that names no **Term**, or a **Context Relationship** label that names no **Context**.
_Avoid_: broken link, dangling reference

### Changes

**Change**:
One accepted write to the **Domain Model** that an **Agent** makes for a **Person**, recorded as one commit.
_Avoid_: edit, write, revision, update

**Batch**:
The sequence of **Changes** gathered on one numbered branch, awaiting a **Maintainer**'s merge into the default
branch.
_Avoid_: round, change set, pending changes

**Maintainer**:
A **Person** who merges a **Batch** into the default branch. The repository, not ubiq, decides who that is.
_Avoid_: admin, owner, reviewer

### People

**Identity Provider**:
The external OpenID Connect provider a deployment trusts to say who a person is.
_Avoid_: IdP, SSO, login provider

**Person**:
A human the **Identity Provider** has identified. Everything a served ubiq does, it does for a Person.
_Avoid_: user, account, principal

**Agent**:
An MCP client acting for a **Person**.
_Avoid_: client, tool, bot
