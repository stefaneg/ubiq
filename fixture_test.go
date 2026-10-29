package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixtureRoot writes a three-context glossary, a context map and one ADR into a temp dir.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"GLOSSARY.md": `# Common

Shared vocabulary.

## Image References

Intro for image refs.

### Language

**Image Ref**:
A reference to an **Image** by **Tag**.
_Avoid_: ref, image reference

**Tag**:
A mutable label on a **Repository**. Second sentence. Third sentence.
_Avoid_: label

**Repository**:
A path in a registry.
_Avoid_: repo (short), path

### Relationships

- A **Tag** moves between **Images**.

### Flagged ambiguities

- **Tag** also means a git **Label**.

## Run Tracking

### Language

**Run**:
One invocation.
_Avoid_: execution

**Unit of Work**:
One process a **Run** starts.
_Avoid_: job

### Relationships

- A **Run** starts zero or more **Units of Work**.
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

## Relationships

### Planning

- An **Image** is produced in a **Run**.

## Flagged ambiguities

- **Run** is defined in Common too.
`,
		"internal/stacks/GLOSSARY.md": `# Stacks

The envctl core.

## Language

### Environment Model

**Stack**:
A named **Stack Image** in an environment.
_Avoid_: deployment, deploy

**Stack Image**:
An image a **Stack** runs, identified by a workspace **Image**.
_Avoid_: stack container
`,
		"GLOSSARY-MAP.md": `# Context map

## Contexts

| Package | Context |
| --- | --- |
| ` + "`internal/workspace/**`" + ` | Workspace |

## Cross-context vocabulary

### Deliberate mirrors

#### Run

- Workspace: a **Run** spans a **Workspace**.
- Stacks: a **Run** spans a **Stack**.

  | Phase | Default |
  | ----- | ------- |
  | pre   | ` + "`true`" + `  |

  Trailing note inside the bullet.

### True collisions

#### Tag

The common **Tag** versus a git tag.

### Common anchors

**[Image Ref](GLOSSARY.md#image-references)** — the **Image Ref** and its **Tag**.
`,
		"docs/adr/0001-pin-tags.md": `# Pin tags on Stack Images

## Decision

**Every Stack Image is pinned.** A Workspace Run records the Tags it used. Unknown Phrase stays plain.
`,
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

func loadFixture(t *testing.T) *Model {
	t.Helper()
	model, err := LoadModel(fixtureRoot(t))
	require.NoError(t, err)
	return model
}
