package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/edit"
	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
	"github.com/stefaneg/ubiq/internal/lookup"
	"github.com/stefaneg/ubiq/internal/person"
)

const defineDescription = `Define a new Term in this repository's glossary. Write the definition as plain text: every ` +
	`mention of an existing Term, or its plural, becomes a Reference, and everything else stays as typed. The Term goes ` +
	`at the end of the Cluster you name, a Cluster that does not exist yet is created, and with no Cluster it goes ` +
	`after the last Term. Name the Context when the repository has more than one. The Term lands at once as a commit ` +
	`authored by you in the open Batch, a ubiq/<n> branch a Maintainer merges, and the next lookup sees it. The answer ` +
	`is the Term as it now reads, the commit, the Batch, and the Findings the Change introduced, which warn and never ` +
	`refuse. Refused when you are a Reader, when the Term exists in that Context, when the Context does not exist, or ` +
	`when the name or definition is empty. Look the words up first so the definition reuses existing Terms.`

const reviseDescription = `Revise a Term in this repository's glossary: replace its definition and Avoided Words in ` +
	`full, move it to the Cluster you name, or rename it with new_name, rewriting every Reference to it. Not yet ` +
	`available on this Server.`

const removeDescription = `Remove a Term from this repository's glossary. Not yet available on this Server.`

const recordDescription = `Record an ADR, a decision that is hard to reverse, in this repository's docs/adr. Give a ` +
	`short title and the text as plain paragraphs: what the context was, what was decided, and why. The Server numbers ` +
	`the file after the highest existing ADR, slugs the title, and puts the title above the text as its heading. The ` +
	`ADR lands at once as a commit authored by you in the open Batch, a ubiq/<n> branch a Maintainer merges, and the ` +
	`next lookup names it beside the Terms it mentions. The answer is the ADR's path and text, the commit, the Batch, ` +
	`and the Findings the Change introduced. Refused when you are a Reader, when the title or text is empty, or when ` +
	`an ADR with the same slug exists.`

type defineInput struct {
	Name       string   `json:"name" jsonschema:"the Term's name, as it should be written"`
	Definition string   `json:"definition" jsonschema:"plain text, one or two sentences; existing Term names in it become References"`
	Avoid      []string `json:"avoid,omitempty" jsonschema:"words the Term rules out in favour of itself"`
	Cluster    string   `json:"cluster,omitempty" jsonschema:"the Cluster to put the Term under; an unknown one is created"`
	Context    string   `json:"context,omitempty" jsonschema:"the Context whose Glossary gets the Term; required when the repository has more than one"`
}

type reviseInput struct {
	Name       string   `json:"name" jsonschema:"the Term's name as it is now"`
	NewName    string   `json:"new_name,omitempty" jsonschema:"the Term's new name, to rename it"`
	Definition string   `json:"definition" jsonschema:"the whole new definition, as plain text"`
	Avoid      []string `json:"avoid,omitempty" jsonschema:"the whole new list of words the Term rules out"`
	Cluster    string   `json:"cluster,omitempty" jsonschema:"the Cluster to move the Term to"`
	Context    string   `json:"context,omitempty" jsonschema:"the Term's Context; required when the repository has more than one"`
}

type removeInput struct {
	Name    string `json:"name" jsonschema:"the Term's name"`
	Context string `json:"context,omitempty" jsonschema:"the Term's Context; required when the repository has more than one"`
}

type recordInput struct {
	Title string `json:"title" jsonschema:"the decision's title"`
	Text  string `json:"text" jsonschema:"the ADR's paragraphs, as plain text"`
}

// addWriteTools adds the four write tools to s. They are listed for every Person; a Reader's call is refused.
func (o Options) addWriteTools(s *mcp.Server) {
	write := &mcp.ToolAnnotations{DestructiveHint: new(bool), OpenWorldHint: new(bool)}
	destructive := &mcp.ToolAnnotations{OpenWorldHint: new(bool)}
	mcp.AddTool(s, &mcp.Tool{Name: "define_term", Description: defineDescription, Annotations: write},
		func(_ context.Context, req *mcp.CallToolRequest, in defineInput) (*mcp.CallToolResult, any, error) {
			p, err := o.writer(req)
			if err != nil {
				return nil, nil, err
			}
			out, err := o.define(p, in)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
		})
	notYet := func(tool string) error { return fmt.Errorf("%s is not yet available on this Server.", tool) }
	mcp.AddTool(s, &mcp.Tool{Name: "revise_term", Description: reviseDescription, Annotations: destructive},
		func(context.Context, *mcp.CallToolRequest, reviseInput) (*mcp.CallToolResult, any, error) {
			return nil, nil, notYet("revise_term")
		})
	mcp.AddTool(s, &mcp.Tool{Name: "remove_term", Description: removeDescription, Annotations: destructive},
		func(context.Context, *mcp.CallToolRequest, removeInput) (*mcp.CallToolResult, any, error) {
			return nil, nil, notYet("remove_term")
		})
	mcp.AddTool(s, &mcp.Tool{Name: "record_adr", Description: recordDescription, Annotations: write},
		func(_ context.Context, req *mcp.CallToolRequest, in recordInput) (*mcp.CallToolResult, any, error) {
			p, err := o.writer(req)
			if err != nil {
				return nil, nil, err
			}
			out, err := o.record(p, in)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
		})
}

// writer is the Person calling, refused unless a Writer.
func (o Options) writer(req *mcp.CallToolRequest) (person.Person, error) {
	var header http.Header
	if req.Extra != nil {
		header = req.Extra.Header
	}
	p, err := o.People.Resolve(header)
	if err != nil {
		return person.Person{}, err
	}
	if p.Role != person.Writer {
		values := strings.Join(o.Writers.Values, ", ")
		if len(o.Writers.Values) > 1 {
			values = "one of " + values
		}
		return person.Person{}, fmt.Errorf("%s is a Reader here, and only a Person whose %s claim holds %s may make Changes.",
			called(p), o.Writers.Claim, values)
	}
	return p, nil
}

// define lands t as a new Term in the open Batch and answers the Term as it now reads, the commit, the Batch and the
// Findings the Change introduced.
func (o Options) define(p person.Person, in defineInput) (string, error) {
	t := edit.Term{Name: strings.TrimSpace(in.Name), Definition: in.Definition, Avoid: in.Avoid, Cluster: in.Cluster}
	if t.Name == "" {
		return "", errors.New("The Term's name is empty.")
	}
	if strings.TrimSpace(t.Definition) == "" {
		return "", fmt.Errorf("The definition of %s is empty.", t.Name)
	}
	if o.Batches == nil {
		return "", errors.New("This Server has no origin to land Changes in.")
	}
	var entry string
	var introduced []health.Finding
	landed, err := o.Batches.Change(batch.Identity{Name: p.Name, Email: p.Email}, func() (batch.Edit, error) {
		before, err := glossary.Load(o.Root)
		if err != nil {
			return batch.Edit{}, err
		}
		c, err := contextNamed(before, in.Context)
		if err != nil {
			return batch.Edit{}, err
		}
		t.Context = c.Key
		path := filepath.Join(o.Root, filepath.FromSlash(c.Path))
		text, err := os.ReadFile(path)
		if err != nil {
			return batch.Edit{}, err
		}
		out, err := edit.Define(string(text), before, t)
		if err != nil {
			return batch.Edit{}, errors.New(sentence(err.Error()))
		}
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			return batch.Edit{}, err
		}
		after, err := glossary.Load(o.Root)
		if err != nil {
			return batch.Edit{}, fmt.Errorf("The Glossary would no longer read after defining %s: %s.", t.Name, err)
		}
		entry = termEntry(after, c.Key, t.Name)
		introduced = newFindings(health.Check(before), health.Check(after))
		return batch.Edit{Subject: "Define Term " + t.Name, Paths: []string{c.Path}}, nil
	})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(o.Log, "Change define_term %s by %s: %s on %s\n", t.Name, label(p), landed.Commit, landed.Batch)
	var b strings.Builder
	fmt.Fprintf(&b, "Defined %s in commit %s on the Batch %s.\n\n%s", t.Name, landed.Commit, landed.Batch, entry)
	if len(introduced) == 0 {
		b.WriteString("\nThe Change introduced no Findings.\n")
	} else {
		b.WriteString("\nFindings the Change introduced:\n")
		health.Write(&b, introduced)
	}
	return b.String(), nil
}

// contextNamed is the Context named by key or name, regardless of case, or the only Context when named is empty.
func contextNamed(model *glossary.Model, named string) (glossary.Context, error) {
	named = strings.TrimSpace(named)
	var names []string
	for _, c := range model.Contexts {
		if named != "" && (strings.EqualFold(c.Name, named) || strings.EqualFold(c.Key, named)) {
			return c, nil
		}
		names = append(names, c.Name)
	}
	if named == "" {
		if len(model.Contexts) == 1 {
			return model.Contexts[0], nil
		}
		return glossary.Context{}, fmt.Errorf("Name the Context, since this repository has %s.", list(names))
	}
	return glossary.Context{}, fmt.Errorf("No Context %s; this repository has %s.", named, list(names))
}

// termEntry is the Term named in the Context with key as lookup writes it.
func termEntry(model *glossary.Model, key, named string) string {
	for id, t := range model.Terms {
		if t.Context == key && strings.EqualFold(t.Name, named) {
			var b strings.Builder
			lookup.New(model).WriteTerm(&b, id)
			return b.String()
		}
	}
	return ""
}

// newFindings is the Findings in after that before did not have, matched by rule and name since lines move.
func newFindings(before, after []health.Finding) []health.Finding {
	seen := map[string]bool{}
	for _, f := range before {
		seen[f.Rule+"|"+f.Name] = true
	}
	var out []health.Finding
	for _, f := range after {
		if !seen[f.Rule+"|"+f.Name] {
			out = append(out, f)
		}
	}
	return out
}

// called is how a refusal names a Person.
func called(p person.Person) string {
	if p.Name != "" {
		return p.Name
	}
	if p.Email != "" {
		return p.Email
	}
	return "This Person"
}

// list joins names as prose: "a", "a and b", "a, b and c".
func list(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// sentence is s with a capital first letter and a full stop.
func sentence(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:] + "."
}
