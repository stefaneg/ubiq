package server

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
	"github.com/stefaneg/ubiq/internal/layout"
	"github.com/stefaneg/ubiq/internal/person"
)

// contextMap is the Context Map's path relative to the root, the one file beside the Glossaries a rename rewrites.
const contextMap = "GLOSSARY-MAP.md"

// revise lands in in the open Batch: the Term's definition and Avoided Words replaced, moved to the Cluster named and
// renamed, with every Reference to it rewritten. It answers the Term as it now reads, the commit, the Batch and the
// Findings the Change introduced.
func (o Options) revise(p person.Person, in reviseInput) (string, error) {
	r := layout.Term{Name: in.Name, Definition: in.Definition, Avoid: in.Avoid, Cluster: in.Cluster}
	var old, name string
	c, landed, introduced, err := o.changeTerms(p, in.Context, func(before *glossary.Model, c glossary.Context) (map[string]string, string, error) {
		r.Context = c.Key
		text, err := o.read(c.Path)
		if err != nil {
			return nil, "", err
		}
		out, err := layout.Revise(text, before, r, in.NewName)
		if err != nil {
			return nil, "", err
		}
		old = termIn(before, c.Key, in.Name).Name
		name = old
		files := map[string]string{c.Path: out}
		if n := strings.TrimSpace(in.NewName); n != "" && n != old {
			name = n
			if err := o.renameElsewhere(files, before, c, old, n); err != nil {
				return nil, "", err
			}
		}
		return files, "Revise Term " + old, nil
	})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(o.Log, "Change revise_term %s by %s: %s on %s\n", old, label(p), landed.Commit, landed.Batch)
	var b strings.Builder
	if name != old {
		fmt.Fprintf(&b, "Revised %s, now named %s, in commit %s on the Batch %s.\n\n", old, name, landed.Commit, landed.Batch)
	} else {
		fmt.Fprintf(&b, "Revised %s in commit %s on the Batch %s.\n\n", old, landed.Commit, landed.Batch)
	}
	b.WriteString(c.entry(name))
	writeIntroduced(&b, introduced)
	return b.String(), nil
}

// remove lands the removal of the Term in named in the open Batch, and answers the commit, the Batch and the Findings
// the Change introduced, among them each Unresolved Phrase the removal left.
func (o Options) remove(p person.Person, in removeInput) (string, error) {
	var name string
	_, landed, introduced, err := o.changeTerms(p, in.Context, func(before *glossary.Model, c glossary.Context) (map[string]string, string, error) {
		text, err := o.read(c.Path)
		if err != nil {
			return nil, "", err
		}
		out, err := layout.Remove(text, before, c.Key, in.Name)
		if err != nil {
			return nil, "", err
		}
		name = termIn(before, c.Key, in.Name).Name
		return map[string]string{c.Path: out}, "Remove Term " + name, nil
	})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(o.Log, "Change remove_term %s by %s: %s on %s\n", name, label(p), landed.Commit, landed.Batch)
	var b strings.Builder
	fmt.Fprintf(&b, "Removed %s in commit %s on the Batch %s.\n", name, landed.Commit, landed.Batch)
	writeIntroduced(&b, introduced)
	return b.String(), nil
}

// renameElsewhere adds to files every other Glossary and the Context Map with the References to the Term old of c
// renamed to name, where any changed.
func (o Options) renameElsewhere(files map[string]string, before *glossary.Model, c glossary.Context, old, name string) error {
	others := map[string]string{contextMap: ""}
	for _, x := range before.Contexts {
		if x.Key != c.Key {
			others[x.Path] = x.Key
		}
	}
	for path, key := range others {
		text, err := o.read(path)
		if errors.Is(err, fs.ErrNotExist) && path == contextMap {
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

// changed is the Context a Change to the Glossaries was made in and the Domain Model after it.
type changed struct {
	glossary.Context
	after *glossary.Model
}

// entry is the Term named in the Context as lookup now writes it.
func (c changed) entry(named string) string { return termEntry(c.after, c.Key, named) }

// changeTerms lands one Change to the Glossaries in the open Batch for p. run gets the Domain Model as read and the
// Context named, and answers the new text of each file it changes, by path relative to the root, and the commit's
// subject; an error it answers is refused as a sentence. changeTerms answers the Context, where the Change landed and
// the Findings it introduced.
func (o Options) changeTerms(p person.Person, named string,
	run func(before *glossary.Model, c glossary.Context) (map[string]string, string, error),
) (changed, batch.Landed, []health.Finding, error) {
	if o.Batches == nil {
		return changed{}, batch.Landed{}, nil, errors.New("This Server has no origin to land Changes in.")
	}
	var c changed
	var introduced []health.Finding
	landed, err := o.Batches.Change(batch.Identity{Name: p.Name, Email: p.Email}, func() (batch.Commit, error) {
		before, err := glossary.Load(o.Root)
		if err != nil {
			return batch.Commit{}, err
		}
		if c.Context, err = contextNamed(before, named); err != nil {
			return batch.Commit{}, err
		}
		files, subject, err := run(before, c.Context)
		if err != nil {
			return batch.Commit{}, errors.New(sentence(err.Error()))
		}
		var paths []string
		for path, text := range files {
			if err := os.WriteFile(filepath.Join(o.Root, filepath.FromSlash(path)), []byte(text), 0o644); err != nil {
				return batch.Commit{}, err
			}
			paths = append(paths, path)
		}
		slices.Sort(paths)
		if c.after, err = glossary.Load(o.Root); err != nil {
			return batch.Commit{}, fmt.Errorf("The Glossary would no longer read after the Change %s: %s.", subject, err)
		}
		introduced = newFindings(health.Check(before), health.Check(c.after))
		return batch.Commit{Subject: subject, Paths: paths}, nil
	})
	return c, landed, introduced, err
}

// read is the text of the file at path relative to the root.
func (o Options) read(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(path)))
	return string(b), err
}

// termIn is the Term of the Context with key named named, regardless of case and surrounding space, or nil.
func termIn(model *glossary.Model, key, named string) *glossary.Term {
	for _, t := range model.Terms {
		if t.Context == key && strings.EqualFold(t.Name, strings.TrimSpace(named)) {
			return t
		}
	}
	return nil
}

// writeIntroduced writes the Findings a Change introduced, or that it introduced none.
func writeIntroduced(b *strings.Builder, introduced []health.Finding) {
	if len(introduced) == 0 {
		b.WriteString("\nThe Change introduced no Findings.\n")
		return
	}
	b.WriteString("\nFindings the Change introduced:\n")
	health.Write(b, introduced)
}
