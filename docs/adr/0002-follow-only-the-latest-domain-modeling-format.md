---
status: superseded in part by ADR-0004, which reads every section
---

# Follow only the latest domain-modeling format

ubiq reads only what the latest
[domain-modeling skill](https://github.com/mattpocock/skills/tree/main/skills/engineering/domain-modeling) defines, and
ignores older sections such as a Glossary's Relationships, Example dialogue, and Flagged ambiguities, and the
Cross-context vocabulary that some Context Maps add. It also drops an older format's Shared Context and Home Context,
because the skill treats a root Glossary as an ordinary Context and root ADRs as system-wide. Supporting the older
formats too would make ubiq disagree with the skill that writes the files it reads.
