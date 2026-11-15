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
	statements      []rawStatement
	headings        []rawHeading
}

// rawStatement is one block of a document outside its Term entries, with the headings above it.
type rawStatement struct {
	label   string // the headings above it below the title, as "A › B"; the title above them all
	heading string // the nearest heading; empty under the title
	line    int
	text    string
}

// rawHeading is one heading below a document's title, with its label.
type rawHeading struct {
	label, text string
}

var (
	reH        = regexp.MustCompile(`^(#{1,6}) (.*)$`)
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

// heading is one "##" to "######" heading of a document.
type heading struct {
	level int
	text  string
	line  int
}

// parseContext reads one glossary file: a "#" heading naming the context, an intro before the first "##" heading,
// and Term entries under any heading. A Term's Cluster is the nearest heading above it, skipping one named Language,
// or General without one; the text between that heading and the Cluster's first Term is the Cluster's intro. Fenced
// code is skipped. displayPath is the path shown to readers; path is where it is read from. The caller sets the key.
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
	var stack []heading
	var pending []string // the lines since the nearest heading, which become a Cluster's intro if a Term follows
	var code fences
	for i := 0; i < len(lines); {
		ln := lines[i]
		if code.skip(ln) {
			i++
			continue
		}
		if m := reH.FindStringSubmatch(ln); m != nil {
			level, name := len(m[1]), strings.TrimSpace(m[2])
			switch {
			case level == 1 && rc.name == "":
				rc.name = name
			case level > 1:
				stack = push(stack, heading{level: level, text: name, line: i + 1})
				if level == 2 || !isLanguage(name) {
					pending = nil
				}
			}
			i++
			continue
		}
		if m := reTermLine.FindStringSubmatch(ln); m != nil {
			var t rawTerm
			t, i = readTerm(lines, i, m[1])
			c := getCluster(clusterName(stack))
			if len(c.terms) == 0 && len(c.intro) == 0 {
				c.intro = pending
			}
			pending = nil
			c.terms = append(c.terms, t)
			continue
		}
		if strings.TrimSpace(ln) != "" {
			switch {
			case len(stack) > 0:
				pending = append(pending, ln)
			case !strings.HasPrefix(ln, ">"):
				rc.intro = append(rc.intro, ln)
			}
		}
		i++
	}
	rc.statements, rc.headings = parseStatements(lines)
	return rc, nil
}

var (
	reNumbered = regexp.MustCompile(`^(\s*)\d+\. (.*)$`)
	reTableSep = regexp.MustCompile(`^[\s|:-]+$`)
)

// parseStatements reads every block of a document outside its Term entries and fenced code: a paragraph, a bullet or
// numbered item with its indented continuation lines, a table row other than a separator, or a blockquote. It also
// returns each heading below the title, with its label.
func parseStatements(lines []string) ([]rawStatement, []rawHeading) {
	var out []rawStatement
	var headings []rawHeading
	var stack []heading
	title := ""
	label := func() string {
		if len(stack) == 0 {
			return title
		}
		names := make([]string, len(stack))
		for i, h := range stack {
			names[i] = h.text
		}
		return strings.Join(names, " › ")
	}
	add := func(line int, text string) {
		s := rawStatement{label: label(), line: line, text: text}
		if len(stack) > 0 {
			s.heading = stack[len(stack)-1].text
		}
		out = append(out, s)
	}
	var para []string
	paraLine, quote := 0, false
	var code fences
	flush := func() {
		if len(para) > 0 {
			add(paraLine, strings.Join(para, " "))
		}
		para, quote = nil, false
	}
	extend := func(i int, text string, isQuote bool) {
		if quote != isQuote {
			flush()
		}
		if len(para) == 0 {
			paraLine = i + 1
		}
		para, quote = append(para, text), isQuote
	}
	for i := 0; i < len(lines); {
		ln := lines[i]
		trimmed := strings.TrimSpace(ln)
		indent, item, isItem := listItem(ln)
		switch {
		case code.skip(ln):
			flush()
			i++
		case reH.MatchString(ln):
			flush()
			m := reH.FindStringSubmatch(ln)
			if name := strings.TrimSpace(m[2]); len(m[1]) > 1 {
				stack = push(stack, heading{level: len(m[1]), text: name, line: i + 1})
				headings = append(headings, rawHeading{label: label(), text: name})
			} else if title == "" {
				title = name
			}
			i++
		case reTermLine.MatchString(ln):
			flush()
			_, i = readTerm(lines, i, "")
		case trimmed == "":
			flush()
			i++
		case strings.HasPrefix(trimmed, "|"):
			flush()
			if !reTableSep.MatchString(trimmed) {
				add(i+1, strings.TrimSpace(strings.Trim(trimmed, "|")))
			}
			i++
		case isItem:
			flush()
			start := i
			for i++; i < len(lines) && continues(lines[i], indent); i++ {
				item += " " + strings.TrimSpace(lines[i])
			}
			add(start+1, item)
		case strings.HasPrefix(trimmed, ">"):
			extend(i, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")), true)
			i++
		default:
			extend(i, trimmed, false)
			i++
		}
	}
	flush()
	return out, headings
}

// listItem splits a bullet or numbered item into its indent and text.
func listItem(line string) (indent, text string, ok bool) {
	if m := reBullet.FindStringSubmatch(line); m != nil {
		return m[1], m[2], true
	}
	if m := reNumbered.FindStringSubmatch(line); m != nil {
		return m[1], m[2], true
	}
	return "", "", false
}

// continues reports whether line continues the text of a list item indented by indent: it is indented further and
// starts neither a table nor a fence, which the main loop reads on their own.
func continues(line, indent string) bool {
	_, opens := fenceMarker(line)
	return strings.HasPrefix(line, indent+"  ") && !strings.HasPrefix(strings.TrimSpace(line), "|") && !opens
}

// readTerm reads the Term entry whose "**Name**:" line is lines[i], up to the next blank line, heading or entry, and
// returns it with the index of the line after it.
func readTerm(lines []string, i int, name string) (rawTerm, int) {
	t := rawTerm{name: strings.TrimSpace(name), line: i + 1}
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
	return t, i
}

// push adds h to the stack of open headings, closing those of its level or deeper.
func push(stack []heading, h heading) []heading {
	for len(stack) > 0 && stack[len(stack)-1].level >= h.level {
		stack = stack[:len(stack)-1]
	}
	return append(stack, h)
}

// clusterName is the nearest open heading not named Language, or General.
func clusterName(stack []heading) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if !isLanguage(stack[i].text) {
			return stack[i].text
		}
	}
	return "General"
}

func isLanguage(heading string) bool { return strings.EqualFold(heading, "Language") }

// fences tracks fenced code across the lines of a document. The zero value is outside any fence.
type fences struct {
	open string // the marker of the open fence, such as "```" or "~~~~", or "" outside one
}

// skip reports whether line is fenced code or one of its fence lines, and moves past it. A fence closes only on a
// line holding nothing but its own character, repeated at least as often as in the marker that opened it.
func (f *fences) skip(line string) bool {
	if f.open != "" {
		t := strings.TrimSpace(line)
		if strings.Trim(t, f.open[:1]) == "" && len(t) >= len(f.open) {
			f.open = ""
		}
		return true
	}
	if marker, ok := fenceMarker(line); ok {
		f.open = marker
		return true
	}
	return false
}

// fenceMarker returns the run of three or more backticks or tildes that opens a fence on line. A backtick fence's
// info string holds no backtick, so a line such as "```code``` and more" is inline code, not a fence.
func fenceMarker(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return "", false
	}
	marker := t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
	if len(marker) < 3 || (t[0] == '`' && strings.Contains(t[len(marker):], "`")) {
		return "", false
	}
	return marker, true
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
