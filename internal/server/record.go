package server

import (
	"os"
	"path"
	"path/filepath"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/layout"
	"github.com/stefaneg/ubiq/internal/person"
)

// record lands a new ADR in the repository's own ADR directory, never a Context's, in the open Batch, and answers its
// path, the ADR as it now reads, the commit, the Batch and the Findings the Change introduced.
func (o Options) record(p person.Person, in recordInput) (string, error) {
	var file, body string
	l, err := o.land(p, "record_adr", func(*glossary.Model) (change, error) {
		existing, err := os.ReadDir(filepath.Join(o.Root, filepath.FromSlash(glossary.ADRDir)))
		if err != nil && !os.IsNotExist(err) {
			return change{}, err
		}
		var names []string
		for _, e := range existing {
			names = append(names, e.Name())
		}
		adr, err := layout.Record(names, in.Title, in.Text)
		if err != nil {
			return change{}, err
		}
		file, body = path.Join(glossary.ADRDir, adr.Name), adr.Body
		return change{files: map[string]string{file: body}, subject: "Record ADR " + adr.Title, named: file}, nil
	})
	if err != nil {
		return "", err
	}
	return l.answer("Recorded "+file, body), nil
}
