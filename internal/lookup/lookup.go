// Package lookup answers what a word means in a Domain Model: the Terms it names, the Terms that avoid it, and,
// failing both, the Terms that mention it.
package lookup

import (
	"sort"
	"strings"
	"unicode"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Lookup indexes a model's Terms by name.
type Lookup struct {
	model   *glossary.Model
	byName  map[string][]string // normalised term name -> term IDs
	avoided map[string][]string // normalised Avoided Word -> IDs of the terms that avoid it
	words   []termWords
	inbound map[string][]string // term ID -> IDs of the terms whose definitions refer to it
}

// termWords holds the word forms of one Term: of its name, and of its name, Avoid line and definition together.
type termWords struct {
	term      *glossary.Term
	name, all map[string]bool
}

// Result is the outcome of looking up one word.
type Result struct {
	Word    string
	Named   []string // IDs of the Terms the word names
	Avoided []string // IDs of the Terms that avoid the word, to use instead
	// Mentions is the IDs of the Terms whose name, Avoid line or definition has every word of the word, when it
	// names no Term and no Term avoids it. Terms whose name has more of the words come first.
	Mentions []string
	lookup   *Lookup
}

// New indexes the model's Terms by name, by Avoided Word, and by the words of their definitions.
func New(model *glossary.Model) *Lookup {
	l := &Lookup{model: model, byName: map[string][]string{}, avoided: map[string][]string{}, inbound: map[string][]string{}}
	for _, t := range model.Terms {
		k := normalise(t.Name)
		l.byName[k] = append(l.byName[k], t.ID)
		for _, w := range t.Redirects {
			k := normalise(w)
			l.avoided[k] = append(l.avoided[k], t.ID)
		}
		l.words = append(l.words, termWords{
			term: t,
			name: wordForms(t.Name),
			all:  wordForms(t.Name + " " + t.Avoid + " " + glossary.PlainText(t.Definition)),
		})
		for _, ref := range t.Refs {
			l.inbound[ref] = append(l.inbound[ref], t.ID)
		}
	}
	for _, ids := range l.inbound {
		l.byTermName(ids)
	}
	sort.Slice(l.words, func(i, j int) bool { return l.termBefore(l.words[i].term, l.words[j].term) })
	for _, index := range []map[string][]string{l.byName, l.avoided} {
		for _, ids := range index {
			sort.Strings(ids)
		}
	}
	return l
}

// Find looks up a word written as prose, such as "stack images", or as an identifier, such as StackImage.
func (l *Lookup) Find(word string) *Result {
	r := &Result{Word: word, lookup: l}
	forms := glossary.Singulars(normalise(word))
	r.Named = first(l.byName, forms)
	r.Avoided = first(l.avoided, forms)
	if r.Named == nil && r.Avoided == nil {
		r.Mentions = l.mentions(splitWords(word))
	}
	return r
}

func (l *Lookup) mentions(words []string) []string {
	type hit struct {
		term *glossary.Term
		rank int // words the name has
	}
	var hits []hit
	for _, tw := range l.words {
		h := hit{term: tw.term}
		all := len(words) > 0
		for _, w := range words {
			all = all && hasForm(tw.all, w)
			if hasForm(tw.name, w) {
				h.rank++
			}
		}
		if all {
			hits = append(hits, h)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].rank > hits[j].rank })
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.term.ID
	}
	return ids
}

// byTermName sorts term IDs by term name, then by ID.
func (l *Lookup) byTermName(ids []string) {
	sort.Slice(ids, func(i, j int) bool { return l.termBefore(l.model.Terms[ids[i]], l.model.Terms[ids[j]]) })
}

func (l *Lookup) termBefore(a, b *glossary.Term) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.ID < b.ID
}

// wordForms is the set of the words of text and their singulars.
func wordForms(text string) map[string]bool {
	forms := map[string]bool{}
	for _, w := range splitWords(text) {
		for _, f := range glossary.Singulars(w) {
			forms[f] = true
		}
	}
	return forms
}

// splitWords is the normalised words of text, without punctuation.
func splitWords(text string) []string {
	return strings.FieldsFunc(normalise(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// hasForm reports whether forms holds word or one of its singulars.
func hasForm(forms map[string]bool, word string) bool {
	for _, f := range glossary.Singulars(word) {
		if forms[f] {
			return true
		}
	}
	return false
}

// first is the entry of the first form the index holds.
func first(index map[string][]string, forms []string) []string {
	for _, f := range forms {
		if ids, ok := index[f]; ok {
			return ids
		}
	}
	return nil
}

// normalise lowercases a word and splits it into space-separated words at spaces, hyphens, underscores and the
// lowercase-to-uppercase steps of an identifier, so StackImage, stack_image and "Stack Image" agree.
func normalise(s string) string {
	var b strings.Builder
	prev := ' '
	for _, r := range s {
		switch {
		case r == '-' || r == '_' || unicode.IsSpace(r):
			r = ' '
		case unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)):
			b.WriteRune(' ')
		}
		if r == ' ' && prev == ' ' {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
		prev = r
	}
	return strings.TrimSpace(b.String())
}
