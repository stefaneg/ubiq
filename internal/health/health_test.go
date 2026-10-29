package health

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestSentencesIgnoresAbbreviationsAndTrailingSpace(t *testing.T) {
	assert.Equal(t, 1, sentences("One sentence."))
	assert.Equal(t, 2, sentences("One. Two"))
	assert.Equal(t, 3, sentences("A mutable label. Second sentence. Third sentence."))
	assert.Equal(t, 1, sentences("Names, e.g. a tag, are mutable."))
	assert.Equal(t, 2, sentences("Is it? It is! "))
	assert.Equal(t, 1, sentences("Reads v1.2.3 as a tag."))
}

func TestCheckAppliesThePageRulesAndSortsByFileAndLine(t *testing.T) {
	findings := Check(glossarytest.Load(t))
	assert.Equal(t, []Finding{
		{Path: "GLOSSARY.md", Rule: "unresolved", Name: "Label", Note: "Common: Image References ambiguities"},
		{Path: "GLOSSARY.md", Line: 15, Rule: "long", Name: "Tag", Note: "3 sentences", Term: "common/tag"},
	}, findings, "file-level findings first, then by line; Repository is named by Tag's definition")
}

func TestCheckReportsAnOrphanThatOnlyAnADRMentions(t *testing.T) {
	model := glossarytest.Load(t)
	for id := range model.Terms {
		model.Terms[id].Refs = nil
	}
	model.Notes, model.MapEntries = nil, nil
	var orphans []string
	for _, f := range Check(model) {
		if f.Rule == "orphan" {
			orphans = append(orphans, f.Name)
		}
	}
	assert.Contains(t, orphans, "Workspace", "the ADR names Workspace; ADR mentions do not count")
	assert.Len(t, orphans, len(model.Terms))
}

func TestCheckReportsTwoContextsSharingANameTheMapDoesNotList(t *testing.T) {
	model := glossarytest.Load(t)
	model.MapEntries = model.MapEntries[1:] // drop the Run mirror
	var unmapped []Finding
	for _, f := range Check(model) {
		if f.Rule == "unmapped" {
			unmapped = append(unmapped, f)
		}
	}
	require.Len(t, unmapped, 2)
	assert.Equal(t, Finding{Path: "GLOSSARY.md", Line: 35, Rule: "unmapped", Name: "Run", Note: "also in Workspace", Term: "common/run"}, unmapped[0])
	assert.Equal(t, "internal/workspace/GLOSSARY.md", unmapped[1].Path)
	assert.Equal(t, "also in Common", unmapped[1].Note)
}

func TestCheckIsSilentWhenClean(t *testing.T) {
	var out bytes.Buffer
	findings := Check(&glossary.Model{Terms: map[string]*glossary.Term{"common/run": {ID: "common/run", Name: "Run", Definition: "One invocation."}},
		Notes: []glossary.Note{{Kind: "relationship", TermIDs: []string{"common/run"}}}})
	assert.Empty(t, findings)
	Write(&out, findings)
	assert.Empty(t, out.String())
	assert.Equal(t, "0 findings over 1 terms: 0 long, 0 orphan, 0 unmapped, 0 unresolved", Summary(findings, 1))
}

func TestCheckNamesContextsByTheirHeading(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY.md": "# Course Video Manager\n\n## Language\n\n**Clip**:\nA cut of a **Take**.\n",
	})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	findings := Check(model)
	require.Len(t, findings, 2)
	assert.Equal(t, "Course Video Manager: Clip", findings[0].Note)
}
