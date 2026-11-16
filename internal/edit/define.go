// Package edit changes a Glossary's text: the Agent writes plain text and this package lays the Markdown out as the
// glossary format and docs/server.md describe. It is pure: text in, text out, no files and no git.
package edit

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Term is what a define asks for: the Term's name, its definition in plain text, its Avoided Words, the Cluster it
// goes under and the key of the Context whose Glossary the text is.
type Term struct {
	Context    string
	Name       string
	Definition string
	Avoid      []string
	Cluster    string
}

var (
	reHeading  = regexp.MustCompile(`^(#{1,6}) (.*)$`)
	reTermLine = regexp.MustCompile(`^\*\*([^*]+)\*\*:\s*$`)
)

// Define adds t to text, the Glossary of t's Context, and returns the new text. model is the Domain Model as read.
// Refused when t's name or definition is empty, when model has no Context t names, or when that Context already has
// a Term of that name, regardless of case.
func Define(text string, model *glossary.Model, t Term) (string, error) {
	t.Name, t.Cluster = strings.TrimSpace(t.Name), strings.TrimSpace(t.Cluster)
	t.Definition = strings.Join(strings.Fields(t.Definition), " ")
	if t.Name == "" {
		return "", errors.New("the Term's name is empty")
	}
	if t.Definition == "" {
		return "", fmt.Errorf("the definition of %s is empty", t.Name)
	}
	if !slices.ContainsFunc(model.Contexts, func(c glossary.Context) bool { return c.Key == t.Context }) {
		return "", fmt.Errorf("no Context %s", t.Context)
	}
	for _, x := range model.Terms {
		if x.Context == t.Context && strings.EqualFold(x.Name, t.Name) {
			return "", fmt.Errorf("the Term %s already exists in %s", x.Name, model.ContextName(t.Context))
		}
	}
	entry := []string{"**" + t.Name + "**:", newReferences(model, t.Name).bold(t.Definition)}
	if avoid := avoidLine(t.Avoid); avoid != "" {
		entry = append(entry, avoid)
	}
	return place(strings.Split(text, "\n"), t.Cluster, entry), nil
}

// place puts entry, a Term's lines, into original at the end of cluster, creating cluster after the last Term and
// before the next section when no heading names it, or after the last Term when cluster is empty; it returns the text.
func place(original []string, cluster string, entry []string) string {
	lines := withoutCode(original)
	last := lastTerm(lines)
	at := endOfLanguage(lines)
	if last >= 0 {
		at = entryEnd(lines, last)
	}
	if cluster != "" {
		if end, ok := clusterEnd(lines, cluster); ok {
			at = end
		} else {
			at = nextSection(lines, at)
			entry = append([]string{strings.Repeat("#", clusterLevel(lines, last)) + " " + cluster, ""}, entry...)
		}
	}
	return insert(original, at, entry)
}

// code stands in for a line of fenced code: not blank, and neither a heading nor a Term's name line.
const code = "\x00"

// withoutCode is lines with every line of fenced code, fences included, replaced by code, so that only the Glossary's
// own structure is scanned. A fence opens on three or more backticks or tildes and closes on a line holding only its
// character, at least as many times.
func withoutCode(lines []string) []string {
	out := slices.Clone(lines)
	open := ""
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case open != "":
			if strings.Trim(t, open[:1]) == "" && len(t) >= len(open) {
				open = ""
			}
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			open = t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
		default:
			continue
		}
		out[i] = code
	}
	return out
}

// lastTerm is the index of the name line of the last Term entry of lines.
func lastTerm(lines []string) int {
	last := -1
	for i, ln := range lines {
		if reTermLine.MatchString(ln) {
			last = i
		}
	}
	return last
}

// endOfLanguage is where the first Term of a Glossary without Terms goes: the end of its Language section, or the end
// of lines without one.
func endOfLanguage(lines []string) int {
	for i, ln := range lines {
		if m := reHeading.FindStringSubmatch(ln); m != nil && len(m[1]) > 1 && strings.EqualFold(strings.TrimSpace(m[2]), "Language") {
			return nextSection(lines, i+1)
		}
	}
	return len(lines)
}

// nextSection is the index of the first "#" or "##" heading at or after i, or the end of lines.
func nextSection(lines []string, i int) int {
	for ; i < len(lines); i++ {
		if m := reHeading.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= 2 {
			break
		}
	}
	return i
}

// clusterLevel is the heading level a new Cluster takes: that of the Cluster heading above line i, or one below the
// Language heading when the Term at i sits directly under it.
func clusterLevel(lines []string, i int) int {
	for ; i >= 0; i-- {
		if m := reHeading.FindStringSubmatch(lines[i]); m != nil && len(m[1]) > 1 {
			if strings.EqualFold(strings.TrimSpace(m[2]), "Language") {
				return len(m[1]) + 1
			}
			return len(m[1])
		}
	}
	return 3
}

// clusterEnd is the index of the next heading after the heading naming cluster, or the end of lines; false when no
// heading names it.
func clusterEnd(lines []string, cluster string) (int, bool) {
	for i, ln := range lines {
		if m := reHeading.FindStringSubmatch(ln); m != nil && len(m[1]) > 1 && strings.EqualFold(strings.TrimSpace(m[2]), cluster) {
			for i++; i < len(lines) && !reHeading.MatchString(lines[i]); i++ {
			}
			return i, true
		}
	}
	return 0, false
}

// entryEnd is the index of the line after the Term entry whose name line is lines[i]: the next blank line, heading
// or name line.
func entryEnd(lines []string, i int) int {
	for i++; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" || reHeading.MatchString(lines[i]) || reTermLine.MatchString(lines[i]) {
			break
		}
	}
	return i
}

// insert puts block into lines right after the last line before at that is not blank, after one blank line, and
// returns the text. The blank lines that followed stay after it; a line that is not blank gets one blank line between.
func insert(lines []string, at int, block []string) string {
	for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	block = append([]string{""}, block...)
	if at < len(lines) && strings.TrimSpace(lines[at]) != "" {
		block = append(block, "")
	}
	return strings.Join(slices.Insert(lines, at, block...), "\n")
}
