package lookup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestFindNamesTheTermsAWordNamesInAnySpelling(t *testing.T) {
	l := New(glossarytest.Load(t))
	for _, word := range []string{"Stack Image", "stack images", "StackImage", "stack_image", "stack-image"} {
		assert.Equal(t, []string{"stacks/stack-image"}, l.Find(word).Named, word)
	}
	assert.Equal(t, []string{"common/run", "workspace/run"}, l.Find("run").Named, "every context's term of that name")
}

func TestFindNamesTheTermsThatAvoidAWord(t *testing.T) {
	l := New(glossarytest.Load(t))
	r := l.Find("repos")
	assert.Empty(t, r.Named)
	assert.Equal(t, []string{"common/repository", "workspace/workspace"}, r.Avoided)
	assert.Equal(t, []string{"common/image-ref"}, l.Find("ImageReference").Avoided)
}

func TestFindFallsBackToTheTermsThatMentionEveryWord(t *testing.T) {
	l := New(glossarytest.Load(t))
	assert.Equal(t, []string{"common/repository"}, l.Find("registry").Mentions)
	assert.Equal(t, []string{"workspace/image", "stacks/stack-image"}, l.Find("workspace images").Mentions,
		"Terms whose name has more of the words come first")
	assert.Empty(t, l.Find("stack").Mentions, "a word that names a Term needs no fallback")
	assert.Empty(t, l.Find("kubernetes").Mentions)
}

func TestResultWritesEverythingTheModelSaysAboutANamedTerm(t *testing.T) {
	var out strings.Builder
	New(glossarytest.Load(t)).Find("stack image").Write(&out)
	assert.Equal(t, `Stack Image (Stacks), internal/stacks/GLOSSARY.md:13
  An image a Stack runs, identified by a workspace Image.
  avoid: stack container
  refers to: Stack, Image (Workspace)
  referred to by: Stack
  relationship: Workspace → Stacks: a Stack Image names a workspace Image.
  relationship: Stacks ↔ Common: a Stack Image is pinned by an Image Ref.
`, out.String())
}

func TestResultNamesTheADRsThatMentionATerm(t *testing.T) {
	var out strings.Builder
	New(glossarytest.Load(t)).Find("tag").Write(&out)
	assert.Contains(t, out.String(), "\n  ADR: Pin tags on Stack Images (docs/adr/0001-pin-tags.md)\n")
}

func TestResultSaysWhichTermsToUseInsteadOfAnAvoidedWord(t *testing.T) {
	var out strings.Builder
	New(glossarytest.Load(t)).Find("deployments").Write(&out)
	assert.True(t, strings.HasPrefix(out.String(), `"deployments" is an Avoided Word. Use instead:
Stack (Stacks), internal/stacks/GLOSSARY.md:9
  A named Stack Image in an environment, deployed by a Run.
`), out.String())
}

func TestResultListsTheTermsThatMentionAWordItCannotName(t *testing.T) {
	l := New(glossarytest.Load(t))
	var out strings.Builder
	l.Find("registry").Write(&out)
	assert.Equal(t, `"registry" names no Term, and no Term avoids it. Terms that mention it:
  Repository (Common): A path in a registry that holds Images under a Label.
`, out.String())
	out.Reset()
	l.Find("kubernetes").Write(&out)
	assert.Equal(t, "\"kubernetes\" names no Term, no Term avoids it, and no Term mentions it.\n", out.String())
}
