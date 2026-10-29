package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestRunHTMLWritesASelfContainedPage(t *testing.T) {
	root := glossarytest.Root(t)
	out := filepath.Join(root, "glossary.html")
	var status bytes.Buffer
	require.NoError(t, run([]string{"html", "-root", root, "-out", out}, nil, nil, &status))
	page, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(page), `"common/tag"`)
	assert.NotContains(t, string(page), "__DATA__")
	assert.NotContains(t, string(page), "</script>\"", "closing tags inside the JSON are escaped")
	assert.Equal(t, "wrote "+out+": 10 terms, 1 ADRs, 1 unresolved phrases\n", status.String())
}

func TestRunCheckReadsStdin(t *testing.T) {
	root := glossarytest.Root(t)
	var out bytes.Buffer
	require.NoError(t, run([]string{"check", "-root", root}, strings.NewReader("fix the Image"), &out, nil))
	assert.Contains(t, out.String(), "terms recognised: Image (Workspace)\n")
}

func TestRunCheckReadsFilesAndPassesCleanText(t *testing.T) {
	root := glossarytest.Root(t)
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

func TestRunHealthPrintsFindingsAndExitsOne(t *testing.T) {
	root := glossarytest.Root(t)
	var out, status bytes.Buffer
	err := run([]string{"health", "-root", root}, nil, &out, &status)
	require.True(t, errors.Is(err, errHealthFindings))
	assert.Equal(t, "GLOSSARY.md: unresolved: Label: Common: Repository\n"+
		"GLOSSARY.md:11: orphan: Image Ref: refers to 2, referenced by none\n"+
		"GLOSSARY.md:15: long: Tag: 3 sentences\n"+
		"GLOSSARY.md:25: unmapped: Run: also in Workspace\n"+
		"internal/workspace/GLOSSARY.md:17: unmapped: Run: also in Common\n", out.String())
	assert.Equal(t, "5 findings over 10 terms: 1 long, 1 orphan, 2 unmapped, 1 unresolved\n", status.String())
}
