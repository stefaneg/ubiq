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

// maxStatements caps the Statements written for a Term, so that a Term the Domain Model mentions everywhere stays a
// short answer.
const maxStatements = 5

// Write renders the result as text: a block for each Term the word names, then a block for each Term that avoids it,
// or failing both, a line for each of its parts, or failing those too, a line for each Term that mentions it.
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
	if len(r.Parts) > 0 {
		fmt.Fprintf(w, "%q names no Term, and no Term avoids it. Its parts:\n", r.Word)
		for _, p := range r.Parts {
			fmt.Fprintf(w, "  %s: %s\n", p.Words, l.partAnswer(p))
		}
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
	field("co-mentioned with", l.names(l.coMentions[t.ID], t.Context))
	statements := l.statements[t.ID]
	for _, s := range statements[:min(len(statements), maxStatements)] {
		field("statement", fmt.Sprintf("%s: %s (%s:%d)", s.Label, glossary.PlainText(s.HTML), s.Path, s.Line))
	}
	if n := len(statements) - maxStatements; n > 0 {
		fmt.Fprintf(w, "  and %d more statements\n", n)
	}
	for _, a := range m.ADRs {
		if slices.Contains(a.TermIDs, t.ID) {
			field("ADR", fmt.Sprintf("%s (%s)", a.Title, a.Path))
		}
	}
}

// partAnswer is the one-line answer for a part: the Terms it names and the Terms to use instead, each by name,
// Context and place.
func (l *Lookup) partAnswer(p Part) string {
	var answer []string
	if len(p.Named) > 0 {
		answer = append(answer, l.places(p.Named))
	}
	if len(p.Avoided) > 0 {
		lead := "an Avoided Word"
		if len(p.Named) > 0 {
			lead = "Also an Avoided Word"
		}
		answer = append(answer, lead+". Use instead: "+l.places(p.Avoided))
	}
	if len(answer) == 0 {
		return "names no Term, and no Term avoids it"
	}
	return strings.Join(answer, ". ")
}

// places lists the Terms as "Name (Context), path:line".
func (l *Lookup) places(ids []string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		t := l.model.Terms[id]
		out[i] = fmt.Sprintf("%s (%s), %s:%d", t.Name, l.model.ContextName(t.Context), l.contextPath(t.Context), t.Line)
	}
	return strings.Join(out, "; ")
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
