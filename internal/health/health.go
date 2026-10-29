package health

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Finding is one glossary entry or phrase that breaks a health rule.
type Finding struct {
	Path string `json:"path"` // glossary file
	Line int    `json:"line"` // term line; 0 for an unresolved phrase
	Rule string `json:"rule"` // long | orphan | unmapped | unresolved
	Name string `json:"name"` // term name or bold phrase
	Note string `json:"note"`
	Term string `json:"term,omitempty"` // term id; empty for an unresolved phrase
}

// Check applies the page's health rules to the model and returns the findings sorted by file, line and rule.
//
//	long       definition longer than two sentences
//	orphan     no definition, relationship bullet, ambiguity or map entry refers to the term (ADRs do not count)
//	unmapped   an entry name defined in two contexts that the map does not name
//	unresolved a bold phrase with no entry
func Check(model *glossary.Model) []Finding {
	paths := map[string]string{}
	for _, c := range model.Contexts {
		paths[c.Key] = c.Path
	}
	inbound, noted, mapped, mapNames := map[string]int{}, map[string]int{}, map[string]int{}, map[string]bool{}
	byName := map[string][]string{}
	for id, t := range model.Terms {
		for _, r := range t.Refs {
			inbound[r]++
		}
		key := strings.ToLower(t.Name)
		byName[key] = append(byName[key], id)
	}
	for _, n := range model.Notes {
		for _, id := range n.TermIDs {
			noted[id]++
		}
	}
	for _, e := range model.MapEntries {
		mapNames[strings.ToLower(e.Name)] = true
		for _, id := range e.TermIDs {
			mapped[id]++
		}
	}
	var out []Finding
	add := func(t *glossary.Term, rule, note string) {
		out = append(out, Finding{Path: paths[t.Context], Line: t.Line, Rule: rule, Name: t.Name, Note: note, Term: t.ID})
	}
	for id, t := range model.Terms {
		if n := sentences(plainText(t.Definition)); n > 2 {
			add(t, "long", fmt.Sprintf("%d sentences", n))
		}
		if inbound[id] == 0 && noted[id] == 0 && mapped[id] == 0 {
			note := "no references in or out"
			if len(t.Refs) > 0 {
				note = fmt.Sprintf("refers to %d, referenced by none", len(t.Refs))
			}
			add(t, "orphan", note)
		}
		key := strings.ToLower(t.Name)
		if len(byName[key]) > 1 && !mapNames[key] {
			var others []string
			for _, other := range byName[key] {
				if other != id {
					others = append(others, model.ContextName(model.Terms[other].Context))
				}
			}
			sort.Strings(others)
			add(t, "unmapped", "also in "+strings.Join(others, ", "))
		}
	}
	for _, u := range model.Unresolved {
		out = append(out, Finding{Path: paths[u.Context], Rule: "unresolved", Name: u.Phrase,
			Note: model.ContextName(u.Context) + ": " + u.Where})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Name < b.Name
	})
	return out
}

var reTag = regexp.MustCompile(`<[^>]*>`)

func plainText(h string) string {
	return html.UnescapeString(reTag.ReplaceAllString(h, ""))
}

// sentences counts sentence ends: ".", "!" or "?" followed by whitespace or the end of the text, except inside an
// abbreviation such as "e.g." where a letter, a dot and a lowercase letter precede the terminator.
func sentences(s string) int {
	n := 0
	seen := false // text since the last terminator
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' || c == '!' || c == '?' {
			atEnd := i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\n' || s[i+1] == '\t'
			abbrev := i >= 3 && isLetter(s[i-3]) && s[i-2] == '.' && s[i-1] >= 'a' && s[i-1] <= 'z'
			if atEnd && !abbrev {
				if seen {
					n++
				}
				seen = false
				continue
			}
		}
		if !isSpace(c) {
			seen = true
		}
	}
	if seen {
		n++
	}
	return n
}

func isLetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }
func isSpace(c byte) bool  { return c == ' ' || c == '\n' || c == '\t' || c == '\r' }

// Write prints one finding per line as "path:line: rule: name: note"; the line number is omitted when the
// finding has none.
func Write(w io.Writer, findings []Finding) {
	for _, f := range findings {
		if f.Line > 0 {
			fmt.Fprintf(w, "%s:%d: %s: %s: %s\n", f.Path, f.Line, f.Rule, f.Name, f.Note)
		} else {
			fmt.Fprintf(w, "%s: %s: %s: %s\n", f.Path, f.Rule, f.Name, f.Note)
		}
	}
}

// Summary counts findings per rule in the page's rule order.
func Summary(findings []Finding, terms int) string {
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Rule]++
	}
	parts := make([]string, 0, 4)
	for _, r := range []string{"long", "orphan", "unmapped", "unresolved"} {
		parts = append(parts, fmt.Sprintf("%d %s", counts[r], r))
	}
	return fmt.Sprintf("%d findings over %d terms: %s", len(findings), terms, strings.Join(parts, ", "))
}
