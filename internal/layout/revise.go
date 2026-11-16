package layout

import (
	"fmt"
	"slices"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Revise replaces the Term t names in text, the Glossary of t's Context, with t: its whole new definition in plain
// text and its whole new list of Avoided Words, moved to t's Cluster when it names another, and renamed to newName
// when that is not empty. It returns the new text; model is the Domain Model as read.
// Refused when t's name or definition is empty, when model has no Context t names or no Term of t's name in it, or
// when newName names another Term of that Context; names are matched regardless of case.
func Revise(text string, model *glossary.Model, t Term, newName string) (string, error) {
	term, err := existing(model, t.Context, t.Name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(t.Definition) == "" {
		return "", fmt.Errorf("the definition of %s is empty", term.Name)
	}
	t.Name = term.Name
	if newName = strings.TrimSpace(newName); newName != "" {
		if other := model.TermNamed(t.Context, newName); other != nil && other != term {
			return "", fmt.Errorf("the Term %s already exists in %s", other.Name, model.ContextName(t.Context))
		}
		t.Name = newName
	}
	if t.Name != term.Name {
		text = Rename(text, model, t.Context, t.Context, term.Name, t.Name)
	}
	original := strings.Split(text, "\n")
	lines := withoutCode(original)
	at := termLine(lines, term.Name)
	if at < 0 {
		return "", fmt.Errorf("there is no Term %s in this text", term.Name)
	}
	end := termEnd(lines, at)
	block := termLines(model, term.Name, t)
	cluster := strings.TrimSpace(t.Cluster)
	if cluster == "" || strings.EqualFold(cluster, term.Cluster) {
		return strings.Join(slices.Replace(original, at, end, block...), "\n"), nil
	}
	return place(cut(original, at, end), cluster, block), nil
}

// cut is lines without lines[at:end], a Term's lines, and without the blank line before them when a blank line or the
// end follows, so that the text around them reads as before.
func cut(lines []string, at, end int) []string {
	lines = slices.Delete(slices.Clone(lines), at, end)
	if at > 0 && strings.TrimSpace(lines[at-1]) == "" && (at == len(lines) || strings.TrimSpace(lines[at]) == "") {
		lines = slices.Delete(lines, at-1, at)
	}
	return lines
}

// existing is the Term of the Context with key named name, refused when name is empty or when model has no such
// Context or Term.
func existing(model *glossary.Model, key, name string) (*glossary.Term, error) {
	if err := ValidName(name); err != nil {
		return nil, err
	}
	if !model.HasContext(key) {
		return nil, fmt.Errorf("no Context %s", key)
	}
	t := model.TermNamed(key, name)
	if t == nil {
		return nil, fmt.Errorf("there is no Term %s in %s", strings.TrimSpace(name), model.ContextName(key))
	}
	return t, nil
}

// termLine is the index of the name line of the Term named name, regardless of case, or -1.
func termLine(lines []string, name string) int {
	for i, ln := range lines {
		if m := reTermLine.FindStringSubmatch(ln); m != nil && strings.EqualFold(strings.TrimSpace(m[1]), name) {
			return i
		}
	}
	return -1
}

// avoidLine is the Avoid line for words, blanks dropped, or "" when none is left.
func avoidLine(words []string) string {
	var avoid []string
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" {
			avoid = append(avoid, w)
		}
	}
	if len(avoid) == 0 {
		return ""
	}
	return "_Avoid_: " + strings.Join(avoid, ", ")
}
