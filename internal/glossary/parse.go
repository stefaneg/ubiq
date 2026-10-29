package glossary

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// ---- raw parse ------------------------------------------------------------

type rawTerm struct {
	name, def, avoid string
	line             int
}

type rawCluster struct {
	name  string
	intro []string
	terms []rawTerm
}

type rawContext struct {
	key, name, path string
	mapName         string // the link text the context map names the context by; empty without a map
	intro           []string
	clusters        map[string]*rawCluster
	clusterOrder    []string
}

var (
	reH        = regexp.MustCompile(`^(#{1,4}) (.*)$`)
	reTermLine = regexp.MustCompile(`^\*\*([^*]+)\*\*:\s*$`)
	reAvoid    = regexp.MustCompile(`^_Avoid_:\s*(.*)$`)
	reBullet   = regexp.MustCompile(`^(\s*)[-*] (.*)$`)
)

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

// parseContext reads one glossary file laid out as the domain-modeling skill's GLOSSARY-FORMAT.md describes: a "#"
// heading naming the context, an intro, and a "## Language" section whose "###" subheadings name clusters. Other
// "##" sections are not part of the format and are skipped. displayPath is the path shown to readers; path is where
// it is read from. The caller sets the key.
func parseContext(displayPath, path string) (*rawContext, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, fmt.Errorf("read glossary %s: %w", displayPath, err)
	}
	rc := &rawContext{path: displayPath, clusters: map[string]*rawCluster{}}
	getCluster := func(name string) *rawCluster {
		if c, ok := rc.clusters[name]; ok {
			return c
		}
		c := &rawCluster{name: name}
		rc.clusters[name] = c
		rc.clusterOrder = append(rc.clusterOrder, name)
		return c
	}
	inIntro, inLanguage := true, false
	var cur *rawCluster // nil until a term or line appears; terms before any subheading go to "General"
	for i := 0; i < len(lines); {
		ln := lines[i]
		if m := reH.FindStringSubmatch(ln); m != nil {
			switch len(m[1]) {
			case 1:
				rc.name = m[2]
			case 2:
				inIntro, inLanguage, cur = false, strings.EqualFold(strings.TrimSpace(m[2]), "Language"), nil
			case 3:
				if inLanguage {
					cur = getCluster(m[2])
				}
			}
			i++
			continue
		}
		if inIntro {
			if strings.TrimSpace(ln) != "" && !strings.HasPrefix(ln, ">") {
				rc.intro = append(rc.intro, ln)
			}
			i++
			continue
		}
		if !inLanguage || strings.TrimSpace(ln) == "" {
			i++
			continue
		}
		if cur == nil {
			cur = getCluster("General")
		}
		m := reTermLine.FindStringSubmatch(ln)
		if m == nil {
			if len(cur.terms) == 0 {
				cur.intro = append(cur.intro, ln)
			}
			i++
			continue
		}
		t := rawTerm{name: strings.TrimSpace(m[1]), line: i + 1}
		i++
		var def []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !reH.MatchString(lines[i]) && !reTermLine.MatchString(lines[i]) {
			if a := reAvoid.FindStringSubmatch(lines[i]); a != nil {
				t.avoid = a[1]
			} else {
				def = append(def, strings.TrimSpace(lines[i]))
			}
			i++
		}
		t.def = strings.Join(def, " ")
		cur.terms = append(cur.terms, t)
	}
	return rc, nil
}

// ---- context map ----------------------------------------------------------

// contextLink is one context the map's Contexts section links: "- [Name](path/to/GLOSSARY.md)".
type contextLink struct {
	name string
	rel  string // slash-separated, relative to the repository root
}

var reContextLink = regexp.MustCompile(`^[-*] \[([^\]]+)\]\(([^)#\s]+)(?:#[^)]*)?\)`)

// parseMapContexts reads the unindented bullets under "## Contexts" that open with a link to a glossary.
func parseMapContexts(lines []string) []contextLink {
	var out []contextLink
	seen := map[string]bool{}
	in := false
	for _, ln := range lines {
		if m := reH.FindStringSubmatch(ln); m != nil && len(m[1]) <= 2 {
			in = len(m[1]) == 2 && strings.EqualFold(m[2], "Contexts")
			continue
		}
		if m := reContextLink.FindStringSubmatch(ln); in && m != nil {
			rel := path.Clean(m[2])
			if path.Base(rel) == glossaryFile && !seen[rel] {
				seen[rel] = true
				out = append(out, contextLink{name: m[1], rel: rel})
			}
		}
	}
	return out
}

type rawRelationship struct {
	label, text string
}

var reRelationship = regexp.MustCompile(`^\*\*([^*]+)\*\*:\s*(.*)$`)

// parseMapRelationships reads the unindented bullets under "## Relationships", joining their indented continuation
// lines. A bullet without a bold label keeps its whole text as the statement.
func parseMapRelationships(lines []string) []rawRelationship {
	var out []rawRelationship
	in := false
	for i := 0; i < len(lines); i++ {
		if m := reH.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= 2 {
			in = len(m[1]) == 2 && strings.EqualFold(strings.TrimSpace(m[2]), "Relationships")
			continue
		}
		m := reBullet.FindStringSubmatch(lines[i])
		if !in || m == nil || m[1] != "" {
			continue
		}
		text := m[2]
		for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") && strings.TrimSpace(lines[i+1]) != "" {
			i++
			text += " " + strings.TrimSpace(lines[i])
		}
		if l := reRelationship.FindStringSubmatch(text); l != nil {
			out = append(out, rawRelationship{label: strings.TrimSpace(l[1]), text: l[2]})
		} else {
			out = append(out, rawRelationship{text: text})
		}
	}
	return out
}

var reLabelSep = regexp.MustCompile(`\s*(?:<->|->|<-|[→↔←⇄,&]|\band\b)\s*`)

// labelNames splits a relationship label such as "Ordering → Billing" into the names it joins.
func labelNames(label string) []string {
	var out []string
	for _, n := range reLabelSep.Split(label, -1) {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}
