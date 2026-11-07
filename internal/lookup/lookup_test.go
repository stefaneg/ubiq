package lookup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
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
	assert.Equal(t, []string{"common/unit-of-work", "common/run"}, l.Find("units").Mentions,
		"Terms whose name has more of the words come first")
	assert.Empty(t, l.Find("stack").Mentions, "a word that names a Term needs no fallback")
	assert.Empty(t, l.Find("kubernetes").Mentions)
}

func TestFindSplitsACompoundIdentifierIntoTheTermsAndAvoidedWordsInside(t *testing.T) {
	l := New(glossarytest.Load(t))
	r := l.Find("StackImageResolver")
	assert.Equal(t, []Part{
		{Words: "stack image", Named: []string{"stacks/stack-image"}},
		{Words: "resolver"},
	}, r.Parts, "the longest run of words that names a Term wins over Stack alone")
	assert.Empty(t, r.Mentions, "a word with a matched part needs no fallback")
	assert.Equal(t, []Part{
		{Words: "repo", Avoided: []string{"common/repository", "workspace/workspace"}},
		{Words: "job", Avoided: []string{"common/unit-of-work", "workspace/run"}},
		{Words: "runner"},
	}, l.Find("RepoJobRunner").Parts, "Avoided Words, singulars included")
	assert.Equal(t, []Part{
		{Words: "workspace", Named: []string{"workspace/workspace"}},
		{Words: "images", Named: []string{"workspace/image"}},
	}, l.Find("workspace images").Parts)
	assert.Empty(t, l.Find("StackImage").Parts, "a word that names a Term as a whole is not split")
	assert.Empty(t, l.Find("ImageReference").Parts, "nor is an Avoided Word")
	assert.Empty(t, l.Find("KubernetesCluster").Parts, "a word with no matched part has none")
}

func TestFindReportsAPartThatBothNamesATermAndIsAvoided(t *testing.T) {
	l := New(shop(t))
	assert.Equal(t, []Part{
		{Words: "run", Named: []string{"shop/run"}, Avoided: []string{"shop/execution"}},
		{Words: "planner"},
	}, l.Find("RunPlanner").Parts)
}

// shop is a Glossary in which the word run both names a Term and is avoided by another.
func shop(t *testing.T) *glossary.Model {
	t.Helper()
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": `# Shop

## Language

**Run**:
A nightly batch.

**Execution**:
One attempt of a **Run**.
_Avoid_: run, attempt
`})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	return model
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

func TestResultWritesOneLinePerPart(t *testing.T) {
	l := New(glossarytest.Load(t))
	var out strings.Builder
	l.Find("StackImageRepoResolver").Write(&out)
	assert.Equal(t, `"StackImageRepoResolver" names no Term, and no Term avoids it. Its parts:
  stack image: Stack Image (Stacks), internal/stacks/GLOSSARY.md:13
  repo: an Avoided Word. Use instead: Repository (Common), GLOSSARY.md:19; Workspace (Workspace), internal/workspace/GLOSSARY.md:13
  resolver: names no Term, and no Term avoids it
`, out.String())
	out.Reset()
	New(shop(t)).Find("RunPlanner").Write(&out)
	assert.Equal(t, `"RunPlanner" names no Term, and no Term avoids it. Its parts:
  run: Run (Shop), GLOSSARY.md:5. Also an Avoided Word. Use instead: Execution (Shop), GLOSSARY.md:8
  planner: names no Term, and no Term avoids it
`, out.String())
}
