package glossary

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ADR is one docs/adr/*.md file with glossary names linked in its body.
type ADR struct {
	ID      string   `json:"id"` // adr/<file slug>
	Title   string   `json:"title"`
	Path    string   `json:"path"`
	Context string   `json:"context"` // core whose vocabulary dominates the body
	HTML    string   `json:"html"`
	TermIDs []string `json:"terms"`
}

type rawADR struct {
	path, title string
	context     string // owning context for a context's own ADR; empty for a system-wide one
	body        []string
}

// parseADRs reads the system-wide docs/adr/*.md under root, then each context's own, from docs/adr beside its
// glossary. A missing directory yields no ADRs.
func parseADRs(root string, contexts []*rawContext) ([]rawADR, error) {
	out, err := parseADRDir(root, "docs/adr", "")
	if err != nil {
		return nil, err
	}
	for _, rc := range contexts {
		if dir := path.Dir(rc.path); dir != "." {
			own, err := parseADRDir(root, path.Join(dir, "docs/adr"), rc.key)
			if err != nil {
				return nil, err
			}
			out = append(out, own...)
		}
	}
	return out, nil
}

// parseADRDir reads every *.md in dir, a slash-separated path relative to root.
func parseADRDir(root, dir, context string) ([]rawADR, error) {
	paths, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.md"))
	sort.Strings(paths)
	var out []rawADR
	for _, p := range paths {
		lines, err := readLines(p)
		if err != nil {
			return nil, fmt.Errorf("read ADR: %w", err)
		}
		rel, _ := filepath.Rel(root, p)
		a := rawADR{path: filepath.ToSlash(rel), context: context}
		for i, ln := range lines {
			if m := reH.FindStringSubmatch(ln); m != nil && len(m[1]) == 1 && a.title == "" {
				a.title = m[2]
				a.body = lines[i+1:]
				break
			}
		}
		if a.title == "" {
			a.title = filepath.Base(p)
			a.body = lines
		}
		out = append(out, a)
	}
	return out, nil
}

// autoBold wraps plain-text occurrences of glossary names in ** so the
// resolver links them. Longest names first; code spans and existing bold are
// left alone. Matching is case-sensitive on the exact name, with an optional
// plural suffix.
func (r *Resolver) autoBold(line string, names []string, res []*regexp.Regexp, found map[string]bool) string {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return line
	}
	var keep []string
	protect := func(re *regexp.Regexp) {
		line = re.ReplaceAllStringFunc(line, func(m string) string {
			keep = append(keep, m)
			return "\x03" + string(rune('A'+len(keep)-1)) + "\x03"
		})
	}
	protect(reCode)
	protect(reBold)
	var toks []string
	for i, re := range res {
		line = re.ReplaceAllStringFunc(line, func(m string) string {
			sub := re.FindStringSubmatch(m)
			found[names[i]] = true
			toks = append(toks, "**"+sub[2]+"**")
			return sub[1] + "\x02" + string(rune('A'+len(toks)-1)) + "\x02" + sub[3]
		})
	}
	for i, t := range toks {
		line = strings.Replace(line, "\x02"+string(rune('A'+i))+"\x02", t, 1)
	}
	for i, k := range keep {
		line = strings.Replace(line, "\x03"+string(rune('A'+i))+"\x03", k, 1)
	}
	return line
}

func (r *Resolver) renderADRs(raws []rawADR) []ADR {
	// every distinct name, longest first
	nameSet := map[string]bool{}
	for _, m := range r.byContext {
		for _, id := range m {
			nameSet[r.model.Terms[id].Name] = true
		}
	}
	var names []string
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	var res []*regexp.Regexp
	for _, n := range names {
		res = append(res, regexp.MustCompile(`(^|[^\w-])(`+regexp.QuoteMeta(n)+`(?:s|es)?)($|[^\w-])`))
	}
	r.lenient = true
	defer func() { r.lenient = false }()
	var out []ADR
	for _, a := range raws {
		found := map[string]bool{}
		body := make([]string, len(a.body))
		for i, ln := range a.body {
			body[i] = r.autoBold(ln, names, res, found)
		}
		best, id := a.context, "adr/"+a.context+"/"+strings.TrimSuffix(path.Base(a.path), ".md")
		if best == "" {
			best, id = r.homeContext(found), "adr/"+strings.TrimSuffix(path.Base(a.path), ".md")
		}
		h, ids := r.block(body, best, "ADR "+a.title)
		out = append(out, ADR{ID: id, Title: a.title, Path: a.path, Context: best, HTML: h, TermIDs: ids})
	}
	return out
}

// homeContext is the context whose names a system-wide ADR uses most, the shared context not counting. A tie goes
// to the earlier context in resolution order, which starts with the shared one.
func (r *Resolver) homeContext(found map[string]bool) string {
	best, bestN := r.order[0], 0
	for _, k := range r.order {
		if k == r.shared {
			continue
		}
		n := 0
		for name := range found {
			if _, ok := r.byContext[k][strings.ToLower(name)]; ok {
				n++
			}
		}
		if n > bestN {
			best, bestN = k, n
		}
	}
	return best
}
