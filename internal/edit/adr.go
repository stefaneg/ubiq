package edit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ADR is a new ADR file: its name, its title on one line, and its body in the ADR format.
type ADR struct {
	Name  string
	Title string
	Body  string
}

// Record is a new ADR from a title and its plain-text paragraphs, given the names of the files already in its
// directory. The name is numbered after the highest existing, NNNN-slug.md, and the body is the title as heading
// above the paragraphs. Refused when the title or text is empty, when the title has nothing to slug, or when an
// existing ADR has the same slug.
func Record(names []string, title, text string) (ADR, error) {
	title, text = strings.Join(strings.Fields(title), " "), paragraphs(text)
	if title == "" {
		return ADR{}, errors.New("the ADR's title is empty")
	}
	if text == "" {
		return ADR{}, fmt.Errorf("the text of the ADR %s is empty", title)
	}
	s := slug(title)
	if s == "" {
		return ADR{}, fmt.Errorf("the title %s has no letters or digits to name the file by", title)
	}
	highest := 0
	for _, n := range names {
		digits, rest, _ := strings.Cut(strings.TrimSuffix(n, ".md"), "-")
		k, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		if rest == s {
			return ADR{}, fmt.Errorf("the title %s slugs to %s, as %s does", title, s, n)
		}
		highest = max(highest, k)
	}
	return ADR{Name: fmt.Sprintf("%04d-%s.md", highest+1, s), Title: title, Body: "# " + title + "\n\n" + text + "\n"}, nil
}

// paragraphs is text with trailing spaces cut from every line, blank lines at either end dropped and every run of
// blank lines made one.
func paragraphs(text string) string {
	var out []string
	for _, ln := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		ln = strings.TrimRightFunc(ln, unicode.IsSpace)
		if ln == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSuffix(strings.Join(out, "\n"), "\n")
}

// slug is title lowercased, apostrophes dropped and every run of other characters that are neither letters nor
// digits made one hyphen, with none at either end.
func slug(title string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r == '\'' || r == '’':
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
		default:
			hyphen = true
		}
	}
	return b.String()
}
