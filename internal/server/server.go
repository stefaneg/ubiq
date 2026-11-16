// Package server is the Server as an HTTP handler: the glossary page at the root path and the MCP tools at /mcp, for
// one repository, every request on behalf of a Person. It knows nothing of its public URL, TLS or the proxy in front;
// see docs/server.md.
package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/mcpserver"
	"github.com/stefaneg/ubiq/internal/page"
	"github.com/stefaneg/ubiq/internal/person"
)

// Options build the Server.
type Options struct {
	Root    string          // the repository the Server reads
	Version string          // ubiq's version, told to MCP clients
	People  person.Resolver // who each request is on behalf of
	Log     io.Writer       // one line per request
}

// New returns the Server's handler. A request whose Person cannot be resolved is answered 401, never as somebody
// else.
func New(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", o.page)
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server {
			var opts []mcpserver.Option
			if b := branch(o.Root); b != "" {
				opts = append(opts, mcpserver.Reads(b))
			}
			return mcpserver.New(o.Root, o.Version, o.People, opts...)
		},
		// The proxy reaches the loopback listener with the upstream's Host, which the SDK's DNS-rebinding guard
		// would refuse.
		&mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		p, err := o.People.Resolve(r.Header)
		if err != nil {
			http.Error(rec, "unauthorised", http.StatusUnauthorized)
			fmt.Fprintf(o.Log, "%s %s %d unauthorised: %s\n", r.Method, r.URL.Path, rec.status, err)
			return
		}
		mux.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), personKey{}, p)))
		fmt.Fprintf(o.Log, "%s %s %d %s\n", r.Method, r.URL.Path, rec.status, label(p))
	})
}

type personKey struct{}

// page serves the glossary page, as ubiq html renders it, with the Person's name and role.
func (o Options) page(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(personKey{}).(person.Person)
	model, err := glossary.Load(o.Root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := p.Name
	if name == "" {
		name = p.Email
	}
	html, err := page.RenderFor(model, page.Viewer{Name: name, Role: string(p.Role)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, html)
}

// branch is the branch checked out at root, read afresh on each call, or empty when root has none.
func branch(root string) string {
	out, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// label names a Person in the log.
func label(p person.Person) string {
	return fmt.Sprintf("%s <%s> %s", p.Name, p.Email, p.Role)
}

// recorder remembers the status a handler answered with.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Flush lets the MCP transport stream through the recorder.
func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
