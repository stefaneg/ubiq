package check

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Checker matches free text against glossary term names and Avoided Words.
type Checker struct {
	model   *glossary.Model
	entries map[string][]string // lowercase term name -> term IDs
	avoided map[string][]string // lowercase Avoided Word -> IDs of the terms that avoid it
	re      *regexp.Regexp
}

// Finding is one phrase found in the checked text.
type Finding struct {
	Line    int
	Written string   // as it appears in the text
	TermIDs []string // the terms recognised, or for an Avoided Word the terms to use instead; empty for a candidate
}

// Report is the outcome of checking one text.
type Report struct {
	Terms      []Finding // glossary terms recognised
	Candidates []Finding // Capitalised Phrases with no entry
	Avoided    []Finding // Avoided Words, with the terms that avoid them
	model      *glossary.Model
}

var reCandidate = regexp.MustCompile(`(^|[^\w-])([A-Z][a-z]+(?:[- ][A-Z][a-z]+)+)($|[^\w-])`)

// reDeterminer matches an article or determiner that opens a capitalised run, such as the sentence-initial "The" in
// "The Upload Queue", which is not part of a Candidate Phrase.
var reDeterminer = regexp.MustCompile(`^(?:The|A|An|This|That|These|Those|Each|Every|Any|Some|No|All|Both|Either|Neither|Its|Their|Our|Your|My|His|Her)[- ]`)

// New indexes every term name, the head-noun plurals of "<head> of <rest>" names, and every Avoided Word that names
// a synonym, longest phrase first, so that "image reference" is an Avoided Word rather than the term "image".
func New(model *glossary.Model) *Checker {
	c := &Checker{model: model, entries: map[string][]string{}, avoided: map[string][]string{}}
	for _, t := range model.Terms {
		key := strings.ToLower(strings.TrimSpace(t.Name))
		c.entries[key] = append(c.entries[key], t.ID)
		for _, w := range t.Redirects {
			c.avoided[w] = append(c.avoided[w], t.ID)
		}
	}
	phrases := make([]string, 0, len(c.entries)+len(c.avoided))
	for p := range c.entries {
		phrases = append(phrases, p)
		phrases = append(phrases, glossary.HeadPlurals(p)...)
		phrases = append(phrases, iesPlural(p)...)
	}
	for w := range c.avoided {
		if _, ok := c.entries[w]; !ok {
			phrases = append(phrases, w)
			phrases = append(phrases, iesPlural(w)...)
		}
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
		if ids := find(c.entries, written); ids != nil {
			r.Terms = append(r.Terms, Finding{Line: n, Written: written, TermIDs: ids})
		} else {
			r.Avoided = append(r.Avoided, Finding{Line: n, Written: written, TermIDs: find(c.avoided, written)})
		}
		if m[7] > m[5] {
			end = pos + m[7] // plural suffix
		}
		taken = append(taken, [2]int{start, end})
		pos = end
	}
	for _, m := range reCandidate.FindAllStringSubmatchIndex(line, -1) {
		start, end := m[4], m[5]
		if lead := reDeterminer.FindString(line[start:end]); lead != "" {
			start += len(lead)
		}
		if !strings.ContainsAny(line[start:end], " -") || overlaps(taken, start, end) {
			continue
		}
		r.Candidates = append(r.Candidates, Finding{Line: n, Written: line[start:end]})
	}
}

// find is the entry of index for written, or for one of its singulars.
func find(index map[string][]string, written string) []string {
	for _, s := range glossary.Singulars(strings.ToLower(written)) {
		if ids, ok := index[s]; ok {
			return ids
		}
	}
	return nil
}

// iesPlural is the plural of a phrase ending in a consonant and y, such as "deliveries" for "delivery", which the
// trailing (s|es) match cannot reach. Other phrases yield nothing.
func iesPlural(p string) []string {
	if len(p) < 2 || p[len(p)-1] != 'y' || strings.ContainsRune("aeiouy", rune(p[len(p)-2])) {
		return nil
	}
	return []string{p[:len(p)-1] + "ies"}
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
	for _, fs := range [][]Finding{r.Terms, r.Avoided} {
		for _, f := range fs {
			sort.Strings(f.TermIDs)
		}
	}
}

// Write renders the report as text: the recognised terms, the candidates, then each Avoided Word with the terms to
// use instead, in the order the text first uses them.
func (r *Report) Write(w io.Writer) {
	fmt.Fprintf(w, "terms recognised: %s\n", r.distinct(r.Terms))
	fmt.Fprintf(w, "capitalised phrases without an entry: %s\n", r.distinctWritten(r.Candidates))
	fmt.Fprintf(w, "avoided words: %s\n", r.avoidedList())
}

func (r *Report) avoidedList() string {
	seen := map[string]bool{}
	var parts []string
	for _, f := range r.Avoided {
		word := strings.ToLower(f.Written)
		if seen[word] {
			continue
		}
		seen[word] = true
		parts = append(parts, word+" → "+r.termList(f.TermIDs, ", "))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

func (r *Report) termList(ids []string, sep string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		t := r.model.Terms[id]
		parts[i] = fmt.Sprintf("%s (%s)", t.Name, r.model.ContextName(t.Context))
	}
	return strings.Join(parts, sep)
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
	return r.termList(ids, "; ")
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
