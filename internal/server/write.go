package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
	"github.com/stefaneg/ubiq/internal/layout"
	"github.com/stefaneg/ubiq/internal/lookup"
	"github.com/stefaneg/ubiq/internal/mcpserver"
	"github.com/stefaneg/ubiq/internal/person"
	"github.com/stefaneg/ubiq/internal/prose"
)

const defineDescription = `Define a new Term in this repository's glossary. Write the definition as plain text: every ` +
	`mention of an existing Term, or its plural, becomes a Reference, and everything else stays as typed. The Term goes ` +
	`at the end of the Cluster you name, a Cluster that does not exist yet is created, and with no Cluster it goes ` +
	`after the last Term. Name the Context when the repository has more than one. The Term lands at once as a commit ` +
	`authored by you in the open Batch, a ubiq/<n> branch a Maintainer merges, and the next lookup sees it. The answer ` +
	`is the Term as it now reads, the commit, the Batch, and the Findings the Change introduced, which warn and never ` +
	`refuse. Refused when you are a Reader, when the Term exists in that Context, when the Context does not exist, or ` +
	`when the name or definition is empty. Look the words up first so the definition reuses existing Terms.`

const reviseDescription = `Revise a Term in this repository's glossary. Look it up first: the definition, as plain ` +
	`text, and the avoid list replace the Term's in full, and existing Term names in the definition become References. ` +
	`Name a Cluster to move the Term to the end of it, creating it when it does not exist; give new_name to rename the ` +
	`Term and rewrite every Reference to it in every glossary. Name the Context when the repository has more than one. ` +
	`The Change lands at once as a commit authored by you in the open Batch, a ubiq/<n> branch a Maintainer merges. ` +
	`The answer is the Term as it now reads, the commit, the Batch, and the Findings the Change introduced. Refused ` +
	`when you are a Reader, when the Term does not exist in that Context or new_name names another that does, when the ` +
	`Context does not exist, or when the name or definition is empty.`

const removeDescription = `Remove a Term from this repository's glossary. Name the Context when the repository has ` +
	`more than one. The Change lands at once as a commit authored by you in the open Batch, a ubiq/<n> branch a ` +
	`Maintainer merges. The answer is the commit, the Batch, and the Findings the Change introduced, among them each ` +
	`Unresolved Phrase where the Term was referenced, to repair with revise_term next. Refused when you are a Reader, ` +
	`when the Term does not exist in that Context, when the Context does not exist, or when the name is empty.`

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

// term is the Term an input names, written as the Agent wrote it.
func (in defineInput) term() layout.Term {
	return layout.Term{Name: in.Name, Definition: in.Definition, Avoid: in.Avoid, Cluster: in.Cluster}
}

func (in reviseInput) term() layout.Term {
	return layout.Term{Name: in.Name, Definition: in.Definition, Avoid: in.Avoid, Cluster: in.Cluster}
}

// addWriteTools adds the four write tools to s. They are listed for every Person; a Reader's call is refused.
func (o Options) addWriteTools(s *mcp.Server) {
	write := &mcp.ToolAnnotations{DestructiveHint: new(bool), OpenWorldHint: new(bool)}
	destructive := &mcp.ToolAnnotations{OpenWorldHint: new(bool)}
	addWriteTool(o, s, &mcp.Tool{Name: "define_term", Description: defineDescription, Annotations: write}, o.define)
	addWriteTool(o, s, &mcp.Tool{Name: "revise_term", Description: reviseDescription, Annotations: destructive}, o.revise)
	addWriteTool(o, s, &mcp.Tool{Name: "remove_term", Description: removeDescription, Annotations: destructive}, o.remove)
	addWriteTool(o, s, &mcp.Tool{Name: "record_adr", Description: recordDescription, Annotations: write}, o.record)
}

// addWriteTool adds tool to s, answering a Writer's call with what run answers and refusing anyone else's.
func addWriteTool[In any](o Options, s *mcp.Server, tool *mcp.Tool, run func(person.Person, In) (string, error)) {
	mcp.AddTool(s, tool, func(_ context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		p, err := o.writer(req)
		if err != nil {
			return nil, nil, err
		}
		out, err := run(p, in)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
	})
}

// writer is the Person calling, refused unless a Writer.
func (o Options) writer(req *mcp.CallToolRequest) (person.Person, error) {
	p, err := o.People.Resolve(mcpserver.Header(req))
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

// change is what one write tool changes: the new text of each file, by path relative to the root, the commit's
// subject, and what the log line names, a Term or an ADR's path.
type change struct {
	files   map[string]string
	subject string
	named   string
}

// landed is a Change as it landed: its Batch and commit, the Domain Model after it, and the Findings it introduced.
type landed struct {
	batch.Landed
	after      *glossary.Model
	introduced []health.Finding
}

// land lands one Change in the open Batch for p, the one flow every write tool shares. valid is why the input is
// refused, before the repository is locked or the Batch prepared, or nil. run gets the Domain Model as read and
// answers what to change; an error it answers is refused as a sentence. The Change is refused, and nothing lands,
// when the Domain Model no longer reads after it.
func (o Options) land(p person.Person, tool string, valid error, run func(before *glossary.Model) (change, error)) (landed, error) {
	if valid != nil {
		return landed{}, errors.New(prose.Sentence(valid.Error()))
	}
	if o.Batches == nil {
		return landed{}, errors.New("This Server has no origin to land Changes in.")
	}
	var l landed
	var c change
	at, err := o.Batches.Change(batch.Identity{Name: p.Name, Email: p.Email}, func() (batch.Commit, error) {
		before, err := glossary.Load(o.Root)
		if err != nil {
			return batch.Commit{}, err
		}
		if c, err = run(before); err != nil {
			return batch.Commit{}, errors.New(prose.Sentence(err.Error()))
		}
		var paths []string
		for path, text := range c.files {
			file := filepath.Join(o.Root, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				return batch.Commit{}, err
			}
			if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
				return batch.Commit{}, err
			}
			paths = append(paths, path)
		}
		slices.Sort(paths)
		if l.after, err = glossary.Load(o.Root); err != nil {
			return batch.Commit{}, fmt.Errorf("The Domain Model would no longer read after the Change %q: %s.", c.subject, err)
		}
		l.introduced = newFindings(health.Check(before), health.Check(l.after))
		return batch.Commit{Subject: c.subject, Paths: paths}, nil
	})
	if err != nil {
		return landed{}, err
	}
	l.Landed = at
	fmt.Fprintf(o.Log, "Change %s %s by %s: %s on %s\n", tool, c.named, label(p), at.Commit, at.Batch)
	return l, nil
}

// answer is a write tool's answer: what it did, with the commit and the Batch, then body, the Term or ADR as it now
// reads, when not empty, then the Findings the Change introduced.
func (l landed) answer(did, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s in commit %s on the Batch %s.\n", did, l.Commit, l.Batch)
	if body != "" {
		b.WriteString("\n" + body)
	}
	if len(l.introduced) == 0 {
		b.WriteString("\nThe Change introduced no Findings.\n")
	} else {
		b.WriteString("\nFindings the Change introduced:\n")
		health.Write(&b, l.introduced)
	}
	return b.String()
}

// lookedUp is the Term of the Context with key named named as lookup writes it, or empty when there is none.
func (l landed) lookedUp(key, named string) string {
	t := l.after.TermNamed(key, named)
	if t == nil {
		return ""
	}
	var b strings.Builder
	lookup.New(l.after).WriteTerm(&b, t.ID)
	return b.String()
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
		return glossary.Context{}, fmt.Errorf("name the Context, since this repository has %s", prose.List(names))
	}
	return glossary.Context{}, fmt.Errorf("no Context %s; this repository has %s", named, prose.List(names))
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

// read is the text of the file at path relative to the root.
func (o Options) read(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(path)))
	return string(b), err
}
