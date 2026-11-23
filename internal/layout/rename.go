package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stefaneg/ubiq/internal/glossary"
)

// Rename rewrites every Reference in text to the Term named name of the Context with key so that it names
// newName, bold and plain, singular and plural, and returns the new text. text is the Glossary of the Context with key
// in, or the Context Map when in is empty; model is the Domain Model as read. A mention resolves as the parser
// resolves it, so a mention of another Term of the same name stays. Headings, which name Contexts and Clusters, Term
// name lines, Avoid lines, code and links are left alone, and so is the number of lines.
func Rename(text string, model *glossary.Model, in, key, name, newName string) string {
	term := model.TermNamed(key, name)
	if term == nil {
		return text
	}
	r := newRenamer(model, in, term.ID, newName)
	original := strings.Split(text, "\n")
	lines := withoutCode(original)
	self := ""
	for i, ln := range lines {
		switch m := reTermLine.FindStringSubmatch(ln); {
		case m != nil:
			self = strings.TrimSpace(m[1])
			continue
		case reHeading.MatchString(ln):
			self = ""
			continue
		case strings.TrimSpace(ln) == "":
			self = ""
		}
		if ln == code || strings.HasPrefix(strings.TrimSpace(ln), "_Avoid_:") {
			continue
		}
		original[i] = r.rewrite(ln, self)
	}
	return strings.Join(original, "\n")
}

// renamer rewrites the References to one Term in the text of one Context.
type renamer struct {
	in, id, to string
	order      []string                     // Context keys in map order
	byContext  map[string]map[string]string // Context key -> lowercase Term name -> Term id
	forms      map[string]string            // written form of a capitalised Term name -> the name
	maxWords   int
}

func newRenamer(model *glossary.Model, in, id, to string) *renamer {
	r := &renamer{in: in, id: id, to: to, byContext: map[string]map[string]string{}, forms: map[string]string{}}
	for _, c := range model.Contexts {
		r.order = append(r.order, c.Key)
		r.byContext[c.Key] = map[string]string{}
	}
	for _, t := range model.Terms {
		r.byContext[t.Context][strings.ToLower(t.Name)] = t.ID
		if first, _ := utf8.DecodeRuneInString(t.Name); !unicode.IsUpper(first) {
			continue
		}
		for _, f := range glossary.WrittenForms(t.Name) {
			r.forms[f] = t.Name
			r.maxWords = max(r.maxWords, len(reWord.FindAllString(f, -1)))
		}
	}
	return r
}

// rewrite is line with the References to the Term renamed; self is the name of the Term whose definition it is.
func (r *renamer) rewrite(line, self string) string {
	var out strings.Builder
	last := 0
	for _, p := range reProtected.FindAllStringIndex(line, -1) {
		out.WriteString(r.rewritePlain(line[last:p[0]], self))
		if s := line[p[0]:p[1]]; strings.HasPrefix(s, "**") {
			out.WriteString("**" + r.rewriteBold(s[2:len(s)-2]) + "**")
		} else {
			out.WriteString(s)
		}
		last = p[1]
	}
	out.WriteString(r.rewritePlain(line[last:], self))
	return out.String()
}

// rewriteBold is phrase, written in bold, renamed when it is a Reference to the Term.
func (r *renamer) rewriteBold(phrase string) string {
	if strings.Contains(phrase, ":") || strings.HasSuffix(phrase, ".") {
		return phrase
	}
	core := strings.TrimSpace(phrase)
	lead, trail := phrase[:strings.Index(phrase, core)], phrase[strings.Index(phrase, core)+len(core):]
	possessive := ""
	for _, s := range []string{"'s", "’s"} {
		if strings.HasSuffix(core, s) {
			core, possessive = strings.TrimSuffix(core, s), s
			break
		}
	}
	renamed, ok := r.renamed(core)
	if !ok {
		return phrase
	}
	return lead + renamed + possessive + trail
}

// rewritePlain is text with each capitalised mention of the Term renamed, longest Term name first, as the parser
// finds them. Mentions of self are no References.
func (r *renamer) rewritePlain(text, self string) string {
	words := reWord.FindAllStringIndex(text, -1)
	var out strings.Builder
	last := 0
	for i := 0; i < len(words); i++ {
		for k := min(r.maxWords, len(words)-i); k > 0; k-- {
			start, end := words[i][0], words[i+k-1][1]
			phrase := text[start:end]
			name, ok := r.forms[phrase]
			if !ok || name == self || strings.Contains(phrase, "  ") || strings.ContainsAny(phrase, "\t\n") {
				continue
			}
			if renamed, ok := r.renamed(phrase); ok {
				out.WriteString(text[last:start] + renamed)
				last = end
			}
			i += k - 1
			break
		}
	}
	out.WriteString(text[last:])
	return out.String()
}

// renamed is phrase as the new name, in the plural when phrase is a plural; false when phrase, resolved in r.in and
// then in the other Contexts in map order, is no Reference to the Term.
func (r *renamer) renamed(phrase string) (string, bool) {
	for _, key := range append([]string{r.in}, r.order...) {
		for _, cand := range glossary.Singulars(phrase) {
			id, ok := r.byContext[key][strings.ToLower(cand)]
			switch {
			case !ok:
				continue
			case id != r.id:
				return "", false
			case cand == phrase:
				return r.to, true
			default:
				return plural(r.to), true
			}
		}
	}
	return "", false
}

// plural is the plural of name: on the head of "<head> of <rest>", else on the last word.
func plural(name string) string {
	if head, rest, ok := strings.Cut(name, " of "); ok {
		return plural(head) + " of " + rest
	}
	l := strings.ToLower(name)
	for _, s := range []string{"s", "x", "z", "ch", "sh"} {
		if strings.HasSuffix(l, s) {
			return name + "es"
		}
	}
	if len(l) > 1 && strings.HasSuffix(l, "y") && !strings.ContainsRune("aeiou", rune(l[len(l)-2])) {
		return name[:len(name)-1] + "ies"
	}
	return name + "s"
}
