// Package glossary builds the glossary Model from a repository's GLOSSARY.md files, GLOSSARY-MAP.md and ADRs.
package glossary

import (
	"html"
	"regexp"
)

// Context is one glossary file.
type Context struct {
	Key      string    `json:"key"`  // slug of Name; prefixes the context's term ids
	Name     string    `json:"name"` // heading text
	Path     string    `json:"path"`
	Intro    string    `json:"intro"` // html
	Clusters []Cluster `json:"clusters"`
}

type Cluster struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Intro   string   `json:"intro"` // html
	TermIDs []string `json:"terms"`
}

// Term is one glossary entry.
type Term struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Context    string   `json:"context"`
	Cluster    string   `json:"cluster"`
	Definition string   `json:"definition"` // html with resolved term links
	Avoid      string   `json:"avoid"`      // the Avoid line as written
	Redirects  []string `json:"redirects"`  // avoided words that name a synonym, see Term.redirects
	Refs       []string `json:"refs"`       // other terms the definition refers to, in bold or capitalised
	Line       int      `json:"line"`
}

// Statement is one block of a Glossary or the Context Map outside a Term entry: a paragraph, a bullet with its
// continuation lines, a table row, or a blockquote. Only Statements that mention a Term or name a Context are kept.
type Statement struct {
	Label    string   `json:"label"` // the headings above it below the "#" title, as "A › B"; the title above them all
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	HTML     string   `json:"html"`
	TermIDs  []string `json:"terms"`    // the Terms it and its nearest heading mention
	Contexts []string `json:"contexts"` // keys of the Contexts it and its nearest heading name, in order
}

type Unresolved struct {
	Phrase  string `json:"phrase"`
	Path    string `json:"path"`    // the file the phrase is in
	Context string `json:"context"` // the context the phrase resolves from; empty for a relationship label
	Where   string `json:"where"`
}

type Model struct {
	Contexts []Context        `json:"contexts"`
	Terms    map[string]*Term `json:"terms"`
	// Statements is the Statements of every Glossary, then of the Context Map.
	Statements []Statement  `json:"statements"`
	Unresolved []Unresolved `json:"unresolved"`
	ADRs       []ADR        `json:"adrs"`
}

// IsContextRelationship reports whether the Statement names two or more Contexts.
func (s Statement) IsContextRelationship() bool { return len(s.Contexts) >= 2 }

// ContextName is the heading of the context with key, or the key when the model has no such context.
func (m *Model) ContextName(key string) string {
	for _, c := range m.Contexts {
		if c.Key == key {
			return c.Name
		}
	}
	return key
}

var reTag = regexp.MustCompile(`<[^>]*>`)

// PlainText is the text of rendered html, such as a Term's Definition, without its tags.
func PlainText(h string) string {
	return html.UnescapeString(reTag.ReplaceAllString(h, ""))
}
