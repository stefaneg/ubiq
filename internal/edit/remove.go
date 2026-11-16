package edit

import (
	"fmt"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Remove deletes the entry of the Term named name from text, the Glossary of the Context with key context, and returns
// the new text; References to it elsewhere stay as written. model is the Domain Model as read. Refused when name is
// empty or when model has no such Context or Term; names are matched regardless of case.
func Remove(text string, model *glossary.Model, context, name string) (string, error) {
	term, err := existing(model, context, name)
	if err != nil {
		return "", err
	}
	original := strings.Split(text, "\n")
	lines := withoutCode(original)
	at := termLine(lines, term.Name)
	if at < 0 {
		return "", fmt.Errorf("there is no Term %s in this text", term.Name)
	}
	return strings.Join(cut(original, at, entryEnd(lines, at)), "\n"), nil
}
