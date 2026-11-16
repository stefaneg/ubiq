package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestARenameByAWriterLandsOneCommitRewritingEveryGlossaryThatRefersToTheTerm(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)

	text, isErr := call(t, cs, "revise_term", map[string]any{
		"context": "workspace", "name": "image", "new_name": "Picture", "cluster": "Pictures",
		"definition": "An OCI image built from a Workspace.", "avoid": []string{"container image"},
	})
	require.False(t, isErr, text)

	commit := onOrigin(t, origin, "%H", "ubiq/1")
	assert.Equal(t, "Ada Lovelace|ada@example.com|ubiq-bot|bot@example.com|Revise Term Image|",
		onOrigin(t, origin, "%an|%ae|%cn|%ce|%s|%b", "ubiq/1"))
	assert.Equal(t, "GLOSSARY-MAP.md\nGLOSSARY.md\ninternal/stacks/GLOSSARY.md\ninternal/workspace/GLOSSARY.md",
		glossarytest.Git(t, "", "--git-dir", origin, "show", "--name-only", "--format=", "ubiq/1"))
	assert.True(t, strings.HasPrefix(text, "Revised Image, now named Picture, in commit "+commit+" on the Batch ubiq/1.\n\n"+
		"Picture (Workspace › Pictures), internal/workspace/GLOSSARY.md:19\n"+
		"  An OCI image built from a Workspace.\n"+
		"  avoid: container image\n"+
		"  refers to: Workspace\n"+
		"  referred to by: Image Ref (Common), Repository (Common), Stack Image (Stacks)\n"), text)
	assert.True(t, strings.HasSuffix(text, "\nThe Change introduced no Findings.\n"), text)

	text, isErr = call(t, cs, "lookup", map[string]any{"words": []string{"Stack Image"}})
	assert.False(t, isErr)
	assert.Contains(t, text, "An image a Stack runs, identified by a workspace Picture.")
}

func TestARemoveByAWriterLandsAndReturnsTheUnresolvedPhrasesItLeft(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)

	text, isErr := call(t, connect(t, url), "remove_term", map[string]any{"context": "Common", "name": "unit of work"})
	require.False(t, isErr, text)

	commit := onOrigin(t, origin, "%H", "ubiq/1")
	assert.Equal(t, "Ada Lovelace|ada@example.com|ubiq-bot|bot@example.com|Remove Term Unit of Work|",
		onOrigin(t, origin, "%an|%ae|%cn|%ce|%s|%b", "ubiq/1"))
	assert.Equal(t, "Removed Unit of Work in commit "+commit+" on the Batch ubiq/1.\n"+
		"\nFindings the Change introduced:\n"+
		"GLOSSARY.md: unresolved: Units of Work: Common: Run\n"+
		"GLOSSARY.md:25: orphan: Run: no references in or out\n", text)
}

func TestAReviseOrRemoveIsRefusedNamingTheTermOrField(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, ada)
	cs := connect(t, url)
	cases := []struct {
		tool    string
		args    map[string]any
		refusal string
	}{
		{"revise_term", map[string]any{"context": "Common", "name": "Digest", "definition": "A hash."}, "There is no Term Digest in Common."},
		{"revise_term", map[string]any{"context": "Common", "name": " ", "definition": "A hash."}, "The Term's name is empty."},
		{"revise_term", map[string]any{"context": "Common", "name": "tag", "definition": " "}, "The definition of tag is empty."},
		{"revise_term", map[string]any{"context": "Common", "name": "Tag", "new_name": "run", "definition": "A label."}, "The Term Run already exists in Common."},
		{"revise_term", map[string]any{"context": "Billing", "name": "Tag", "definition": "A label."}, "No Context Billing; this repository has Workspace, Stacks and Common."},
		{"revise_term", map[string]any{"name": "Tag", "definition": "A label."}, "Name the Context, since this repository has Workspace, Stacks and Common."},
		{"remove_term", map[string]any{"context": "Common", "name": "Digest"}, "There is no Term Digest in Common."},
		{"remove_term", map[string]any{"context": "Common", "name": ""}, "The Term's name is empty."},
		{"remove_term", map[string]any{"context": "Billing", "name": "Tag"}, "No Context Billing; this repository has Workspace, Stacks and Common."},
	}
	for _, c := range cases {
		text, isErr := call(t, cs, c.tool, c.args)
		assert.True(t, isErr, c.refusal)
		assert.Equal(t, c.refusal, text)
	}
	assert.NotContains(t, glossarytest.Git(t, "", "--git-dir", origin, "log", "--all", "--format=%s"), " Term ")
}

func TestAReadersReviseAndRemoveAreRefusedNamingTheWriterRule(t *testing.T) {
	origin := glossarytest.Origin(t, glossarytest.Root(t))
	url, _, _ := serveOrigin(t, origin, bob)
	cs := connect(t, url)
	for tool, args := range map[string]map[string]any{
		"revise_term": {"context": "Common", "name": "Tag", "definition": "A label."},
		"remove_term": {"context": "Common", "name": "Tag"},
	} {
		text, isErr := call(t, cs, tool, args)
		assert.True(t, isErr)
		assert.Equal(t, "Bob Reader is a Reader here, and only a Person whose groups claim holds glossary-writers may make Changes.", text)
	}
	assert.False(t, strings.Contains(glossarytest.Git(t, "", "--git-dir", origin, "branch"), "ubiq/"))
}
