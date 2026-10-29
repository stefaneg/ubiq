package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Checker matches free text against glossary term names.
type Checker struct {
	model   *Model
	entries map[string][]string // lowercase term name -> term IDs
	re      *regexp.Regexp
}

// Finding is one phrase found in the checked text.
type Finding struct {
	Line    int
	Written string   // as it appears in the text
	TermIDs []string // the terms recognised; empty for a candidate
}

// Report is the outcome of checking one text.
type Report struct {
	Terms      []Finding // glossary terms recognised
	Candidates []Finding // Capitalised Phrases with no entry
	model      *Model
}

var reCandidate = regexp.MustCompile(`(^|[^\w-])([A-Z][a-z]+(?:[- ][A-Z][a-z]+)+)($|[^\w-])`)

// NewChecker indexes every term name and the head-noun plurals of "<head> of <rest>" names, longest
// phrase first.
func NewChecker(model *Model) *Checker {
	c := &Checker{model: model, entries: map[string][]string{}}
	for _, t := range model.Terms {
		key := strings.ToLower(strings.TrimSpace(t.Name))
		c.entries[key] = append(c.entries[key], t.ID)
	}
	phrases := make([]string, 0, len(c.entries))
	for p := range c.entries {
		phrases = append(phrases, p)
		phrases = append(phrases, headPlurals(p)...)
	}
	sort.Slice(phrases, func(i, j int) bool {
		if len(phrases[i]) != len(phrases[j]) {
			return len(phrases[i]) > len(phrases[j])
		}
		return phrases[i] < phrases[j]
	})
	quoted := make([]string, len(phrases))
	for i, p := range phrases {
		quoted[i] = regexp.QuoteMeta(p)
	}
	c.re = regexp.MustCompile(`(?i)(^|[^\w-])(` + strings.Join(quoted, "|") + `)(s|es)?($|[^\w-])`)
	return c
}

// Check scans text line by line.
func (c *Checker) Check(text string) *Report {
	r := &Report{model: c.model}
	for i, line := range strings.Split(text, "\n") {
		c.checkLine(i+1, line, r)
	}
	r.sortTermIDs()
	return r
}

// checkLine resumes each search right after the previous phrase so adjacent phrases both match.
func (c *Checker) checkLine(n int, line string, r *Report) {
	var taken [][2]int
	pos := 0
	for pos <= len(line) {
		m := c.re.FindStringSubmatchIndex(line[pos:])
		if m == nil {
			break
		}
		start, end := pos+m[4], pos+m[5]
		written := line[start:end]
		r.Terms = append(r.Terms, Finding{Line: n, Written: written, TermIDs: c.lookup(written)})
		if m[7] > m[5] {
			end = pos + m[7] // plural suffix
		}
		taken = append(taken, [2]int{start, end})
		pos = end
	}
	for _, m := range reCandidate.FindAllStringSubmatchIndex(line, -1) {
		if overlaps(taken, m[4], m[5]) {
			continue
		}
		r.Candidates = append(r.Candidates, Finding{Line: n, Written: line[m[4]:m[5]]})
	}
}

func (c *Checker) lookup(written string) []string {
	key := strings.ToLower(written)
	if ids, ok := c.entries[key]; ok {
		return ids
	}
	for _, s := range singulars(key) {
		if ids, ok := c.entries[s]; ok {
			return ids
		}
	}
	return nil
}

func overlaps(taken [][2]int, start, end int) bool {
	for _, t := range taken {
		if start < t[1] && end > t[0] {
			return true
		}
	}
	return false
}

func (r *Report) sortTermIDs() {
	for _, f := range r.Terms {
		sort.Strings(f.TermIDs)
	}
}

// Write renders the report as text: the recognised terms, then the candidates.
func (r *Report) Write(w io.Writer) {
	fmt.Fprintf(w, "terms recognised: %s\n", r.distinct(r.Terms))
	fmt.Fprintf(w, "capitalised phrases without an entry: %s\n", r.distinctWritten(r.Candidates))
}

func (r *Report) termList(ids []string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		t := r.model.Terms[id]
		parts[i] = fmt.Sprintf("%s (%s)", t.Name, r.model.contextName(t.Context))
	}
	return strings.Join(parts, "; ")
}

func (r *Report) distinct(fs []Finding) string {
	seen := map[string]bool{}
	var ids []string
	for _, f := range fs {
		for _, id := range f.TermIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "none"
	}
	return r.termList(ids)
}

func (r *Report) distinctWritten(fs []Finding) string {
	seen := map[string]bool{}
	var out []string
	for _, f := range fs {
		if !seen[f.Written] {
			seen[f.Written] = true
			out = append(out, f.Written)
		}
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}

// contextName is the heading of the context with key, or the key when the model has no such context.
func (m *Model) contextName(key string) string {
	for _, c := range m.Contexts {
		if c.Key == key {
			return c.Name
		}
	}
	return key
}
