// Package mcpserver serves a repository's Domain Model to agents over the Model Context Protocol, with the read tools
// lookup, check and health, which answer as ubiq lookup, ubiq check and ubiq health do, and whoami, which tells the
// calling Person. Every transport builds its tools here, so stdio and the Server cannot drift apart. Each call reads
// the model afresh, so an answer is never older than the files; see docs/adr/0003-read-the-domain-model-on-every-call.md.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stefaneg/ubiq/internal/check"
	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
	"github.com/stefaneg/ubiq/internal/lookup"
	"github.com/stefaneg/ubiq/internal/person"
)

const instructions = `This repository keeps its ubiquitous language, the names its code and docs use for domain ` +
	`concepts, in GLOSSARY.md files. Use lookup before you name a new type, function, file, or concept, and when you ` +
	`meet a domain word you do not know. Use check on text you write before you finish it. Use health to see what in ` +
	`the glossary needs fixing, and whoami to learn who you act for and their role. Prefer the Terms these tools give ` +
	`over synonyms, and never use a word a Term avoids.`

const lookupDescription = `Look up words in this repository's glossary. For each word it gives the Term the word ` +
	`names: its Context and Cluster, file and line, its definition, the words the Term avoids, the Terms it refers to and that refer to ` +
	`it, the Terms the glossary's statements mention alongside it, up to five of those statements, and the ADRs that ` +
	`mention it. When a Term lists the word under Avoid, it says so ` +
	`and gives the Term to use instead. When neither holds for the whole word, it splits a compound such as ` +
	`StackImageResolver into the Terms and avoided words inside it, one line each; when no part matches either, it ` +
	`lists the Terms that mention the word. Words may be prose ("stack images") or identifiers (StackImage, ` +
	`stack_image). Call it before naming a new type, function, ` +
	`file, or concept, to reuse the glossary's name rather than invent a synonym.`

const checkDescription = `Check text against this repository's glossary: a doc, comment, commit message, PR ` +
	`description, or list of identifiers. It lists the Terms the text uses, the words it uses that a Term avoids with ` +
	`the Term to use instead, and its Capitalised Phrases that match no Term, which are either missing Terms or words ` +
	`to replace. Call it on text you wrote before you finish.`

const healthDescription = `Run the Health Check on this repository's glossary: one "path:line: rule: name: note" line ` +
	`per Finding, then a summary line counting them by rule. The rules are long (a definition over two sentences), ` +
	`orphan (a Term nothing mentions), unmapped (a name two Contexts define with no statement relating them) and ` +
	`unresolved (a bold phrase that names no Term). Call it after changing the glossary to find what is left to fix.`

const whoamiDescription = `Tell who this server acts for: the Person's name, email and role. Over stdio that is the ` +
	`git identity of the working tree with role Maintainer; on the Server it is the logged-in Person, a Reader or a ` +
	`Writer. Call it to learn what you may do here.`

type lookupInput struct {
	Words []string `json:"words" jsonschema:"the words to look up, each a word, a phrase or an identifier"`
}

type checkInput struct {
	Text string `json:"text" jsonschema:"the text to check"`
}

// Option changes the instructions a server gives its clients.
type Option func(instructions string) string

// Reads tells clients that the server reads branch of its own copy of the repository, so they do not compare its
// answers with their own checkout.
func Reads(branch string) Option {
	return func(instructions string) string {
		return instructions + ` This server reads branch ` + branch + ` of its own copy of the repository, not your ` +
			`checkout, so its answers can differ from the files on your machine.`
	}
}

// New returns a server for the Domain Model under root that learns the calling Person from people.
func New(root, version string, people person.Resolver, opts ...Option) *mcp.Server {
	instructions := instructions
	for _, o := range opts {
		instructions = o(instructions)
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "ubiq", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)}
	mcp.AddTool(s, &mcp.Tool{Name: "lookup", Description: lookupDescription, Annotations: readOnly},
		func(_ context.Context, _ *mcp.CallToolRequest, in lookupInput) (*mcp.CallToolResult, any, error) {
			if len(in.Words) == 0 {
				return nil, nil, errors.New("name at least one word to look up")
			}
			model, err := load(root)
			if err != nil {
				return nil, nil, err
			}
			l := lookup.New(model)
			var out strings.Builder
			for i, w := range in.Words {
				if i > 0 {
					out.WriteString("\n")
				}
				l.Find(w).Write(&out)
			}
			return text(out.String()), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "check", Description: checkDescription, Annotations: readOnly},
		func(_ context.Context, _ *mcp.CallToolRequest, in checkInput) (*mcp.CallToolResult, any, error) {
			model, err := load(root)
			if err != nil {
				return nil, nil, err
			}
			var out strings.Builder
			check.New(model).Check(in.Text).Write(&out)
			return text(out.String()), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "health", Description: healthDescription, Annotations: readOnly},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			model, err := load(root)
			if err != nil {
				return nil, nil, err
			}
			findings := health.Check(model)
			var out strings.Builder
			health.Write(&out, findings)
			fmt.Fprintln(&out, health.Summary(findings, len(model.Terms)))
			return text(out.String()), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "whoami", Description: whoamiDescription, Annotations: readOnly},
		func(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			var header http.Header
			if req.Extra != nil {
				header = req.Extra.Header
			}
			p, err := people.Resolve(header)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("name: %s\nemail: %s\nrole: %s\n", orUnset(p.Name), orUnset(p.Email), p.Role)), nil, nil
		})
	return s
}

func load(root string) (*glossary.Model, error) {
	model, err := glossary.Load(root)
	if err != nil {
		return nil, fmt.Errorf("read the glossary under %s: %w", root, err)
	}
	return model, nil
}

func orUnset(s string) string {
	if s == "" {
		return "unset"
	}
	return s
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
