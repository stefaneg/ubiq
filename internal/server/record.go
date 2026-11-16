package server

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/edit"
	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
	"github.com/stefaneg/ubiq/internal/person"
)

// adrDir is where record_adr writes, relative to the root: the repository's own ADRs, never a Context's.
const adrDir = "docs/adr"

// record lands a new ADR in the open Batch and answers its path, the ADR as it now reads, the commit, the Batch and
// the Findings the Change introduced.
func (o Options) record(p person.Person, in recordInput) (string, error) {
	if o.Batches == nil {
		return "", errors.New("This Server has no origin to land Changes in.")
	}
	var file, body string
	var introduced []health.Finding
	landed, err := o.Batches.Change(batch.Identity{Name: p.Name, Email: p.Email}, func() (batch.Edit, error) {
		dir := filepath.Join(o.Root, filepath.FromSlash(adrDir))
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return batch.Edit{}, err
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		adr, err := edit.Record(names, in.Title, in.Text)
		if err != nil {
			return batch.Edit{}, errors.New(sentence(err.Error()))
		}
		before, err := glossary.Load(o.Root)
		if err != nil {
			return batch.Edit{}, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return batch.Edit{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, adr.Name), []byte(adr.Body), 0o644); err != nil {
			return batch.Edit{}, err
		}
		after, err := glossary.Load(o.Root)
		if err != nil {
			return batch.Edit{}, fmt.Errorf("The Domain Model would no longer read after recording %s: %s.", adr.Title, err)
		}
		file, body = path.Join(adrDir, adr.Name), adr.Body
		introduced = newFindings(health.Check(before), health.Check(after))
		return batch.Edit{Subject: "Record ADR " + adr.Title, Paths: []string{file}}, nil
	})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(o.Log, "Change record_adr %s by %s: %s on %s\n", file, label(p), landed.Commit, landed.Batch)
	var b strings.Builder
	fmt.Fprintf(&b, "Recorded %s in commit %s on the Batch %s.\n\n%s", file, landed.Commit, landed.Batch, body)
	if len(introduced) == 0 {
		b.WriteString("\nThe Change introduced no Findings.\n")
	} else {
		b.WriteString("\nFindings the Change introduced:\n")
		health.Write(&b, introduced)
	}
	return b.String(), nil
}
