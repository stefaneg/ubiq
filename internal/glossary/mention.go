package glossary

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// mentions finds capitalised mentions of terms in plain text: a term name that starts with a capital letter, written
// exactly as defined or as one of its plurals. See docs/adr/0001-references-are-bold-or-capitalised-mentions.md.
type mentions struct {
	forms    map[string]string // written form -> term name
	maxWords int
}

func newMentions(terms map[string]*Term) *mentions {
	m := &mentions{forms: map[string]string{}}
	for _, t := range terms {
		if r, _ := utf8.DecodeRuneInString(t.Name); !unicode.IsUpper(r) {
			continue
		}
		for _, f := range writtenForms(t.Name) {
			m.forms[f] = t.Name
			if n := len(reWord.FindAllStringIndex(f, -1)); n > m.maxWords {
				m.maxWords = n
			}
		}
	}
	return m
}

// writtenForms lists a name and the plurals a definition may write it in.
func writtenForms(name string) []string {
	out := []string{name, name + "s", name + "es"}
	if strings.HasSuffix(name, "y") {
		out = append(out, name[:len(name)-1]+"ies")
	}
	return append(out, HeadPlurals(name)...)
}

var (
	reWord      = regexp.MustCompile(`[\p{L}\p{N}_]+(?:-[\p{L}\p{N}_]+)*`)
	reProtected = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|\\[[^\\]]+\\]\\([^)]*\\)")
)

// bold wraps each capitalised mention in ** so that the resolver links it, longest name first. Code spans, bold
// phrases and links are left alone, and so are mentions of self, the name of the term being defined.
func (m *mentions) bold(text, self string) string {
	var out strings.Builder
	last := 0
	for _, p := range reProtected.FindAllStringIndex(text, -1) {
		out.WriteString(m.boldPlain(text[last:p[0]], self))
		out.WriteString(text[p[0]:p[1]])
		last = p[1]
	}
	out.WriteString(m.boldPlain(text[last:], self))
	return out.String()
}

func (m *mentions) boldPlain(text, self string) string {
	words := reWord.FindAllStringIndex(text, -1)
	var out strings.Builder
	last := 0
	for i := 0; i < len(words); i++ {
		for k := min(m.maxWords, len(words)-i); k > 0; k-- {
			start, end := words[i][0], words[i+k-1][1]
			name, ok := m.forms[text[start:end]]
			if !ok || name == self || !singleSpaced(text[start:end]) {
				continue
			}
			out.WriteString(text[last:start] + "**" + text[start:end] + "**")
			last, i = end, i+k-1
			break
		}
	}
	out.WriteString(text[last:])
	return out.String()
}

// singleSpaced reports whether the words of a phrase are separated by single spaces, as a term name's are.
func singleSpaced(phrase string) bool {
	return !strings.Contains(phrase, "  ") && !strings.ContainsAny(phrase, "\t\n")
}
