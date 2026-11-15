// PROTOTYPE, throwaway: `ubiq serve` for the map "Served Ubiq". Answers "how does logging in feel and behave, end
// to end": one process that serves the glossary page and the MCP tools over HTTP behind Pomerium, reading the
// Person from the proxy's assertion JWT. Read-only. No tests, no polish, not for main.
//
//	ubiq serve [-root dir] [-listen 127.0.0.1:8766] [-jwks url] [-ca dev/certs/ca.pem]
//	           [-origin url] [-default main] [-committer "ubiq serve <ubiq@localhost>"]
//
// With -origin the Server owns a clone at -root, reads from the open Batch and offers define_term (proto_write.go).
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdk "github.com/pomerium/sdk-go"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/mcpserver"
	"github.com/stefaneg/ubiq/internal/page"
)

const assertionHeader = "X-Pomerium-Jwt-Assertion"

// writerGroup is the Writer Rule's default: claim groups, value ubiq-writers.
const writerGroup = "ubiq-writers"

func runServe(args []string, status io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(status)
	root := fs.String("root", ".", "repository root holding GLOSSARY.md, GLOSSARY-MAP.md and docs/adr")
	listen := fs.String("listen", "127.0.0.1:8766", "address to listen on; loopback, Pomerium sits in front")
	jwks := fs.String("jwks", "https://authenticate.localhost.pomerium.io/.well-known/pomerium/jwks.json",
		"Pomerium's assertion signing keys")
	ca := fs.String("ca", "dev/certs/ca.pem", "extra CA to trust when fetching the keys; empty for system roots only")
	origin := fs.String("origin", "", "git remote the Server clones into -root and pushes Batches to; empty for read-only")
	defBranch := fs.String("default", "main", "the default branch Batches start from and merge")
	committer := fs.String("committer", "ubiq serve <ubiq@localhost>", "the Server's own git identity, the committer of every Change")
	if err := fs.Parse(args); err != nil {
		return err
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	verifier, err := newVerifier(*jwks, *ca)
	if err != nil {
		return err
	}
	who := func(ctx context.Context, h http.Header) (*person, error) { return personFrom(ctx, verifier, h) }

	var repo *batchRepo
	if *origin != "" {
		repo = &batchRepo{dir: abs, origin: *origin, defaultBranch: *defBranch, prefix: "ubiq/", committer: *committer, status: status}
		if err := repo.ensureClone(); err != nil {
			return err
		}
		// A conflict at startup must not stop reads: the clone still holds a Batch to serve. Writes refuse until a
		// Maintainer resolves it.
		if batch, err := repo.prepare(); err != nil {
			fmt.Fprintf(status, "startup: %v\n", err)
		} else {
			fmt.Fprintf(status, "reading from Batch %s of %s\n", batch, *origin)
		}
	}

	srv := mcpserver.New(abs, version())
	srv.AddReceivingMiddleware(logMCP(status, who))
	addDefineTerm(srv, repo, who)
	mcp.AddTool(srv, &mcp.Tool{Name: "whoami", Description: "Who the server believes is calling, from the proxy's assertion."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			p, err := who(ctx, req.Extra.Header)
			if err != nil {
				return nil, nil, err
			}
			b, _ := json.MarshalIndent(p, "", "  ")
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
		})

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		// Pomerium reaches the loopback listener with the upstream's Host (host.docker.internal:8766); the SDK's
		// DNS-rebinding guard would answer 403 "invalid Host header" to that.
		&mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true}))
	mux.HandleFunc("/whoami", func(w http.ResponseWriter, r *http.Request) {
		p, err := who(r.Context(), r.Header)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		p, err := who(r.Context(), r.Header)
		if err != nil {
			http.Error(w, "no Person: "+err.Error(), http.StatusUnauthorized)
			return
		}
		model, err := glossary.Load(abs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out, err := page.Render(model)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, withBanner(out, p))
	})
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := who(r.Context(), r.Header)
		fmt.Fprintf(status, "%s %s %s as %s\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, p.label())
		mux.ServeHTTP(w, r)
	})
	fmt.Fprintf(status, "ubiq serve (PROTOTYPE) on http://%s, root %s\n", *listen, abs)
	return http.ListenAndServe(*listen, logged)
}

type person struct {
	Subject string   `json:"sub"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Groups  []string `json:"groups"`
	Role    string   `json:"role"`
}

func (p *person) label() string {
	if p == nil {
		return "nobody"
	}
	return p.Email + " (" + p.Role + ")"
}

func personFrom(ctx context.Context, v *sdk.Verifier, h http.Header) (*person, error) {
	raw := h.Get(assertionHeader)
	if raw == "" {
		return nil, errors.New("no " + assertionHeader + " header: not behind Pomerium, or pass_identity_headers is off")
	}
	id, err := v.GetIdentity(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("verify assertion: %w", err)
	}
	p := &person{Subject: id.Subject, Email: id.Email, Name: id.Name, Groups: id.Groups, Role: "Reader"}
	for _, g := range id.Groups {
		if g == writerGroup {
			p.Role = "Writer"
		}
	}
	return p, nil
}

func newVerifier(jwks, ca string) (*sdk.Verifier, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, err
		}
		pool.AppendCertsFromPEM(pem)
	}
	client := &http.Client{Timeout: 10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	return sdk.New(&sdk.Options{JWKSEndpoint: jwks, HTTPClient: client})
}

// logMCP prints every MCP method with the Person the transport's headers name.
func logMCP(status io.Writer, who func(context.Context, http.Header) (*person, error)) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			var p *person
			if extra := req.GetExtra(); extra != nil {
				p, _ = who(ctx, extra.Header)
			}
			detail := ""
			if c, ok := req.(*mcp.CallToolRequest); ok {
				detail = " " + c.Params.Name
			}
			fmt.Fprintf(status, "%s mcp %s%s as %s\n", time.Now().Format("15:04:05"), method, detail, p.label())
			return next(ctx, method, req)
		}
	}
}

func withBanner(doc string, p *person) string {
	banner := fmt.Sprintf(`<div style="background:#ffe8a3;padding:.5em 1em;font-family:sans-serif">`+
		`PROTOTYPE &middot; signed in as <b>%s</b> &lt;%s&gt; &middot; %s &middot; groups: %s</div>`,
		html.EscapeString(p.Name), html.EscapeString(p.Email), p.Role, html.EscapeString(strings.Join(p.Groups, ", ")))
	i := strings.Index(doc, "<body")
	if i < 0 {
		return banner + doc
	}
	j := strings.Index(doc[i:], ">")
	if j < 0 {
		return banner + doc
	}
	at := i + j + 1
	return doc[:at] + banner + doc[at:]
}
