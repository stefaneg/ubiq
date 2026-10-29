# Follow only the latest domain-modeling format

ubiq reads only what the latest
[domain-modeling skill](https://github.com/mattpocock/skills/tree/main/skills/engineering/domain-modeling) defines, and
ignores older sections such as a Glossary's Relationships, Example dialogue, and Flagged ambiguities, and the
Cross-context vocabulary that axis added to its Context Map. It also drops the axis tool's Shared Context and Home
Context, because the skill treats a root Glossary as an ordinary Context and root ADRs as system-wide. Supporting the
older formats too, as the axis tool does, would make ubiq disagree with the skill that writes the files it reads.
