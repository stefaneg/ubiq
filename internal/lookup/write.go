package lookup

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// maxMentions caps the Terms listed for a word that names none, so that a common word stays a short answer.
const maxMentions = 10

// Write renders the result as text: a block for each Term the word names, then a block for each Term that avoids it,
// or failing both, a line for each Term that mentions it.
func (r *Result) Write(w io.Writer) {
	l := r.lookup
	for _, id := range r.Named {
		l.writeTerm(w, l.model.Terms[id])
	}
	if len(r.Avoided) > 0 {
		fmt.Fprintf(w, "%q is an Avoided Word. Use instead:\n", r.Word)
		for _, id := range r.Avoided {
			l.writeTerm(w, l.model.Terms[id])
		}
	}
	if len(r.Named) > 0 || len(r.Avoided) > 0 {
		return
	}
	if len(r.Mentions) == 0 {
		fmt.Fprintf(w, "%q names no Term, no Term avoids it, and no Term mentions it.\n", r.Word)
		return
	}
	fmt.Fprintf(w, "%q names no Term, and no Term avoids it. Terms that mention it:\n", r.Word)
	for _, id := range r.Mentions[:min(len(r.Mentions), maxMentions)] {
		t := l.model.Terms[id]
		fmt.Fprintf(w, "  %s (%s): %s\n", t.Name, l.model.ContextName(t.Context), glossary.PlainText(t.Definition))
	}
	if n := len(r.Mentions) - maxMentions; n > 0 {
		fmt.Fprintf(w, "  and %d more\n", n)
	}
}

// writeTerm writes a Term's name, Context and place, its definition, and each line of what else the model says about
// it, leaving out the lines that would be empty.
func (l *Lookup) writeTerm(w io.Writer, t *glossary.Term) {
	m := l.model
	fmt.Fprintf(w, "%s (%s), %s:%d\n", t.Name, m.ContextName(t.Context), l.contextPath(t.Context), t.Line)
	fmt.Fprintf(w, "  %s\n", glossary.PlainText(t.Definition))
	field := func(label, value string) {
		if value != "" {
			fmt.Fprintf(w, "  %s: %s\n", label, value)
		}
	}
	field("avoid", t.Avoid)
	field("refers to", l.names(t.Refs, t.Context))
	field("referred to by", l.names(l.inbound[t.ID], t.Context))
	for _, rel := range m.Relationships {
		if slices.Contains(rel.TermIDs, t.ID) {
			field("relationship", rel.Label+": "+glossary.PlainText(rel.HTML))
		}
	}
	for _, a := range m.ADRs {
		if slices.Contains(a.TermIDs, t.ID) {
			field("ADR", fmt.Sprintf("%s (%s)", a.Title, a.Path))
		}
	}
}

// names lists the Terms by name, adding the Context of each one outside ctx.
func (l *Lookup) names(ids []string, ctx string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		t := l.model.Terms[id]
		parts[i] = t.Name
		if t.Context != ctx {
			parts[i] += " (" + l.model.ContextName(t.Context) + ")"
		}
	}
	return strings.Join(parts, ", ")
}

func (l *Lookup) contextPath(key string) string {
	for _, c := range l.model.Contexts {
		if c.Key == key {
			return c.Path
		}
	}
	return ""
}
