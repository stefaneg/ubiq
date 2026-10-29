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
		{Path: "GLOSSARY.md", Rule: "unresolved", Name: "Label", Note: "Common: Repository"},
		{Path: "GLOSSARY.md", Line: 15, Rule: "long", Name: "Tag", Note: "3 sentences", Term: "common/tag"},
		{Path: "GLOSSARY.md", Line: 25, Rule: "unmapped", Name: "Run", Note: "also in Workspace", Term: "common/run"},
		{Path: "internal/workspace/GLOSSARY.md", Line: 17, Rule: "unmapped", Name: "Run", Note: "also in Common", Term: "workspace/run"},
	}, findings, "file-level findings first, then by line; a definition names Repository and a context relationship names Image Ref")
}

func TestCheckReportsAnOrphanThatOnlyAnADRMentions(t *testing.T) {
	model := glossarytest.Load(t)
	for id := range model.Terms {
		model.Terms[id].Refs = nil
	}
	model.Relationships = nil
	var orphans []string
	for _, f := range Check(model) {
		if f.Rule == "orphan" {
			orphans = append(orphans, f.Name)
		}
	}
	assert.Contains(t, orphans, "Workspace", "the ADR names Workspace; ADR mentions do not count")
	assert.Len(t, orphans, len(model.Terms))
}

func TestCheckIsSilentWhenClean(t *testing.T) {
	var out bytes.Buffer
	findings := Check(&glossary.Model{Terms: map[string]*glossary.Term{
		"common/run":  {ID: "common/run", Name: "Run", Definition: "One invocation.", Refs: []string{"common/step"}},
		"common/step": {ID: "common/step", Name: "Step", Definition: "One part of a run.", Refs: []string{"common/run"}},
	}})
	assert.Empty(t, findings)
	Write(&out, findings)
	assert.Empty(t, out.String())
	assert.Equal(t, "0 findings over 2 terms: 0 long, 0 orphan, 0 unmapped, 0 unresolved", Summary(findings, 2))
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
