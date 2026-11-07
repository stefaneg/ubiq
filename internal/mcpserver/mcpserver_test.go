package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
)

// connect starts the server for root and returns a client session to it.
func connect(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := New(root, "test").Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call calls a tool and returns its text and whether it is an error.
func call(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestLookupAnswersEachWord(t *testing.T) {
	cs := connect(t, glossarytest.Root(t))
	text, isErr := call(t, cs, "lookup", map[string]any{"words": []string{"Workspace", "repo"}})
	assert.False(t, isErr)
	assert.True(t, strings.HasPrefix(text, "Workspace (Workspace), internal/workspace/GLOSSARY.md:13\n"), text)
	assert.Contains(t, text, "\n\n\"repo\" is an Avoided Word. Use instead:\n")
}

func TestCheckReportsTheTermsAndAvoidedWordsOfAText(t *testing.T) {
	cs := connect(t, glossarytest.Root(t))
	text, isErr := call(t, cs, "check", map[string]any{"text": "push the Stack Image to the repo"})
	assert.False(t, isErr)
	assert.Equal(t, `terms recognised: Stack Image (Stacks)
capitalised phrases without an entry: none
avoided words: repo → Repository (Common), Workspace (Workspace)
`, text)
}

func TestEveryCallReadsTheGlossaryAsItIsNow(t *testing.T) {
	root := glossarytest.Root(t)
	cs := connect(t, root)
	text, _ := call(t, cs, "lookup", map[string]any{"words": []string{"Upload Queue"}})
	require.Contains(t, text, "names no Term")
	glossarytest.WriteTree(t, root, map[string]string{"internal/stacks/GLOSSARY.md": `# Stacks

## Language

**Upload Queue**:
The images waiting to be pushed.
`})
	text, _ = call(t, cs, "lookup", map[string]any{"words": []string{"Upload Queue"}})
	assert.True(t, strings.HasPrefix(text, "Upload Queue (Stacks), internal/stacks/GLOSSARY.md:5\n"), text)
}

func TestAGlossaryThatCannotBeReadIsAToolError(t *testing.T) {
	cs := connect(t, t.TempDir())
	text, isErr := call(t, cs, "check", map[string]any{"text": "anything"})
	assert.True(t, isErr)
	assert.Contains(t, text, "read the glossary under ")
}

func TestToolsAreReadOnly(t *testing.T) {
	cs := connect(t, glossarytest.Root(t))
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
	}
	assert.ElementsMatch(t, []string{"lookup", "check"}, names)
}
