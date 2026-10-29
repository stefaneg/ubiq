package glossary

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

const (
	mapFile      = "GLOSSARY-MAP.md"
	glossaryFile = "GLOSSARY.md"
)

// Load parses the glossaries, the context map and the ADRs found under root, laid out as the domain-modeling
// skill describes. With a GLOSSARY-MAP.md, the contexts are the glossaries its Contexts section links; without one,
// the root GLOSSARY.md is the only context.
func Load(root string) (*Model, error) {
	links, _, err := contextLinks(root)
	if err != nil {
		return nil, err
	}
	var contexts []*rawContext
	byKey := map[string]*rawContext{}
	for _, l := range links {
		rc, err := parseContext(l.rel, filepath.Join(root, filepath.FromSlash(l.rel)))
		if err != nil {
			return nil, err
		}
		if rc.name == "" {
			rc.name = l.name
		}
		if rc.name == "" {
			rc.name = "Glossary"
		}
		rc.key, rc.shared = slug(rc.name), path.Dir(l.rel) == "."
		if prev, ok := byKey[rc.key]; ok {
			return nil, fmt.Errorf("%s: context key %q is already used by %s", rc.path, rc.key, prev.path)
		}
		byKey[rc.key] = rc
		contexts = append(contexts, rc)
	}
	// the shared context first, then map order
	sort.SliceStable(contexts, func(i, j int) bool { return contexts[i].shared && !contexts[j].shared })
	adrs, err := parseADRs(root, contexts)
	if err != nil {
		return nil, err
	}
	return build(contexts, adrs), nil
}

// contextLinks lists the context glossaries under root and returns the context map's lines, nil without a map.
func contextLinks(root string) ([]contextLink, []string, error) {
	lines, err := readLines(filepath.Join(root, mapFile))
	if errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Stat(filepath.Join(root, glossaryFile)); errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("no %s or %s under %s", mapFile, glossaryFile, root)
		}
		return []contextLink{{rel: glossaryFile}}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read context map: %w", err)
	}
	links := parseMapContexts(lines)
	if len(links) == 0 {
		return nil, nil, fmt.Errorf("%s links no context glossary under ## Contexts", mapFile)
	}
	return links, lines, nil
}
