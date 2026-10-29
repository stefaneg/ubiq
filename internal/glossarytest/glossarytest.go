// Package glossarytest writes glossary repositories for tests.
package glossarytest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Root writes a three-context glossary, a context map and one ADR into a temp dir. The Relationships, Flagged
// ambiguities and Cross-context vocabulary sections are not part of the domain-modeling format and are not read.
func Root(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"GLOSSARY.md": `# Common

Shared vocabulary.

## Language

### Image References

Intro for image refs.

**Image Ref**:
A reference to an **Image** by **Tag**.
_Avoid_: ref, image reference

**Tag**:
A mutable label on a **Repository**. Second sentence. Third sentence.
_Avoid_: label

**Repository**:
A path in a registry that holds **Images** under a **Label**.
_Avoid_: repo (short), path

### Run Tracking

**Run**:
One invocation, which starts **Units of Work**.
_Avoid_: execution

**Unit of Work**:
One process a **Run** starts.
_Avoid_: job

## Relationships

- A **Tag** moves between **Images** and a **Ghost**.

## Flagged ambiguities

- **Tag** also means a git **Phantom**.
`,
		"internal/workspace/GLOSSARY.md": `# Workspace

The imgctl core.

## Language

### Planning

**Image**:
An OCI image built from a **Workspace**.
_Avoid_: container image, image name

**Workspace**:
The directory tree imgctl builds from.
_Avoid_: project, repo

**Run**:
A workspace-scoped **Run**.
_Avoid_: job
`,
		"internal/stacks/GLOSSARY.md": `# Stacks

The envctl core.

## Language

### Environment Model

**Stack**:
A named **Stack Image** in an environment, deployed by a **Run**.
_Avoid_: deployment, deploy

**Stack Image**:
An image a **Stack** runs, identified by a workspace **Image**.
_Avoid_: stack container
`,
		"GLOSSARY-MAP.md": `# Context map

## Contexts

- [Workspace](internal/workspace/GLOSSARY.md) — the imgctl core.
- [Stacks](./internal/stacks/GLOSSARY.md): the envctl core.
- [Common](GLOSSARY.md) — shared by both, with clusters like [Image References](GLOSSARY.md#image-references).

| Package | Context |
| --- | --- |
| ` + "`internal/workspace/**`" + ` | Workspace |

## Cross-context vocabulary

### Deliberate mirrors

#### Run

- Workspace: a **Run** spans a **Spectre**.
`,
		"docs/adr/0001-pin-tags.md": `# Pin tags on Stack Images

## Decision

**Every Stack Image is pinned.** A Workspace Run records the Tags it used. Unknown Phrase stays plain.

- Phases:

  | Phase | Default |
  | ----- | ------- |
  | pre   | ` + "`true`" + `  |

  Trailing note inside the bullet.
`,
	}
	WriteTree(t, root, files)
	return root
}

// WriteTree writes files, keyed by slash-separated path relative to root.
func WriteTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

// Load loads the model from a fresh Root.
func Load(t *testing.T) *glossary.Model {
	t.Helper()
	model, err := glossary.Load(Root(t))
	require.NoError(t, err)
	return model
}
