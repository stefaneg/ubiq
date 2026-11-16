package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

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

// logBuffer holds what the Server logs, safe to write from its handlers while a test reads it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *logBuffer) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Reset()
}

// serve stands the Server up for root under a test server, learning every Person from r, and returns its URL and
// what it logs.
func serve(t *testing.T, root string, r person.Resolver) (string, *logBuffer) {
	t.Helper()
	log := &logBuffer{}
	ts := httptest.NewServer(New(Options{Root: root, Version: "test", People: r, Log: log}))
	t.Cleanup(ts.Close)
	return ts.URL, log
}

// get fetches url and returns the status and body.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	res, err := http.Get(url)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

func TestThePageIsTheHTMLPageWithThePersonsNameAndRole(t *testing.T) {
	url, _ := serve(t, glossarytest.Root(t), ada)
	status, body := get(t, url+"/")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"common/tag"`, "the Domain Model")
	assert.Contains(t, body, `"rule":"long","name":"Tag"`, "the Health Check")
	assert.Contains(t, body, `"person":{"name":"Ada Lovelace","role":"Writer"}`)
}

func TestARequestWithNoPersonIsUnauthorised(t *testing.T) {
	nobody := resolver{err: errors.New("no assertion on the request")}
	url, _ := serve(t, glossarytest.Root(t), nobody)
	status, body := get(t, url+"/")
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.False(t, strings.Contains(body, "common/tag"), "the page is not served")
	assert.NotContains(t, body, "no assertion", "the reason is logged, not returned")

	res, err := http.Post(url+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	_ = res.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

// hostRewriter sends every request with the Host a proxy in front would forward, as Pomerium does.
type hostRewriter struct{ host string }

func (h hostRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = h.host
	return http.DefaultTransport.RoundTrip(r)
}

// connect opens an MCP session to the Server at url over the streamable transport, with a proxy's Host.
func connect(t *testing.T, url string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             url + "/mcp",
		HTTPClient:           &http.Client{Transport: hostRewriter{"mcp.glossary.example.com"}},
		DisableStandaloneSSE: true,
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), transport, nil)
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

func TestMCPServesTheFourReadToolsBehindAProxysHost(t *testing.T) {
	url, _ := serve(t, glossarytest.Root(t), ada)
	cs := connect(t, url)
	tools, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"lookup", "check", "health", "whoami"}, names)

	text, isErr := call(t, cs, "lookup", map[string]any{"words": []string{"Tag"}})
	assert.False(t, isErr)
	assert.Contains(t, text, "Tag (Common › Image References), GLOSSARY.md:15\n")
	text, isErr = call(t, cs, "check", map[string]any{"text": "fix the Image"})
	assert.False(t, isErr)
	assert.Contains(t, text, "terms recognised: Image (Workspace)\n")
	text, isErr = call(t, cs, "health", nil)
	assert.False(t, isErr)
	assert.Contains(t, text, "GLOSSARY.md:15: long: Tag: 3 sentences\n")
}

func TestWhoamiAnswersTheFixedPersonAsWriter(t *testing.T) {
	url, _ := serve(t, glossarytest.Root(t), person.Fixed("Ada Lovelace", "ada@example.com"))
	text, isErr := call(t, connect(t, url), "whoami", nil)
	assert.False(t, isErr)
	assert.Equal(t, "name: Ada Lovelace\nemail: ada@example.com\nrole: Writer\n", text)
}

func TestTheInstructionsNameTheBranchBeingRead(t *testing.T) {
	root := glossarytest.Root(t)
	out, err := exec.Command("git", "-C", root, "init", "-q", "-b", "ubiq/3").CombinedOutput()
	require.NoError(t, err, string(out))
	url, _ := serve(t, root, ada)
	assert.Contains(t, connect(t, url).InitializeResult().Instructions, "branch ubiq/3")
}

func TestEachRequestLogsOneLine(t *testing.T) {
	url, log := serve(t, glossarytest.Root(t), ada)
	get(t, url+"/")
	assert.Equal(t, "GET / 200 Ada Lovelace <ada@example.com> Writer\n", log.String())
}

func TestAnUnauthorisedRequestLogsItsReason(t *testing.T) {
	url, log := serve(t, glossarytest.Root(t), resolver{err: errors.New("no assertion on the request")})
	get(t, url+"/")
	assert.Equal(t, "GET / 401 unauthorised: no assertion on the request\n", log.String())
}

func TestAToolCallLogsOneLineAndNothingElse(t *testing.T) {
	url, log := serve(t, glossarytest.Root(t), ada)
	cs := connect(t, url)
	log.Reset()
	call(t, cs, "whoami", nil)
	// the answer can reach the client before the handler returns and logs
	const line = "POST /mcp 200 Ada Lovelace <ada@example.com> Writer\n"
	assert.Eventually(t, func() bool { return log.String() != "" }, time.Second, time.Millisecond)
	assert.Equal(t, line, log.String())
}
