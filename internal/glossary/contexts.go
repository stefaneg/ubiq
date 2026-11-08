package glossary

import (
	"regexp"
	"sort"
	"strings"
)

// contextNames finds the Contexts a Statement names: by heading or by the map's link text, in bold, in a bold label
// such as "Ordering → Billing", or written capitalised exactly as the name.
type contextNames struct {
	byLower  map[string]string // lowercase name -> key, for bold phrases
	exact    map[string]string // name as written -> key, for plain text
	maxWords int
}

func newContextNames(contexts []*rawContext) *contextNames {
	c := &contextNames{byLower: map[string]string{}, exact: map[string]string{}}
	for _, rc := range contexts {
		for _, n := range []string{rc.name, rc.mapName} {
			if n == "" {
				continue
			}
			c.byLower[strings.ToLower(n)] = rc.key
			c.exact[n] = rc.key
			c.maxWords = max(c.maxWords, len(reWord.FindAllStringIndex(n, -1)))
		}
	}
	return c
}

// key is the key of the Context a bold phrase names, or "".
func (c *contextNames) key(phrase string) string {
	return c.byLower[strings.ToLower(strings.TrimSpace(phrase))]
}

// label splits a bold phrase that joins Context names, such as "Ordering → Billing", into its parts. A phrase is a
// label when it has two or more parts and either an arrow or a part that names a Context; otherwise label is nil.
func (c *contextNames) label(phrase string) []string {
	parts := labelNames(phrase)
	if len(parts) < 2 {
		return nil
	}
	if reArrow.MatchString(phrase) {
		return parts
	}
	for _, p := range parts {
		if c.key(p) != "" {
			return parts
		}
	}
	return nil
}

var (
	reArrow     = regexp.MustCompile(`<->|->|<-|[→↔←⇄]`)
	reLabelSep  = regexp.MustCompile(`\s*(?:<->|->|<-|[→↔←⇄,&]|\band\b)\s*`)
	reNonSpaces = regexp.MustCompile(`\S`)
)

// labelNames splits a label such as "Ordering → Billing" into the names it joins.
func labelNames(label string) []string {
	var out []string
	for _, n := range reLabelSep.Split(label, -1) {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// named lists the keys of the Contexts text names, in the order it names them, each once. Code spans are skipped,
// and a link counts by its text.
func (c *contextNames) named(text string) []string {
	blank := func(m string) string { return reNonSpaces.ReplaceAllString(m, " ") }
	text = reLink.ReplaceAllString(reCode.ReplaceAllStringFunc(text, blank), "$1")
	type hit struct {
		at  int
		key string
	}
	var hits []hit
	for _, m := range reBold.FindAllStringSubmatchIndex(text, -1) {
		phrase := text[m[2]:m[3]]
		if k := c.key(phrase); k != "" {
			hits = append(hits, hit{m[0], k})
			continue
		}
		for _, p := range c.label(phrase) {
			if k := c.key(p); k != "" {
				hits = append(hits, hit{m[0], k})
			}
		}
	}
	plain := reBold.ReplaceAllStringFunc(text, blank)
	words := reWord.FindAllStringIndex(plain, -1)
	for i := 0; i < len(words); i++ {
		for n := min(c.maxWords, len(words)-i); n > 0; n-- {
			start, end := words[i][0], words[i+n-1][1]
			if k, ok := c.exact[plain[start:end]]; ok && singleSpaced(plain[start:end]) {
				hits = append(hits, hit{start, k})
				i += n - 1
				break
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	var keys []string
	for _, h := range hits {
		keys = union(keys, []string{h.key})
	}
	return keys
}
