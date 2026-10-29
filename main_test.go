package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHTMLWritesASelfContainedPage(t *testing.T) {
	root := fixtureRoot(t)
	out := filepath.Join(root, "glossary.html")
	var status bytes.Buffer
	require.NoError(t, run([]string{"html", "-root", root, "-out", out}, nil, nil, &status))
	page, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(page), `"common/tag"`)
	assert.NotContains(t, string(page), "__DATA__")
	assert.NotContains(t, string(page), "</script>\"", "closing tags inside the JSON are escaped")
	assert.Equal(t, "wrote "+out+": 10 terms, 5 notes, 3 map entries, 1 ADRs, 1 unresolved phrases\n", status.String())
}

func TestRunCheckReadsStdin(t *testing.T) {
	root := fixtureRoot(t)
	var out bytes.Buffer
	require.NoError(t, run([]string{"check", "-root", root}, strings.NewReader("fix the Image"), &out, nil))
	assert.Contains(t, out.String(), "terms recognised: Image (Workspace)\n")
}

func TestRunCheckReadsFilesAndPassesCleanText(t *testing.T) {
	root := fixtureRoot(t)
	msg := filepath.Join(root, "msg.txt")
	require.NoError(t, os.WriteFile(msg, []byte("rebuild the Image"), 0o644))
	var out bytes.Buffer
	require.NoError(t, run([]string{"check", "-root", root, msg}, nil, &out, nil))
	assert.Contains(t, out.String(), "terms recognised: Image (Workspace)\n")
}

func TestRunRejectsAnUnknownCommand(t *testing.T) {
	err := run([]string{"frobnicate"}, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}
