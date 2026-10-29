package glossary_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestLoadParsesTermsWithClusterAvoidAndLine(t *testing.T) {
	model := glossarytest.Load(t)
	tag := model.Terms["common/tag"]
	require.NotNil(t, tag)
	assert.Equal(t, "Tag", tag.Name)
	assert.Equal(t, "Image References", tag.Cluster)
	assert.Equal(t, "label", tag.Avoid)
	assert.Equal(t, 15, tag.Line)
	assert.Equal(t, []string{"common/image-ref", "common/tag", "common/repository"},
		model.Contexts[2].Clusters[0].TermIDs)
}

func TestRedirectsDropShortFormsOfTheTermName(t *testing.T) {
	model := glossarytest.Load(t)
	assert.Equal(t, []string{"image reference"}, model.Terms["common/image-ref"].Redirects)
	assert.Equal(t, []string{"repo", "path"}, model.Terms["common/repository"].Redirects)
	assert.Equal(t, []string{"container image", "image name"}, model.Terms["workspace/image"].Redirects)
}

func TestDefinitionLinksResolveInTheOwnContextThenInMapOrder(t *testing.T) {
	model := glossarytest.Load(t)
	assert.Equal(t, []string{"stacks/stack-image", "workspace/run"}, model.Terms["stacks/stack"].Refs,
		"Workspace precedes Common in the map")
	assert.Equal(t, []string{"common/unit-of-work"}, model.Terms["common/run"].Refs)
	assert.Equal(t, []string{"common/run"}, model.Terms["common/unit-of-work"].Refs, "a context's own term wins")
	stackImage := model.Terms["stacks/stack-image"]
	assert.Equal(t, []string{"stacks/stack", "workspace/image"}, stackImage.Refs)
	assert.Contains(t, stackImage.Definition, `<a class="t" href="#workspace/image" data-id="workspace/image">Image</a>`)
	assert.Equal(t, []string{"workspace/image", "common/tag"}, model.Terms["common/image-ref"].Refs)
}

func TestPluralBoldPhrasesResolveToTheSingularTerm(t *testing.T) {
	model := glossarytest.Load(t)
	assert.Equal(t, []string{"workspace/image"}, model.Terms["common/repository"].Refs)
}

func TestUnresolvedBoldPhrasesAreMarkedAndListed(t *testing.T) {
	model := glossarytest.Load(t)
	assert.Contains(t, model.Terms["common/repository"].Definition, `<span class="unres">Label</span>`)
	require.Len(t, model.Unresolved, 1)
	assert.Equal(t, glossary.Unresolved{Phrase: "Label", Path: "GLOSSARY.md", Context: "common", Where: "Repository"}, model.Unresolved[0])
}

func TestHeadNounPluralsResolveToTheirTerm(t *testing.T) {
	model := glossarytest.Load(t)
	run := model.Terms["common/run"]
	assert.Contains(t, run.Definition, `data-id="common/unit-of-work">Units of Work</a>`)
	assert.Equal(t, []string{"common/unit-of-work"}, run.Refs)
}

func TestSectionsOutsideLanguageAreNotRead(t *testing.T) {
	model := glossarytest.Load(t)
	for _, u := range model.Unresolved {
		assert.NotContains(t, []string{"Ghost", "Phantom", "Spectre"}, u.Phrase)
	}
	var clusters []string
	for _, cl := range model.Contexts[2].Clusters {
		clusters = append(clusters, cl.Name)
	}
	assert.Equal(t, []string{"Image References", "Run Tracking"}, clusters)
}

func TestTermsWithoutASubheadingLandInGeneral(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY.md": "# Ordering\n\nTakes orders.\n\n## Language\n\n**Order**:\nA request for goods.\n",
	})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	require.Len(t, model.Contexts[0].Clusters, 1)
	assert.Equal(t, "General", model.Contexts[0].Clusters[0].Name)
	assert.Equal(t, []string{"ordering/order"}, model.Contexts[0].Clusters[0].TermIDs)
	assert.Equal(t, "<p>Takes orders.</p>", model.Contexts[0].Intro)
}

func TestTablesAndTrailingParagraphsNestInsideTheirBullet(t *testing.T) {
	html := glossarytest.Load(t).ADRs[0].HTML
	assert.Contains(t, html, `<table class="md"><tr><th>Phase</th><th>Default</th></tr><tr><td>pre</td><td><code>true</code></td></tr></table>`)
	assert.Contains(t, html, `<p>Trailing note inside the bullet.</p></li></ul>`)
}

func TestADRsLinkGlossaryNamesAndKeepBoldSentencesPlain(t *testing.T) {
	model := glossarytest.Load(t)
	require.Len(t, model.ADRs, 1)
	adr := model.ADRs[0]
	assert.Equal(t, "adr/0001-pin-tags", adr.ID)
	assert.Equal(t, "Pin tags on Stack Images", adr.Title)
	assert.Equal(t, "docs/adr/0001-pin-tags.md", adr.Path)
	assert.Empty(t, adr.Context, "system-wide")
	assert.Equal(t, []string{"workspace/workspace", "workspace/run", "common/tag"}, adr.TermIDs)
	assert.Contains(t, adr.HTML, `<h3 class="mdh">Decision</h3>`)
	assert.Contains(t, adr.HTML, `<b>Every Stack Image is pinned.</b>`)
	assert.Contains(t, adr.HTML, `data-id="common/tag">Tags</a>`)
	assert.NotContains(t, adr.HTML, "unres")
	assert.Len(t, model.Unresolved, 1, "ADR bold is emphasis, never reported")
}

func TestLoadReportsAMissingGlossary(t *testing.T) {
	_, err := glossary.Load(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GLOSSARY.md")
}
