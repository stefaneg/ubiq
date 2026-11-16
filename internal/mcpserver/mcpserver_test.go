package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
	"github.com/stefaneg/ubiq/internal/person"
)

// resolver answers every request with one Person, or with an error when err is set.
type resolver struct {
	p   person.Person
	err error
}

func (r resolver) Resolve(http.Header) (person.Person, error) { return r.p, r.err }

var ada = resolver{p: person.Person{Name: "Ada Lovelace", Email: "ada@example.com", Role: person.Writer}}

// connect starts the server for root, calling every Person Ada, and returns a client session to it.
func connect(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	return connectAs(t, root, ada)
}

// connectAs starts the server for root with the Person resolver r and returns a client session to it.
func connectAs(t *testing.T, root string, r person.Resolver) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := New(root, "test", r, "").Connect(ctx, serverT, nil)
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
	assert.True(t, strings.HasPrefix(text, "Workspace (Workspace › Planning), internal/workspace/GLOSSARY.md:13\n"), text)
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
	assert.ElementsMatch(t, []string{"lookup", "check", "health", "whoami"}, names)
}

func TestHealthReturnsTheHealthCheckAsUbiqHealthPrintsIt(t *testing.T) {
	cs := connect(t, glossarytest.Root(t))
	text, isErr := call(t, cs, "health", nil)
	assert.False(t, isErr)
	assert.Equal(t, "GLOSSARY-MAP.md: unresolved: Spectre: Workspace: Cross-context vocabulary › Deliberate mirrors › Run\n"+
		"GLOSSARY.md: unresolved: Ghost: Common: Relationships\n"+
		"GLOSSARY.md: unresolved: Label: Common: Repository\n"+
		"GLOSSARY.md: unresolved: Phantom: Common: Flagged ambiguities\n"+
		"GLOSSARY.md:15: long: Tag: 3 sentences\n"+
		"GLOSSARY.md:25: unmapped: Run: also in Workspace\n"+
		"internal/workspace/GLOSSARY.md:17: unmapped: Run: also in Common\n"+
		"7 findings over 10 terms: 1 long, 0 orphan, 2 unmapped, 4 unresolved\n", text)
}

func TestHealthSaysSoWhenThereAreNoFindings(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY.md": "# Shop\n\n## Language\n\n**Order**:\nA request for **Goods**.\n\n**Goods**:\nWhat an **Order** asks for.\n",
	})
	cs := connect(t, root)
	text, isErr := call(t, cs, "health", nil)
	assert.False(t, isErr)
	assert.Equal(t, "0 findings over 2 terms: 0 long, 0 orphan, 0 unmapped, 0 unresolved\n", text)
}

func TestWhoamiTellsThePersonsNameEmailAndRole(t *testing.T) {
	cs := connect(t, glossarytest.Root(t))
	text, isErr := call(t, cs, "whoami", nil)
	assert.False(t, isErr)
	assert.Equal(t, "name: Ada Lovelace\nemail: ada@example.com\nrole: Writer\n", text)
}

func TestWhoamiSaysUnsetForWhatIsNotKnown(t *testing.T) {
	cs := connectAs(t, glossarytest.Root(t), resolver{p: person.Person{Name: "Ada Lovelace", Role: person.Maintainer}})
	text, _ := call(t, cs, "whoami", nil)
	assert.Equal(t, "name: Ada Lovelace\nemail: unset\nrole: Maintainer\n", text)
}

func TestWhoamiRelaysWhyThePersonIsUnknown(t *testing.T) {
	cs := connectAs(t, glossarytest.Root(t), resolver{err: errors.New("nobody is logged in.")})
	text, isErr := call(t, cs, "whoami", nil)
	assert.True(t, isErr)
	assert.Equal(t, "nobody is logged in.", text)
}

func TestInstructionsAndDescriptionsMentionHealthAndWhoami(t *testing.T) {
	cs := connect(t, glossarytest.Root(t))
	instructions := cs.InitializeResult().Instructions
	assert.Contains(t, instructions, "health")
	assert.Contains(t, instructions, "whoami")
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	for _, tool := range res.Tools {
		assert.NotEmpty(t, tool.Description, tool.Name)
	}
}
