package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/glossarytest"
	"github.com/stefaneg/ubiq/internal/person"
)

var (
	// bot is the Server's identity, the committer of every Change.
	bot = batch.Identity{Name: "ubiq-bot", Email: "bot@example.com"}
	// bob is a Reader.
	bob = resolver{p: person.Person{Name: "Bob Reader", Email: "bob@example.com", Role: person.Reader}}
)

// serveOrigin stands the Server up over a clone of origin it makes in a temp dir, learning every Person from r, and
// returns its URL, the clone and what it logs.
func serveOrigin(t *testing.T, origin string, r person.Resolver) (string, string, *logBuffer) {
	t.Helper()
	log := &logBuffer{}
	root := filepath.Join(t.TempDir(), "clone")
	repo, err := batch.Open(root, origin, bot, log)
	require.NoError(t, err)
	return serveWith(t, Options{Root: root, People: r, Writers: writers, Batches: repo}, log), root, log
}

// onOrigin is what git says about rev on origin, in format.
func onOrigin(t *testing.T, origin, format, rev string) string {
	t.Helper()
	return glossarytest.Git(t, "", "--git-dir", origin, "log", "-1", "--format="+format, rev)
}

func TestTheFourWriteToolsAreListedForAReaderAsWritesBesideTheReadTools(t *testing.T) {
	url, _ := serve(t, glossarytest.Root(t), bob)
	tools, err := connect(t, url).ListTools(context.Background(), nil)
	require.NoError(t, err)
	readOnly := map[string]bool{}
	for _, tool := range tools.Tools {
		readOnly[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}
	assert.Equal(t, map[string]bool{
		"lookup": true, "check": true, "health": true, "whoami": true,
		"define_term": false, "revise_term": false, "remove_term": false, "record_adr": false,
	}, readOnly)
}

func TestADefineByAWriterLandsOneCommitOnTheFirstBatch(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	main := onOrigin(t, origin, "%H", "main")
	url, _, _ := serveOrigin(t, origin, ada)

	text, isErr := call(t, connect(t, url), "define_term", map[string]any{
		"context": "Common", "name": "Digest", "definition": "A content hash that pins an Image.",
	})
	require.False(t, isErr, text)

	commit := onOrigin(t, origin, "%H", "ubiq/1")
	assert.Equal(t, main, onOrigin(t, origin, "%P", "ubiq/1"), "one commit on the default branch")
	assert.Equal(t, "Ada Lovelace|ada@example.com|ubiq-bot|bot@example.com|Define Term Digest|",
		onOrigin(t, origin, "%an|%ae|%cn|%ce|%s|%b", "ubiq/1"))
	assert.Contains(t, text, commit)
	assert.Contains(t, text, "ubiq/1")
}

func TestTheNextLookupFindsTheDefinedTermWithItsCluster(t *testing.T) {
	url, _, _ := serveOrigin(t, glossarytest.Origin(t, glossarytest.Root(t)), ada)
	cs := connect(t, url)
	text, isErr := call(t, cs, "define_term", map[string]any{
		"context": "Common", "cluster": "Image References", "name": "Digest",
		"definition": "A content hash that pins an Image.", "avoid": []string{"checksum"},
	})
	require.False(t, isErr, text)
	entry := "Digest (Common › Image References), GLOSSARY.md:23\n" +
		"  A content hash that pins an Image.\n" +
		"  avoid: checksum\n" +
		"  refers to: Image (Workspace)\n"
	assert.Contains(t, text, "\n\n"+entry, "the result holds the entry as it now reads")

	text, isErr = call(t, cs, "lookup", map[string]any{"words": []string{"Digest"}})
	assert.False(t, isErr)
	assert.Equal(t, entry, text)
}

// define calls define_term in Common and requires it to land.
func define(t *testing.T, cs *mcp.ClientSession, name, definition string) string {
	t.Helper()
	text, isErr := call(t, cs, "define_term", map[string]any{"context": "Common", "name": name, "definition": definition})
	require.False(t, isErr, text)
	return text
}

func TestASecondDefineLandsOnTheSameBatch(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	define(t, cs, "Digest", "A content hash.")
	assert.Contains(t, define(t, cs, "Registry", "A server that holds Repositories."), "on the Batch ubiq/1.")
	assert.Equal(t, "Define Term Registry\nDefine Term Digest\nSeed",
		glossarytest.Git(t, "", "--git-dir", origin, "log", "--format=%s", "ubiq/1"))
}

func TestAReadersDefineIsRefusedNamingTheWriterRule(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, bob)
	text, isErr := call(t, connect(t, url), "define_term",
		map[string]any{"context": "Common", "name": "Digest", "definition": "A content hash."})
	assert.True(t, isErr)
	assert.Equal(t, "Bob Reader is a Reader here, and only a Person whose groups claim holds glossary-writers may make Changes.", text)
	assert.NotContains(t, glossarytest.Git(t, "", "--git-dir", origin, "branch"), "ubiq/")
}

func TestADefineIsRefusedNamingTheTermOrField(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	for args, refusal := range map[*map[string]any]string{
		{"context": "Common", "name": "tag", "definition": "A label."}:    "The Term Tag already exists in Common.",
		{"context": "Common", "name": " ", "definition": "A label."}:      "The Term's name is empty.",
		{"context": "Common", "name": "Digest", "definition": " "}:        "The definition of Digest is empty.",
		{"context": "Billing", "name": "Digest", "definition": "A hash."}: "No Context Billing; this repository has Workspace, Stacks and Common.",
		{"name": "Digest", "definition": "A hash."}:                       "Name the Context, since this repository has Workspace, Stacks and Common.",
	} {
		text, isErr := call(t, cs, "define_term", *args)
		assert.True(t, isErr, refusal)
		assert.Equal(t, refusal, text)
	}
	assert.NotContains(t, glossarytest.Git(t, "", "--git-dir", origin, "log", "--all", "--format=%s"), "Define Term")
}

func TestADefineNeedsNoContextWhenTheRepositoryHasOne(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": "# Shop\n\n## Language\n\n**Order**:\nA request for goods.\n"})
	url, _, _ := serveOrigin(t, glossarytest.Origin(t, root), ada)
	text, isErr := call(t, connect(t, url), "define_term", map[string]any{"name": "Customer", "definition": "Places an Order."})
	require.False(t, isErr, text)
	assert.Contains(t, text, "Customer (Shop), GLOSSARY.md:8\n  Places an Order.\n")
}

func TestAPushRejectedBecauseOriginMovedIsRetriedAfterAFetch(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, root, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	define(t, cs, "Digest", "A content hash.")

	// A Maintainer's commit reaches the Batch on origin while the Server pushes its next Change: the Server's
	// pre-push hook pushes it first, once.
	other := filepath.Join(t.TempDir(), "other")
	glossarytest.Git(t, "", "clone", "--quiet", "--branch", "ubiq/1", origin, other)
	glossarytest.WriteTree(t, other, map[string]string{"NOTES.md": "notes\n"})
	glossarytest.Git(t, other, "add", "NOTES.md")
	glossarytest.Git(t, other, "-c", "user.name=Maintainer", "-c", "user.email=m@example.com", "commit", "--quiet", "-m", "Notes")
	marker := filepath.Join(t.TempDir(), "pushed")
	glossarytest.WriteTree(t, root, map[string]string{".git/hooks/pre-push": "#!/bin/sh\n" +
		"unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE\n" +
		"[ -e '" + marker + "' ] && exit 0\n" +
		"touch '" + marker + "'\n" +
		"git -C '" + other + "' push --quiet origin ubiq/1\n"})
	require.NoError(t, os.Chmod(filepath.Join(root, ".git/hooks/pre-push"), 0o755))

	define(t, cs, "Registry", "A server that holds Repositories.")
	assert.Equal(t, "Define Term Registry\nNotes\nDefine Term Digest\nSeed",
		glossarytest.Git(t, "", "--git-dir", origin, "log", "--format=%s", "ubiq/1"))
}

func TestAPushRejectedTwiceIsRefusedAndReadsNeverShowTheChange(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, root, log := serveOrigin(t, origin, ada)
	glossarytest.WriteTree(t, root, map[string]string{".git/hooks/pre-push": "#!/bin/sh\necho 'origin is read-only' >&2\nexit 1\n"})
	require.NoError(t, os.Chmod(filepath.Join(root, ".git/hooks/pre-push"), 0o755))
	cs := connect(t, url)

	text, isErr := call(t, cs, "define_term", map[string]any{"context": "Common", "name": "Digest", "definition": "A content hash."})
	assert.True(t, isErr)
	assert.Equal(t, "Origin refused the push to the Batch ubiq/1 twice, so the Change did not land.", text)
	assert.Contains(t, log.String(), "origin is read-only", "the reason is logged for a Maintainer")
	text, isErr = call(t, cs, "lookup", map[string]any{"words": []string{"Digest"}})
	assert.False(t, isErr)
	assert.NotContains(t, text, "A content hash.")
	_, page := get(t, url)
	assert.NotContains(t, page, "A content hash.")
}

func TestTwoConcurrentDefinesBothLand(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	var wg sync.WaitGroup
	for _, name := range []string{"Digest", "Registry"} {
		cs := connect(t, url)
		wg.Go(func() {
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "define_term",
				Arguments: map[string]any{"context": "Common", "name": name, "definition": "A thing."}})
			if assert.NoError(t, err) {
				assert.False(t, res.IsError, name)
			}
		})
	}
	wg.Wait()
	assert.ElementsMatch(t, []string{"Define Term Digest", "Define Term Registry", "Seed"},
		strings.Split(glossarytest.Git(t, "", "--git-dir", origin, "log", "--format=%s", "ubiq/1"), "\n"))
}

func TestADefineThatIntroducesFindingsLandsAndReturnsExactlyThose(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	// Tag's long definition and the unresolved Ghost were there before, and are not the Change's.
	text := define(t, connect(t, url), "Stack", "A deployed unit that pins a Tag. It stops. It ends.")
	assert.True(t, strings.HasSuffix(text, "\nFindings the Change introduced:\n"+
		"GLOSSARY.md:33: long: Stack: 3 sentences\n"+
		"GLOSSARY.md:33: orphan: Stack: refers to 1, referenced by none\n"+
		"GLOSSARY.md:33: unmapped: Stack: also in Stacks\n"+
		"internal/stacks/GLOSSARY.md:9: unmapped: Stack: also in Common\n"), text)
	assert.Equal(t, "Define Term Stack", onOrigin(t, origin, "%s", "ubiq/1"))
}

func TestADefineWithoutFindingsSaysSo(t *testing.T) {
	url, _, _ := serveOrigin(t, glossarytest.Origin(t, glossarytest.Root(t)), ada)
	// The Relationships already mention Ghost, so it is no orphan, and defining it resolves a Finding.
	text := define(t, connect(t, url), "Ghost", "A Tag that no Image holds.")
	assert.True(t, strings.HasSuffix(text, "\nThe Change introduced no Findings.\n"), text)
}

func TestEachChangeLogsOneLineNamingThePersonToolTermAndCommit(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, log := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	log.Reset()
	define(t, cs, "Digest", "A content hash.")
	assert.Eventually(t, func() bool { return strings.Count(log.String(), "\n") == 2 }, time.Second, time.Millisecond)
	assert.Equal(t, "Change define_term Digest by Ada Lovelace <ada@example.com> Writer: "+
		onOrigin(t, origin, "%H", "ubiq/1")+" on ubiq/1\n"+
		"POST /mcp 200 Ada Lovelace <ada@example.com> Writer\n", log.String())
}

func TestTheFirstStartClonesOriginAndLaterStartsReuseTheClone(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	root := filepath.Join(t.TempDir(), "clone")
	log := &logBuffer{}
	_, err := batch.Open(root, origin, bot, log)
	require.NoError(t, err)
	assert.Equal(t, "cloning "+origin+" into "+root+"\n", log.String())
	assert.FileExists(t, filepath.Join(root, "GLOSSARY.md"))
	assert.Equal(t, "ubiq/1", glossarytest.Git(t, root, "branch", "--show-current"), "reads see the open Batch")

	glossarytest.WriteTree(t, root, map[string]string{".git/kept": "the clone's own"})
	log.Reset()
	_, err = batch.Open(root, origin, bot, log)
	require.NoError(t, err)
	assert.Empty(t, log.String())
	assert.FileExists(t, filepath.Join(root, ".git/kept"))
}

func TestAStartRefusesACloneOfAnotherOrigin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clone")
	_, err := batch.Open(root, glossarytest.Origin(t, glossarytest.Root(t)), bot, &logBuffer{})
	require.NoError(t, err)
	other := glossarytest.Origin(t, glossarytest.Root(t))
	_, err = batch.Open(root, other, bot, &logBuffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not of "+other)
}
