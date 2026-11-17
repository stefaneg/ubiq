package layout_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/layout"
)

// record runs Record and requires it to succeed.
func record(t *testing.T, names []string, title, text string) (string, string) {
	t.Helper()
	adr, err := layout.Record(names, title, text)
	require.NoError(t, err)
	return adr.Name, adr.Body
}

func TestAnADRIsNumberedAfterTheHighestExisting(t *testing.T) {
	name, _ := record(t, []string{"0001-a.md", "0007-b.md", "0003-c.md"}, "Pin tags", "We pin tags.")
	assert.Equal(t, "0008-pin-tags.md", name)
}

func TestTheFirstADRIsNumberOne(t *testing.T) {
	name, _ := record(t, nil, "Pin tags", "We pin tags.")
	assert.Equal(t, "0001-pin-tags.md", name)
}

func TestTheBodyIsTheTitleAsHeadingAndTheParagraphsBelow(t *testing.T) {
	_, body := record(t, nil, "  Pin   tags\n", "\n\nWe pin tags.  \r\nAlways.\n\n\n\nBecause digests are opaque.\n\n")
	assert.Equal(t, "# Pin tags\n\nWe pin tags.\nAlways.\n\nBecause digests are opaque.\n", body)
}

func TestAnADRIsRefusedNamingWhatIsWrong(t *testing.T) {
	names := []string{"0001-pin-tags.md", "README.md"}
	for _, c := range []struct{ title, text, refusal string }{
		{" ", "We pin tags.", "the ADR's title is empty"},
		{"Pin digests", " \n ", "the text of the ADR Pin digests is empty"},
		{"Pin Tags!", "We pin tags.", "the title Pin Tags! slugs to pin-tags, as 0001-pin-tags.md does"},
		{"!?", "We pin tags.", "the title !? has no letters or digits to name the file by"},
	} {
		_, err := layout.Record(names, c.title, c.text)
		if assert.Error(t, err, c.refusal) {
			assert.Equal(t, c.refusal, err.Error())
		}
	}
}

func TestTheSlugLowercasesHyphenatesAndStripsAsTheExistingNamesDo(t *testing.T) {
	for title, slug := range map[string]string{
		"References are bold or capitalised mentions":                    "references-are-bold-or-capitalised-mentions",
		"Follow only the latest domain-modeling format":                  "follow-only-the-latest-domain-modeling-format",
		"Read every block; only Term definitions and bold are structure": "read-every-block-only-term-definitions-and-bold-are-structure",
		"  Pin tags, not digests!  ":                                     "pin-tags-not-digests",
		"Use `git` -- not the forge's API":                               "use-git-not-the-forges-api",
		"Prefer UTF-8 über Latin-1":                                      "prefer-utf-8-über-latin-1",
	} {
		name, _ := record(t, nil, title, "Text.")
		assert.Equal(t, "0001-"+slug+".md", name, title)
	}
}
