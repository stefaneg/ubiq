package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModelParsesTermsWithClusterAvoidAndLine(t *testing.T) {
	model := loadFixture(t)
	tag := model.Terms["common/tag"]
	require.NotNil(t, tag)
	assert.Equal(t, "Tag", tag.Name)
	assert.Equal(t, "Image References", tag.Cluster)
	assert.Equal(t, "label", tag.Avoid)
	assert.Equal(t, 15, tag.Line)
	assert.Equal(t, []string{"common/image-ref", "common/tag", "common/repository"},
		model.Contexts[0].Clusters[0].TermIDs)
}

func TestRedirectsDropShortFormsOfTheTermName(t *testing.T) {
	model := loadFixture(t)
	assert.Equal(t, []string{"image reference"}, model.Terms["common/image-ref"].Redirects)
	assert.Equal(t, []string{"repo", "path"}, model.Terms["common/repository"].Redirects)
	assert.Equal(t, []string{"container image", "image name"}, model.Terms["workspace/image"].Redirects)
}

func TestDefinitionLinksResolveOwnContextThenCommonThenOtherCore(t *testing.T) {
	model := loadFixture(t)
	stackImage := model.Terms["stacks/stack-image"]
	assert.Equal(t, []string{"stacks/stack", "workspace/image"}, stackImage.Refs)
	assert.Contains(t, stackImage.Definition, `<a class="t" href="#workspace/image" data-id="workspace/image">Image</a>`)
	assert.Equal(t, []string{"workspace/image", "common/tag"}, model.Terms["common/image-ref"].Refs)
}

func TestPluralBoldPhrasesResolveToTheSingularTerm(t *testing.T) {
	model := loadFixture(t)
	rel := model.Notes[0]
	assert.Equal(t, "relationship", rel.Kind)
	assert.Equal(t, "Image References", rel.Cluster)
	assert.Equal(t, []string{"common/tag", "workspace/image"}, rel.TermIDs)
}

func TestUnresolvedBoldPhrasesAreMarkedAndListed(t *testing.T) {
	model := loadFixture(t)
	amb := model.Notes[1]
	assert.Equal(t, "ambiguity", amb.Kind)
	assert.Contains(t, amb.HTML, `<span class="unres">Label</span>`)
	require.Len(t, model.Unresolved, 1)
	assert.Equal(t, Unresolved{Phrase: "Label", Context: "common", Where: "Image References ambiguities"}, model.Unresolved[0])
}

func TestHeadNounPluralsResolveToTheirTerm(t *testing.T) {
	model := loadFixture(t)
	var rel Note
	for _, n := range model.Notes {
		if n.Context == "common" && n.Cluster == "Run Tracking" && n.Kind == "relationship" {
			rel = n
		}
	}
	assert.Contains(t, rel.HTML, `data-id="common/unit-of-work">Units of Work</a>`)
	assert.Equal(t, []string{"common/run", "common/unit-of-work"}, rel.TermIDs)
}

func TestCoreFileTopLevelAmbiguitiesLandInGeneral(t *testing.T) {
	model := loadFixture(t)
	var general []Note
	for _, n := range model.Notes {
		if n.Context == "workspace" && n.Kind == "ambiguity" {
			general = append(general, n)
		}
	}
	require.Len(t, general, 1)
	assert.Equal(t, "General", general[0].Cluster)
	assert.Equal(t, []string{"workspace/run"}, general[0].TermIDs)
}

func TestContextMapEntriesCarryKindAndHeadingTerms(t *testing.T) {
	model := loadFixture(t)
	require.Len(t, model.MapEntries, 3)
	run, tag, anchor := model.MapEntries[0], model.MapEntries[1], model.MapEntries[2]
	assert.Equal(t, "mirror", run.Kind)
	assert.Equal(t, "Run", run.Name)
	assert.Equal(t, []string{"workspace/run", "workspace/workspace", "common/run", "stacks/stack"}, run.TermIDs)
	assert.Equal(t, "collision", tag.Kind)
	assert.Equal(t, "anchor", anchor.Kind)
	assert.Equal(t, "Image Ref", anchor.Name)
	assert.Contains(t, anchor.HTML, `href="#common/image-ref"`)
}

func TestTablesAndTrailingParagraphsNestInsideTheirBullet(t *testing.T) {
	model := loadFixture(t)
	html := model.MapEntries[0].HTML
	assert.Contains(t, html, `<table class="md"><tr><th>Phase</th><th>Default</th></tr><tr><td>pre</td><td><code>true</code></td></tr></table>`)
	assert.Contains(t, html, `<p>Trailing note inside the bullet.</p></li></ul>`)
}

func TestADRsLinkGlossaryNamesAndKeepBoldSentencesPlain(t *testing.T) {
	model := loadFixture(t)
	require.Len(t, model.ADRs, 1)
	adr := model.ADRs[0]
	assert.Equal(t, "adr/0001-pin-tags", adr.ID)
	assert.Equal(t, "Pin tags on Stack Images", adr.Title)
	assert.Equal(t, "docs/adr/0001-pin-tags.md", adr.Path)
	assert.Equal(t, "workspace", adr.Context)
	assert.Equal(t, []string{"workspace/workspace", "workspace/run", "common/tag"}, adr.TermIDs)
	assert.Contains(t, adr.HTML, `<h3 class="mdh">Decision</h3>`)
	assert.Contains(t, adr.HTML, `<b>Every Stack Image is pinned.</b>`)
	assert.Contains(t, adr.HTML, `data-id="common/tag">Tags</a>`)
	assert.NotContains(t, adr.HTML, "unres")
	assert.Len(t, model.Unresolved, 1, "ADR bold is emphasis, never reported")
}

func TestLoadModelReportsAMissingGlossary(t *testing.T) {
	_, err := LoadModel(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GLOSSARY.md")
}
