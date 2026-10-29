package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentencesIgnoresAbbreviationsAndTrailingSpace(t *testing.T) {
	assert.Equal(t, 1, sentences("One sentence."))
	assert.Equal(t, 2, sentences("One. Two"))
	assert.Equal(t, 3, sentences("A mutable label. Second sentence. Third sentence."))
	assert.Equal(t, 1, sentences("Names, e.g. a tag, are mutable."))
	assert.Equal(t, 2, sentences("Is it? It is! "))
	assert.Equal(t, 1, sentences("Reads v1.2.3 as a tag."))
}

func TestHealthAppliesThePageRulesAndSortsByFileAndLine(t *testing.T) {
	findings := Health(loadFixture(t))
	assert.Equal(t, []HealthFinding{
		{Path: "GLOSSARY.md", Rule: "unresolved", Name: "Label", Note: "Common: Image References ambiguities"},
		{Path: "GLOSSARY.md", Line: 15, Rule: "long", Name: "Tag", Note: "3 sentences"},
	}, findings, "file-level findings first, then by line; Repository is named by Tag's definition")
}

func TestHealthReportsAnOrphanThatOnlyAnADRMentions(t *testing.T) {
	model := loadFixture(t)
	for id := range model.Terms {
		model.Terms[id].Refs = nil
	}
	model.Notes, model.MapEntries = nil, nil
	var orphans []string
	for _, f := range Health(model) {
		if f.Rule == "orphan" {
			orphans = append(orphans, f.Name)
		}
	}
	assert.Contains(t, orphans, "Workspace", "the ADR names Workspace; ADR mentions do not count")
	assert.Len(t, orphans, len(model.Terms))
}

func TestHealthReportsTwoContextsSharingANameTheMapDoesNotList(t *testing.T) {
	model := loadFixture(t)
	model.MapEntries = model.MapEntries[1:] // drop the Run mirror
	var unmapped []HealthFinding
	for _, f := range Health(model) {
		if f.Rule == "unmapped" {
			unmapped = append(unmapped, f)
		}
	}
	require.Len(t, unmapped, 2)
	assert.Equal(t, HealthFinding{Path: "GLOSSARY.md", Line: 35, Rule: "unmapped", Name: "Run", Note: "also in Workspace"}, unmapped[0])
	assert.Equal(t, "internal/workspace/GLOSSARY.md", unmapped[1].Path)
	assert.Equal(t, "also in Common", unmapped[1].Note)
}

func TestRunHealthPrintsFindingsAndExitsOne(t *testing.T) {
	root := fixtureRoot(t)
	var out, status bytes.Buffer
	err := run([]string{"health", "-root", root}, nil, &out, &status)
	require.True(t, errors.Is(err, errHealthFindings))
	assert.Equal(t, "GLOSSARY.md: unresolved: Label: Common: Image References ambiguities\n"+
		"GLOSSARY.md:15: long: Tag: 3 sentences\n", out.String())
	assert.Equal(t, "2 findings over 10 terms: 1 long, 0 orphan, 0 unmapped, 1 unresolved\n", status.String())
}

func TestRunHealthIsSilentAndSucceedsWhenClean(t *testing.T) {
	var out bytes.Buffer
	findings := Health(&Model{Terms: map[string]*Term{"common/run": {ID: "common/run", Name: "Run", Definition: "One invocation."}},
		Notes: []Note{{Kind: "relationship", TermIDs: []string{"common/run"}}}})
	assert.Empty(t, findings)
	WriteHealth(&out, findings)
	assert.Empty(t, out.String())
	assert.Equal(t, "0 findings over 1 terms: 0 long, 0 orphan, 0 unmapped, 0 unresolved", healthSummary(findings, 1))
}
