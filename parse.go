package main

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

type rawBlock struct { // a bullet or paragraph
	text string
}

type rawCluster struct {
	name     string
	intro    []string
	terms    []rawTerm
	rels     []rawBlock
	ambig    []rawBlock
	dialogue []string
	order    int
}

type rawContext struct {
	key, name, path string
	shared          bool // the root glossary
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

var sectionNames = map[string]string{
	"language":                    "language",
	"relationships":               "relationships",
	"example dialogue":            "dialogue",
	"flagged ambiguities":         "ambiguities",
	"cross-cutting relationships": "relationships",
}

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

// parseContext reads one glossary file. displayPath is the path shown to readers; path is where it is read from.
// The caller sets the key.
func parseContext(displayPath, path string) (*rawContext, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, fmt.Errorf("read glossary %s: %w", displayPath, err)
	}
	rc := &rawContext{path: displayPath, clusters: map[string]*rawCluster{}}
	var h2, h3 string
	section, cluster := "", ""
	var cur *rawCluster
	getCluster := func(name string) *rawCluster {
		if c, ok := rc.clusters[name]; ok {
			return c
		}
		c := &rawCluster{name: name, order: len(rc.clusterOrder)}
		rc.clusters[name] = c
		rc.clusterOrder = append(rc.clusterOrder, name)
		return c
	}
	resolveHeadings := func() {
		// Root file: h2 = cluster, h3 = section. Core file: h2 = section, h3 = cluster.
		section, cluster = "", ""
		if s, ok := sectionNames[strings.ToLower(h2)]; ok {
			section = s
			cluster = h3
			if strings.EqualFold(h3, "Cross-Cutting Relationships") {
				cluster = "Cross-Cutting"
			}
		} else if s, ok := sectionNames[strings.ToLower(h3)]; ok {
			section = s
			cluster = h2
		} else {
			cluster = h2
			if h3 != "" {
				cluster = h3
			}
		}
		if cluster == "" {
			cluster = "General"
		}
		cur = getCluster(cluster)
	}

	i := 0
	for i < len(lines) {
		ln := lines[i]
		if m := reH.FindStringSubmatch(ln); m != nil {
			switch len(m[1]) {
			case 1:
				rc.name = m[2]
			case 2:
				h2, h3 = m[2], ""
				resolveHeadings()
			case 3:
				h3 = m[2]
				resolveHeadings()
			}
			i++
			continue
		}
		if h2 == "" { // context intro
			if strings.TrimSpace(ln) != "" && !strings.HasPrefix(ln, ">") {
				rc.intro = append(rc.intro, ln)
			}
			i++
			continue
		}
		switch section {
		case "language":
			if m := reTermLine.FindStringSubmatch(ln); m != nil {
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
				continue
			}
			if strings.TrimSpace(ln) != "" && len(cur.terms) == 0 {
				cur.intro = append(cur.intro, ln)
			}
		case "relationships", "ambiguities":
			if m := reBullet.FindStringSubmatch(ln); m != nil && m[1] == "" {
				text := m[2]
				i++
				for i < len(lines) && strings.HasPrefix(lines[i], "  ") && strings.TrimSpace(lines[i]) != "" {
					text += " " + strings.TrimSpace(lines[i])
					i++
				}
				if section == "relationships" {
					cur.rels = append(cur.rels, rawBlock{text})
				} else {
					cur.ambig = append(cur.ambig, rawBlock{text})
				}
				continue
			}
		case "dialogue":
			if strings.HasPrefix(ln, ">") {
				cur.dialogue = append(cur.dialogue, strings.TrimSpace(strings.TrimPrefix(ln, ">")))
			}
		default:
			if strings.TrimSpace(ln) != "" && section == "" {
				cur.intro = append(cur.intro, ln)
			}
		}
		i++
	}
	return rc, nil
}

// ---- context map ----------------------------------------------------------

type rawMapEntry struct {
	kind, name string
	body       []string
}

var reAnchorLine = regexp.MustCompile(`^\*\*\[([^\]]+)\]\([^)]*\)\*\*\s*[—-]\s*(.*)$`)

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

func parseMap(lines []string) []rawMapEntry {
	var out []rawMapEntry
	in := false
	kind := ""
	var cur *rawMapEntry
	kinds := map[string]string{
		"deliberate mirrors":          "mirror",
		"true collisions":             "collision",
		"resolved deliberate mirrors": "resolved",
		"common anchors":              "anchor",
	}
	for _, ln := range lines {
		if m := reH.FindStringSubmatch(ln); m != nil {
			switch len(m[1]) {
			case 2:
				in = strings.EqualFold(m[2], "Cross-context vocabulary")
				cur = nil
			case 3:
				if !in {
					continue
				}
				if k, ok := kinds[strings.ToLower(m[2])]; ok {
					kind = k
				} else {
					kind = ""
				}
				cur = nil
			case 4:
				if !in || kind == "" {
					cur = nil
					continue
				}
				out = append(out, rawMapEntry{kind: kind, name: m[2]})
				cur = &out[len(out)-1]
			}
			continue
		}
		if !in || kind == "" {
			continue
		}
		if kind == "anchor" {
			if m := reAnchorLine.FindStringSubmatch(ln); m != nil {
				out = append(out, rawMapEntry{kind: kind, name: m[1], body: []string{m[2]}})
				cur = &out[len(out)-1]
				continue
			}
		}
		if cur != nil {
			cur.body = append(cur.body, ln)
		}
	}
	return out
}
