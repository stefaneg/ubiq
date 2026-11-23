// Package glossarytest writes glossary repositories for tests.
package glossarytest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Root writes a three-context glossary, a context map and one ADR into a temp dir. Its Relationships, Flagged
// ambiguities and Cross-context vocabulary sections hold the Statements, including the unresolved Ghost, Phantom and
// Spectre.
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

## Relationships

- **Workspace → Stacks**: a **Stack Image** names a workspace **Image**.
- **Stacks ↔ Common**: a **Stack Image** is pinned by an
  **Image Ref**.

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

// Origin makes a bare repository in a temp dir whose default branch, main, holds one commit of the files under root,
// and returns its path: an origin for the Server to clone. Run it with git's user and system config shut out, as
// Isolate does.
func Origin(t *testing.T, root string) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin.git")
	Git(t, "", "init", "--quiet", "--bare", "--initial-branch=main", origin)
	Git(t, root, "init", "--quiet", "--initial-branch=main")
	Git(t, root, "add", "--all")
	Git(t, root, "-c", "user.name=Seed", "-c", "user.email=seed@example.com", "commit", "--quiet", "-m", "Seed")
	Git(t, root, "push", "--quiet", origin, "main")
	return origin
}

// Git runs the host's git in dir, the current directory when empty, and returns its trimmed output.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// Isolate shuts the user's and the system's git config out of every git the process runs, so that a developer's
// signing, hooks path or identity cannot change a test. Call it from TestMain.
func Isolate() {
	_ = os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_DIR"} {
		_ = os.Unsetenv(v)
	}
}
