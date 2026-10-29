// Package glossary builds the glossary Model from a repository's GLOSSARY.md files, GLOSSARY-MAP.md and ADRs.
package glossary

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
	Refs       []string `json:"refs"`       // outbound term ids from the definition
	Line       int      `json:"line"`
}

// ContextRelationship is one bullet of the context map's Relationships section, such as
// "- **Ordering → Billing**: Ordering emits events Billing consumes".
type ContextRelationship struct {
	Label    string   `json:"label"`    // the bold label as written
	Contexts []string `json:"contexts"` // keys of the contexts the label names, in label order
	HTML     string   `json:"html"`     // the statement after the label
	TermIDs  []string `json:"terms"`    // terms the statement refers to
}

type Unresolved struct {
	Phrase  string `json:"phrase"`
	Path    string `json:"path"` // the file the phrase is in
	Context string `json:"context"`
	Where   string `json:"where"`
}

type Model struct {
	Contexts []Context        `json:"contexts"`
	Terms    map[string]*Term `json:"terms"`
	// Relationships is the context map's Relationships section; empty without a map.
	Relationships []ContextRelationship `json:"relationships"`
	Unresolved    []Unresolved          `json:"unresolved"`
	ADRs          []ADR                 `json:"adrs"`
}

// ContextName is the heading of the context with key, or the key when the model has no such context.
func (m *Model) ContextName(key string) string {
	for _, c := range m.Contexts {
		if c.Key == key {
			return c.Name
		}
	}
	return key
}
