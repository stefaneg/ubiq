// Package layout lays a Glossary's Markdown out: the Agent writes plain text and this package turns it into the
// glossary format's text, as docs/server.md describes. It is pure: text in, text out, no files and no git.
package layout

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Term is a Term as an Agent writes it: its name, its definition in plain text, its Avoided Words, the Cluster it goes
// under and the key of the Context whose Glossary the text is.
type Term struct {
	Context    string
	Name       string
	Definition string
	Avoid      []string
	Cluster    string
}

// Valid refuses a Term whose name or definition is empty, before anything reads a Glossary.
func (t Term) Valid() error {
	if err := ValidName(t.Name); err != nil {
		return err
	}
	if strings.TrimSpace(t.Definition) == "" {
		return fmt.Errorf("the definition of %s is empty", strings.TrimSpace(t.Name))
	}
	return nil
}

// ValidName refuses an empty Term name, before anything reads a Glossary.
func ValidName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("the Term's name is empty")
	}
	return nil
}

var (
	reHeading  = regexp.MustCompile(`^(#{1,6}) (.*)$`)
	reTermLine = regexp.MustCompile(`^\*\*([^*]+)\*\*:\s*$`)
)

// Define adds t to text, the Glossary of t's Context, and returns the new text. model is the Domain Model as read.
// Refused when t is not valid, when model has no Context t names, or when that Context already has a Term of that
// name, regardless of case, spacing or punctuation.
func Define(text string, model *glossary.Model, t Term) (string, error) {
	if err := t.Valid(); err != nil {
		return "", err
	}
	t.Name, t.Cluster = strings.TrimSpace(t.Name), strings.TrimSpace(t.Cluster)
	if !model.HasContext(t.Context) {
		return "", fmt.Errorf("no Context %s", t.Context)
	}
	if x := model.TermClashing(t.Context, t.Name); x != nil {
		return "", clash(model, x, t.Name)
	}
	return place(strings.Split(text, "\n"), t.Cluster, termLines(model, t.Name, t)), nil
}

// clash is why name is refused for the Term x of model it clashes with.
func clash(model *glossary.Model, x *glossary.Term, name string) error {
	if strings.EqualFold(x.Name, name) {
		return fmt.Errorf("the Term %s already exists in %s", x.Name, model.ContextName(x.Context))
	}
	return fmt.Errorf("the name %s differs from the Term %s of %s only in case, spacing or punctuation",
		name, x.Name, model.ContextName(x.Context))
}

// termLines is the lines t is written as: its name line, its definition with every mention of a Term of model other
// than self bolded, and its Avoid line when it has Avoided Words.
func termLines(model *glossary.Model, self string, t Term) []string {
	definition := strings.Join(strings.Fields(t.Definition), " ")
	lines := []string{"**" + t.Name + "**:", newReferences(model, self).bold(definition)}
	if avoid := avoidLine(t.Avoid); avoid != "" {
		lines = append(lines, avoid)
	}
	return lines
}

// place puts block, a Term's lines, into original at the end of cluster, creating cluster after the last Term and
// before the next section when no heading names it, or after the last Term when cluster is empty; it returns the text.
func place(original []string, cluster string, block []string) string {
	lines := withoutCode(original)
	last := lastTerm(lines)
	at := endOfLanguage(lines)
	if last >= 0 {
		at = termEnd(lines, last)
	}
	if cluster != "" {
		if end, ok := clusterEnd(lines, cluster); ok {
			at = end
		} else {
			at = nextSection(lines, at)
			block = append([]string{strings.Repeat("#", clusterLevel(lines, last)) + " " + cluster, ""}, block...)
		}
	}
	return insert(original, at, block)
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

// lastTerm is the index of the name line of the last Term of lines.
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

// termEnd is the index of the line after the Term whose name line is lines[i]: the next blank line, heading
// or name line.
func termEnd(lines []string, i int) int {
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
