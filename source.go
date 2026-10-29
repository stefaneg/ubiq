package main

import (
	"path/filepath"
)

// glossaryFile is one GLOSSARY.md the model is built from, keyed by context.
type glossaryFile struct {
	key, rel string
}

// glossaryFiles lists the three glossaries in resolution-order precedence after the owning context.
var glossaryFiles = []glossaryFile{
	{key: "common", rel: "GLOSSARY.md"},
	{key: "workspace", rel: "internal/workspace/GLOSSARY.md"},
	{key: "stacks", rel: "internal/stacks/GLOSSARY.md"},
}

// LoadModel parses the glossaries, the context map and the ADRs found under root.
func LoadModel(root string) (*Model, error) {
	var contexts []*rawContext
	for _, g := range glossaryFiles {
		rc, err := parseContext(g.key, g.rel, filepath.Join(root, filepath.FromSlash(g.rel)))
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, rc)
	}
	mapEntries, err := parseMap(filepath.Join(root, "GLOSSARY-MAP.md"))
	if err != nil {
		return nil, err
	}
	adrs, err := parseADRs(root)
	if err != nil {
		return nil, err
	}
	return build(contexts, mapEntries, adrs), nil
}
