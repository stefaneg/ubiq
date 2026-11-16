# A Change lands with its Findings

The Server could run the Health Check on every write and refuse any Change that introduces a Finding, but it lands
the Change and returns the Findings it introduced as warnings instead. An Agent defining several Terms must be free
to reference one before it has defined it, a removed Term may leave Unresolved Phrases that the next Change repairs,
and every Finding is already visible on the page and in `health`, so a gate would only force the Agent to order its
work around the checker. A write refuses only for reasons the Domain Model cannot absorb: a Reader calling it, a
define that names an existing Term or a revise or remove that names a missing one, an unknown Context, empty input,
or a Batch in conflict with the default branch.
