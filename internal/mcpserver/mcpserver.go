// Package mcpserver serves a repository's Domain Model to agents over the Model Context Protocol, with a lookup tool
// and a check tool that answer as ubiq lookup and ubiq check do. Each call reads the model afresh, so an answer is
// never older than the files; see docs/adr/0003-read-the-domain-model-on-every-call.md.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stefaneg/ubiq/internal/check"
	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/lookup"
)

const instructions = `This repository keeps its ubiquitous language, the names its code and docs use for domain ` +
	`concepts, in GLOSSARY.md files. Use lookup before you name a new type, function, file, or concept, and when you ` +
	`meet a domain word you do not know. Use check on text you write before you finish it. Prefer the Terms these ` +
	`tools give over synonyms, and never use a word a Term avoids.`

const lookupDescription = `Look up words in this repository's glossary. For each word it gives the Term the word ` +
	`names: its definition, Context, file and line, the words the Term avoids, the Terms it refers to and that refer to ` +
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

type lookupInput struct {
	Words []string `json:"words" jsonschema:"the words to look up, each a word, a phrase or an identifier"`
}

type checkInput struct {
	Text string `json:"text" jsonschema:"the text to check"`
}

// New returns a server for the Domain Model under root.
func New(root, version string) *mcp.Server {
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
	return s
}

func load(root string) (*glossary.Model, error) {
	model, err := glossary.Load(root)
	if err != nil {
		return nil, fmt.Errorf("read the glossary under %s: %w", root, err)
	}
	return model, nil
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
