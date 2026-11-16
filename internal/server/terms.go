package server

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/layout"
	"github.com/stefaneg/ubiq/internal/person"
)

// define lands in as a new Term in the open Batch and answers the Term as it now reads, the commit, the Batch and the
// Findings the Change introduced.
func (o Options) define(p person.Person, in defineInput) (string, error) {
	t := in.term()
	t.Name = strings.TrimSpace(t.Name)
	l, err := o.land(p, "define_term", func(before *glossary.Model) (change, error) {
		c, err := contextNamed(before, in.Context)
		if err != nil {
			return change{}, err
		}
		t.Context = c.Key
		text, err := o.read(c.Path)
		if err != nil {
			return change{}, err
		}
		out, err := layout.Define(text, before, t)
		if err != nil {
			return change{}, err
		}
		return change{files: map[string]string{c.Path: out}, subject: "Define Term " + t.Name, named: t.Name}, nil
	})
	if err != nil {
		return "", err
	}
	return l.answer("Defined "+t.Name, l.lookedUp(t.Context, t.Name)), nil
}

// revise lands in in the open Batch: the Term's definition and Avoided Words replaced, moved to the Cluster named and
// renamed, with every Reference to it rewritten. It answers the Term as it now reads, the commit, the Batch and the
// Findings the Change introduced.
func (o Options) revise(p person.Person, in reviseInput) (string, error) {
	t := in.term()
	var old, name string
	l, err := o.land(p, "revise_term", func(before *glossary.Model) (change, error) {
		c, err := contextNamed(before, in.Context)
		if err != nil {
			return change{}, err
		}
		t.Context = c.Key
		text, err := o.read(c.Path)
		if err != nil {
			return change{}, err
		}
		out, err := layout.Revise(text, before, t, in.NewName)
		if err != nil {
			return change{}, err
		}
		old = before.TermNamed(c.Key, t.Name).Name
		name = old
		files := map[string]string{c.Path: out}
		if n := strings.TrimSpace(in.NewName); n != "" && n != old {
			name = n
			if err := o.renameElsewhere(files, before, c, old, n); err != nil {
				return change{}, err
			}
		}
		return change{files: files, subject: "Revise Term " + old, named: old}, nil
	})
	if err != nil {
		return "", err
	}
	did := "Revised " + old
	if name != old {
		did += ", now named " + name + ","
	}
	return l.answer(did, l.lookedUp(t.Context, name)), nil
}

// remove lands the removal of the Term in names in the open Batch, and answers the commit, the Batch and the Findings
// the Change introduced, among them each Unresolved Phrase the removal left.
func (o Options) remove(p person.Person, in removeInput) (string, error) {
	var name string
	l, err := o.land(p, "remove_term", func(before *glossary.Model) (change, error) {
		c, err := contextNamed(before, in.Context)
		if err != nil {
			return change{}, err
		}
		text, err := o.read(c.Path)
		if err != nil {
			return change{}, err
		}
		out, err := layout.Remove(text, before, c.Key, in.Name)
		if err != nil {
			return change{}, err
		}
		name = before.TermNamed(c.Key, in.Name).Name
		return change{files: map[string]string{c.Path: out}, subject: "Remove Term " + name, named: name}, nil
	})
	if err != nil {
		return "", err
	}
	return l.answer("Removed "+name, ""), nil
}

// renameElsewhere adds to files every other Glossary and the Context Map with the References to the Term old of c
// renamed to name, where any changed.
func (o Options) renameElsewhere(files map[string]string, before *glossary.Model, c glossary.Context, old, name string) error {
	others := map[string]string{glossary.MapFile: ""}
	for _, x := range before.Contexts {
		if x.Key != c.Key {
			others[x.Path] = x.Key
		}
	}
	for path, key := range others {
		text, err := o.read(path)
		if errors.Is(err, fs.ErrNotExist) && path == glossary.MapFile {
			continue
		}
		if err != nil {
			return err
		}
		if out := layout.Rename(text, before, key, c.Key, old, name); out != text {
			files[path] = out
		}
	}
	return nil
}
