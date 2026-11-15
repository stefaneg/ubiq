# Read the Domain Model on every call

The MCP server reads the Glossaries, the Context Map and the ADRs afresh for every Lookup and Vocabulary Check,
rather than keeping the model in memory and watching the files. Agents edit these files during the session that asks
about them, so a cached model would answer from stale language. Reading is cheap: a cold `ubiq check` took about 35 ms
over a synthetic Glossary of 1,000 Terms and about 280 ms over 10,000, against agent turns of seconds. Revisit this
only if a real Domain Model makes a call slow enough to notice.
