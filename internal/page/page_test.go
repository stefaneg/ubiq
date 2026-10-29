package page

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestRenderEmbedsTheHealthFindings(t *testing.T) {
	html, err := Render(glossarytest.Load(t))
	require.NoError(t, err)
	assert.Contains(t, html, `{"path":"GLOSSARY.md","line":15,"rule":"long","name":"Tag","note":"3 sentences","term":"common/tag"}`)
}
