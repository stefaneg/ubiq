package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestARecordedADRLandsNumberedAndSluggedAndTheNextLookupMentionsIt(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, root, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)

	text, isErr := call(t, cs, "record_adr", map[string]any{
		"title": "Pin Images by digest", "text": "Every Tag moves.\n\nA digest does not.",
	})
	require.False(t, isErr, text)

	path := "docs/adr/0002-pin-images-by-digest.md"
	adr := "# Pin Images by digest\n\nEvery Tag moves.\n\nA digest does not.\n"
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	require.NoError(t, err)
	assert.Equal(t, adr, string(body))
	assert.Equal(t, "Ada Lovelace|ada@example.com|ubiq-bot|bot@example.com|Record ADR Pin Images by digest|",
		onOrigin(t, origin, "%an|%ae|%cn|%ce|%s|%b", "ubiq/1"))
	assert.Equal(t, "Recorded "+path+" in commit "+onOrigin(t, origin, "%H", "ubiq/1")+" on the Batch ubiq/1.\n\n"+
		adr+"\nThe Change introduced no Findings.\n", text)

	text, isErr = call(t, cs, "lookup", map[string]any{"words": []string{"Tag"}})
	require.False(t, isErr)
	assert.Contains(t, text, "\n  ADR: Pin Images by digest ("+path+")\n")
}

func TestARecordIsRefusedNamingWhatIsWrong(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	for _, c := range []struct {
		args    map[string]any
		refusal string
	}{
		{map[string]any{"title": " ", "text": "We pin."}, "The ADR's title is empty."},
		{map[string]any{"title": "Pin digests", "text": " "}, "The text of the ADR Pin digests is empty."},
		{map[string]any{"title": "Pin tags", "text": "We pin."}, "The title Pin tags slugs to pin-tags, as 0001-pin-tags.md does."},
	} {
		text, isErr := call(t, cs, "record_adr", c.args)
		assert.True(t, isErr, c.refusal)
		assert.Equal(t, c.refusal, text)
	}
	assert.NotContains(t, glossarytest.Git(t, "", "--git-dir", origin, "log", "--all", "--format=%s"), "Record ADR")
}

func TestAReadersRecordIsRefusedNamingTheWriterRule(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, bob)
	text, isErr := call(t, connect(t, url), "record_adr", map[string]any{"title": "Pin digests", "text": "We pin."})
	assert.True(t, isErr)
	assert.Equal(t, "Bob Reader is a Reader here, and only a Person whose groups claim holds glossary-writers may make Changes.", text)
	assert.NotContains(t, glossarytest.Git(t, "", "--git-dir", origin, "branch"), "ubiq/")
}
