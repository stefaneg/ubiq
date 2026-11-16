package edit

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Revision is what a revise asks for: the Term's name and Context, its whole new definition in plain text and its
// whole new list of Avoided Words, the Cluster to move it to, if any, and its new name, if it is renamed.
type Revision struct {
	Context    string
	Name       string
	NewName    string
	Definition string
	Avoid      []string
	Cluster    string
}

// Revise replaces the entry of the Term r names in text, the Glossary of r's Context, and returns the new text.
// model is the Domain Model as read.
// Refused when r's name or definition is empty, when model has no Context r names or no Term of r's name in it, or
// when a rename names another Term of that Context; names are matched regardless of case.
func Revise(text string, model *glossary.Model, r Revision) (string, error) {
	term, err := existing(model, r.Context, r.Name)
	if err != nil {
		return "", err
	}
	definition := strings.Join(strings.Fields(r.Definition), " ")
	if definition == "" {
		return "", fmt.Errorf("the definition of %s is empty", term.Name)
	}
	name := term.Name
	if r.NewName = strings.TrimSpace(r.NewName); r.NewName != "" {
		if other := termNamed(model, r.Context, r.NewName); other != nil && other != term {
			return "", fmt.Errorf("the Term %s already exists in %s", other.Name, model.ContextName(r.Context))
		}
		name = r.NewName
	}
	if name != term.Name {
		text = Rename(text, model, r.Context, r.Context, term.Name, name)
	}
	original := strings.Split(text, "\n")
	lines := withoutCode(original)
	at := termLine(lines, term.Name)
	if at < 0 {
		return "", fmt.Errorf("there is no Term %s in this text", term.Name)
	}
	end := entryEnd(lines, at)
	entry := []string{"**" + name + "**:", newReferences(model, term.Name).bold(definition)}
	if avoid := avoidLine(r.Avoid); avoid != "" {
		entry = append(entry, avoid)
	}
	cluster := strings.TrimSpace(r.Cluster)
	if cluster == "" || strings.EqualFold(cluster, term.Cluster) {
		return strings.Join(slices.Replace(original, at, end, entry...), "\n"), nil
	}
	return place(cut(original, at, end), cluster, entry), nil
}

// cut is lines without lines[at:end], an entry, and without the blank line before it when a blank line or the end
// follows, so that the text around it reads as before.
func cut(lines []string, at, end int) []string {
	lines = slices.Delete(slices.Clone(lines), at, end)
	if at > 0 && strings.TrimSpace(lines[at-1]) == "" && (at == len(lines) || strings.TrimSpace(lines[at]) == "") {
		lines = slices.Delete(lines, at-1, at)
	}
	return lines
}

// existing is the Term of the Context with key context named name, refused when name is empty or when model has no
// such Context or Term.
func existing(model *glossary.Model, context, name string) (*glossary.Term, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("the Term's name is empty")
	}
	if !slices.ContainsFunc(model.Contexts, func(c glossary.Context) bool { return c.Key == context }) {
		return nil, fmt.Errorf("no Context %s", context)
	}
	t := termNamed(model, context, name)
	if t == nil {
		return nil, fmt.Errorf("there is no Term %s in %s", name, model.ContextName(context))
	}
	return t, nil
}

// termNamed is the Term of the Context with key context named name, regardless of case, or nil.
func termNamed(model *glossary.Model, context, name string) *glossary.Term {
	for _, t := range model.Terms {
		if t.Context == context && strings.EqualFold(t.Name, name) {
			return t
		}
	}
	return nil
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
