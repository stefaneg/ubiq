package edit

import (
	"regexp"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// references turns mentions of a Domain Model's Terms in plain text into References.
type references struct {
	forms    map[string]bool // lowercase written forms of every Term's name
	maxWords int
}

var (
	reWord      = regexp.MustCompile(`[\p{L}\p{N}_]+(?:-[\p{L}\p{N}_]+)*`)
	reProtected = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|\\[[^\\]]+\\]\\([^)]*\\)")
)

// newReferences knows the names of model's Terms, less self, the Term being written.
func newReferences(model *glossary.Model, self string) *references {
	r := &references{forms: map[string]bool{}}
	for _, t := range model.Terms {
		if strings.EqualFold(t.Name, self) {
			continue
		}
		for _, f := range glossary.WrittenForms(t.Name) {
			r.forms[strings.ToLower(f)] = true
			r.maxWords = max(r.maxWords, len(reWord.FindAllString(f, -1)))
		}
	}
	return r
}

// bold wraps every mention of a Term's name or plural in **, without regard to case, longest name first. Code spans,
// bold phrases and links are left alone.
func (r *references) bold(text string) string {
	var out strings.Builder
	last := 0
	for _, p := range reProtected.FindAllStringIndex(text, -1) {
		out.WriteString(r.boldPlain(text[last:p[0]]))
		out.WriteString(text[p[0]:p[1]])
		last = p[1]
	}
	out.WriteString(r.boldPlain(text[last:]))
	return out.String()
}

func (r *references) boldPlain(text string) string {
	words := reWord.FindAllStringIndex(text, -1)
	var out strings.Builder
	last := 0
	for i := 0; i < len(words); i++ {
		for k := min(r.maxWords, len(words)-i); k > 0; k-- {
			start, end := words[i][0], words[i+k-1][1]
			phrase := text[start:end]
			if !r.forms[strings.ToLower(phrase)] || strings.Contains(phrase, "  ") || strings.ContainsAny(phrase, "\t\n") {
				continue
			}
			out.WriteString(text[last:start] + "**" + phrase + "**")
			last, i = end, i+k-1
			break
		}
	}
	out.WriteString(text[last:])
	return out.String()
}
