package check

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestCheckRecognisesTermsIncludingPlurals(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("two Stack Images and a tag")
	require.Equal(t, []Finding{
		{Line: 1, Written: "Stack Image", TermIDs: []string{"stacks/stack-image"}},
		{Line: 1, Written: "tag", TermIDs: []string{"common/tag"}},
	}, report.Terms)
}

func TestCheckRecognisesHeadNounPlurals(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("the Units of Work still running")
	require.Equal(t, []Finding{
		{Line: 1, Written: "Units of Work", TermIDs: []string{"common/unit-of-work"}},
	}, report.Terms)
}

func TestCheckMatchesAdjacentPhrases(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("image tag")
	assert.Equal(t, "image", report.Terms[0].Written)
	assert.Equal(t, "tag", report.Terms[1].Written)
}

func TestCheckIgnoresWordsTheGlossaryAvoids(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("pushed to the repo")
	assert.Empty(t, report.Terms)
	assert.Empty(t, report.Candidates)
}

func TestCheckListsCapitalisedPhrasesWithoutAnEntry(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("the Build Cache Key of a Stack Image is stale")
	require.Equal(t, []Finding{{Line: 1, Written: "Build Cache Key"}}, report.Candidates)
}

func TestCheckListsEveryTermSharingAName(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("a run")
	require.Equal(t, []Finding{{Line: 1, Written: "run", TermIDs: []string{"common/run", "workspace/run"}}}, report.Terms)
}

func TestReportWritesTermsThenCandidates(t *testing.T) {
	report := New(glossarytest.Load(t)).Check("the repo holds a Stack Image\nthe Build Cache Key is a label")
	var out strings.Builder
	report.Write(&out)
	assert.Equal(t, `terms recognised: Stack Image (Stacks)
capitalised phrases without an entry: Build Cache Key
`, out.String())
}
