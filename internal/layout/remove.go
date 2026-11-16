package layout

import (
	"fmt"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Remove deletes the Term named name from text, the Glossary of the Context with key, and returns
// the new text; References to it elsewhere stay as written. model is the Domain Model as read. Refused when name is
// empty or when model has no such Context or Term; names are matched regardless of case.
func Remove(text string, model *glossary.Model, key, name string) (string, error) {
	term, err := existing(model, key, name)
	if err != nil {
		return "", err
	}
	original := strings.Split(text, "\n")
	lines := withoutCode(original)
	at := termLine(lines, term.Name)
	if at < 0 {
		return "", fmt.Errorf("there is no Term %s in this text", term.Name)
	}
	return strings.Join(cut(original, at, termEnd(lines, at)), "\n"), nil
}
