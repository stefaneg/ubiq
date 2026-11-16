package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/glossarytest"
)

// maintainer clones origin for a Maintainer to work in by hand and returns the clone, on the default branch.
func maintainer(t *testing.T, origin string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "maintainer")
	glossarytest.Git(t, "", "clone", "--quiet", origin, dir)
	glossarytest.Git(t, dir, "config", "user.name", "Maintainer")
	glossarytest.Git(t, dir, "config", "user.email", "maintainer@example.com")
	return dir
}

// commitFiles commits files in the Maintainer's clone dir on the branch checked out there and pushes it.
func commitFiles(t *testing.T, dir, subject string, files map[string]string) {
	t.Helper()
	glossarytest.WriteTree(t, dir, files)
	glossarytest.Git(t, dir, "add", "--all")
	glossarytest.Git(t, dir, "commit", "--quiet", "-m", subject)
	glossarytest.Git(t, dir, "push", "--quiet", "origin", "HEAD")
}

// develop is a developer's commit on the default branch, pushed, touching no glossary.
func develop(t *testing.T, origin string) {
	t.Helper()
	dir := maintainer(t, origin)
	notes := "notes after " + glossarytest.Git(t, dir, "rev-parse", "HEAD") + "\n"
	commitFiles(t, dir, "Notes", map[string]string{"NOTES.md": notes})
}

// mergeStyles merge a Batch into the default branch on origin the ways a Maintainer may.
var mergeStyles = map[string]func(t *testing.T, origin, batch string){
	"fast-forward": func(t *testing.T, origin, batch string) {
		dir := maintainer(t, origin)
		glossarytest.Git(t, dir, "merge", "--quiet", "--ff-only", "origin/"+batch)
		glossarytest.Git(t, dir, "push", "--quiet", "origin", "main")
	},
	"merge commit": func(t *testing.T, origin, batch string) {
		develop(t, origin)
		dir := maintainer(t, origin)
		glossarytest.Git(t, dir, "merge", "--quiet", "--no-ff", "--no-edit", "origin/"+batch)
		glossarytest.Git(t, dir, "push", "--quiet", "origin", "main")
	},
	"squash": func(t *testing.T, origin, batch string) {
		develop(t, origin)
		dir := maintainer(t, origin)
		glossarytest.Git(t, dir, "merge", "--quiet", "--squash", "origin/"+batch)
		glossarytest.Git(t, dir, "commit", "--quiet", "-m", "Squash "+batch)
		glossarytest.Git(t, dir, "push", "--quiet", "origin", "main")
	},
	"rebase": func(t *testing.T, origin, batch string) {
		develop(t, origin)
		dir := maintainer(t, origin)
		glossarytest.Git(t, dir, "checkout", "--quiet", "-b", "rebased", "origin/"+batch)
		glossarytest.Git(t, dir, "rebase", "--quiet", "main")
		glossarytest.Git(t, dir, "push", "--quiet", "origin", "rebased:main")
	},
}

func TestAfterAMaintainerMergesTheBatchTheNextChangeOpensTheNextBatchFromTheDefaultBranch(t *testing.T) {
	for style, merge := range mergeStyles {
		t.Run(style, func(t *testing.T) {
			origin := glossarytest.Origin(t, glossarytest.Root(t))
			url, _, _ := serveOrigin(t, origin, ada)
			cs := connect(t, url)
			define(t, cs, "Digest", "A content hash.")
			merge(t, origin, "ubiq/1")

			text := define(t, cs, "Registry", "A server that holds Repositories.")
			assert.Contains(t, text, "on the Batch ubiq/2.")
			assert.Equal(t, onOrigin(t, origin, "%H", "main"), onOrigin(t, origin, "%P", "ubiq/2"),
				"one commit on the default branch")
			assert.Equal(t, "Define Term Registry", onOrigin(t, origin, "%s", "ubiq/2"))
			require.Contains(t, glossarytest.Git(t, "", "--git-dir", origin, "show", "ubiq/2:GLOSSARY.md"), "**Digest**")
		})
	}
}

// openByHand pushes a Batch a Maintainer opens by hand from the default branch, with no Change on it.
func openByHand(t *testing.T, origin, batch string) {
	t.Helper()
	glossarytest.Git(t, maintainer(t, origin), "push", "--quiet", "origin", "main:refs/heads/"+batch)
}

func TestABatchOpenedByHandWithNoChangeIsReusedFromTheDefaultBranch(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	openByHand(t, origin, "ubiq/1")
	develop(t, origin)
	url, _, _ := serveOrigin(t, origin, ada)

	assert.Contains(t, define(t, connect(t, url), "Digest", "A content hash."), "on the Batch ubiq/1.")
	assert.Equal(t, onOrigin(t, origin, "%H", "main"), onOrigin(t, origin, "%P", "ubiq/1"))
}

func TestABatchOpenedByHandAfterAMergeIsNeverTakenAsMerged(t *testing.T) {
	for style, merge := range mergeStyles {
		t.Run(style, func(t *testing.T) {
			origin := glossarytest.Origin(t, glossarytest.Root(t))
			url, _, _ := serveOrigin(t, origin, ada)
			cs := connect(t, url)
			define(t, cs, "Digest", "A content hash.")
			merge(t, origin, "ubiq/1")
			openByHand(t, origin, "ubiq/2")
			develop(t, origin)

			assert.Contains(t, define(t, cs, "Registry", "A server that holds Repositories."), "on the Batch ubiq/2.")
			assert.Equal(t, onOrigin(t, origin, "%H", "main"), onOrigin(t, origin, "%P", "ubiq/2"))
			assert.NotContains(t, glossarytest.Git(t, "", "--git-dir", origin, "branch"), "ubiq/3")
		})
	}
}

// mirrored is text with the Term Mirror added after Unit of Work, where define_term puts a Term in Common.
func mirrored(text string) string {
	return strings.Replace(text, "_Avoid_: job\n", "_Avoid_: job\n\n**Mirror**:\nA copy.\n", 1)
}

// conflict lands Digest on ubiq/1 through cs and then has a developer add Mirror on the default branch at the same
// place, so that the Batch conflicts with the default branch.
func conflict(t *testing.T, cs *mcp.ClientSession, origin string) {
	t.Helper()
	define(t, cs, "Digest", "A content hash.")
	dir := maintainer(t, origin)
	text, err := os.ReadFile(filepath.Join(dir, "GLOSSARY.md"))
	require.NoError(t, err)
	commitFiles(t, dir, "Mirror", map[string]string{"GLOSSARY.md": mirrored(string(text))})
}

const refusal = "The Batch ubiq/1 conflicts with the default branch main in GLOSSARY.md, so Changes are refused " +
	"until a Maintainer merges main into ubiq/1 and pushes."

func TestAChangeWhileTheBatchConflictsIsRefusedAndAResolutionLetsTheNextOneLand(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, root, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	conflict(t, cs, origin)

	text, isErr := call(t, cs, "define_term", map[string]any{"context": "Common", "name": "Registry", "definition": "A server."})
	assert.True(t, isErr)
	assert.Equal(t, refusal, text)
	assert.Empty(t, glossarytest.Git(t, root, "status", "--porcelain"), "the merge is aborted")
	assert.Equal(t, "Define Term Digest", onOrigin(t, origin, "%s", "ubiq/1"))
	text, isErr = call(t, cs, "lookup", map[string]any{"words": []string{"Digest"}})
	assert.False(t, isErr)
	assert.Contains(t, text, "A content hash.", "reads continue")

	// A Maintainer resolves the conflict on the Batch, keeping both Terms.
	dir := maintainer(t, origin)
	glossarytest.Git(t, dir, "checkout", "--quiet", "-b", "ubiq/1", "origin/ubiq/1")
	assert.Error(t, exec.Command("git", "-C", dir, "merge", "--quiet", "origin/main").Run())
	batch := glossarytest.Git(t, dir, "show", "origin/ubiq/1:GLOSSARY.md")
	glossarytest.WriteTree(t, dir, map[string]string{"GLOSSARY.md": mirrored(batch + "\n")})
	glossarytest.Git(t, dir, "commit", "--quiet", "--all", "--no-edit")
	glossarytest.Git(t, dir, "push", "--quiet", "origin", "ubiq/1")

	assert.Contains(t, define(t, cs, "Registry", "A server."), "on the Batch ubiq/1.")
	glossary := glossarytest.Git(t, "", "--git-dir", origin, "show", "ubiq/1:GLOSSARY.md")
	for _, term := range []string{"**Digest**", "**Mirror**", "**Registry**"} {
		assert.Contains(t, glossary, term)
	}
}

func TestEmptyInputIsRefusedForWhatItIsEvenWhileTheBatchConflicts(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	conflict(t, cs, origin)
	cases := []struct {
		tool    string
		args    map[string]any
		refusal string
	}{
		{"define_term", map[string]any{"context": "Common", "name": " ", "definition": "A hash."}, "The Term's name is empty."},
		{"define_term", map[string]any{"context": "Common", "name": "Digest", "definition": " "}, "The definition of Digest is empty."},
		{"revise_term", map[string]any{"context": "Common", "name": "", "definition": "A hash."}, "The Term's name is empty."},
		{"revise_term", map[string]any{"context": "Common", "name": "Tag", "definition": ""}, "The definition of Tag is empty."},
		{"remove_term", map[string]any{"context": "Common", "name": " "}, "The Term's name is empty."},
		{"record_adr", map[string]any{"title": " ", "text": "We pin."}, "The ADR's title is empty."},
		{"record_adr", map[string]any{"title": "Pin digests", "text": " "}, "The text of the ADR Pin digests is empty."},
		{"record_adr", map[string]any{"title": "?!", "text": "We pin."}, "The title ?! has no letters or digits to name the file by."},
	}
	for _, c := range cases {
		text, isErr := call(t, cs, c.tool, c.args)
		assert.True(t, isErr, c.refusal)
		assert.Equal(t, c.refusal, text)
	}
}

func TestAConflictFoundAtStartupIsLoggedAndReadsKeepAnswering(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	first, _, _ := serveOrigin(t, origin, ada)
	conflict(t, connect(t, first), origin)

	url, _, log := serveOrigin(t, origin, ada)
	assert.Contains(t, log.String(), refusal+"\n")
	status, _ := get(t, url)
	assert.Equal(t, 200, status)
	cs := connect(t, url)
	text, isErr := call(t, cs, "lookup", map[string]any{"words": []string{"Digest"}})
	assert.False(t, isErr)
	assert.Contains(t, text, "A content hash.", "the clone serves the Batch as it stands")
	text, _ = call(t, cs, "define_term", map[string]any{"context": "Common", "name": "Registry", "definition": "A server."})
	assert.Equal(t, refusal, text)
}

// serveRepository stands the Server up over a clone of origin, as serveOrigin does, and returns its URL, its Batch
// repository and what it logs.
func serveRepository(t *testing.T, origin string) (string, *batch.Repository, *logBuffer) {
	t.Helper()
	log := &logBuffer{}
	root := filepath.Join(t.TempDir(), "clone")
	repo, err := batch.Open(root, origin, bot, log)
	require.NoError(t, err)
	return serveWith(t, Options{Root: root, People: ada, Writers: writers, Batches: repo}, log), repo, log
}

func TestDevelopersCommitsOnTheDefaultBranchShowAfterARefreshWithNoChange(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, repo, _ := serveRepository(t, origin)
	cs := connect(t, url)
	define(t, cs, "Digest", "A content hash.")
	dir := maintainer(t, origin)
	stacks, err := os.ReadFile(filepath.Join(dir, "internal/stacks/GLOSSARY.md"))
	require.NoError(t, err)
	commitFiles(t, dir, "Mirror", map[string]string{"internal/stacks/GLOSSARY.md": string(stacks) + "\n**Mirror**:\nA copy.\n"})

	repo.Refresh()

	text, isErr := call(t, cs, "lookup", map[string]any{"words": []string{"Mirror", "Digest"}})
	assert.False(t, isErr)
	assert.Contains(t, text, "A copy.")
	assert.Contains(t, text, "A content hash.", "the Batch still shows")
	_, page := get(t, url)
	assert.Contains(t, page, "A copy.")
}

func TestARefreshLogsAConflictOnceUntilItChanges(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	first, _, _ := serveOrigin(t, origin, ada)
	conflict(t, connect(t, first), origin)

	_, repo, log := serveRepository(t, origin)
	repo.Refresh()
	repo.Refresh()
	assert.Equal(t, 1, strings.Count(log.String(), refusal))
}
