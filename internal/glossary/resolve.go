package glossary

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// avoidedWords splits an Avoid line into words, dropping parenthetical notes.
func avoidedWords(avoid string) []string {
	var out []string
	for _, w := range strings.Split(avoid, ",") {
		w = strings.TrimSpace(reParen.ReplaceAllString(w, ""))
		if w != "" {
			out = append(out, strings.ToLower(w))
		}
	}
	return out
}

var reParen = regexp.MustCompile(`\s*\([^)]*\)`)

// redirects is the Avoid words that name a synonym. A word made only of words from the term's own name, such as
// "name" under Image Base Name, is a short form of the term rather than a synonym and is left out.
func (t *Term) redirects() []string {
	own := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(t.Name), isWordBreak) {
		own[w] = true
	}
	var out []string
	for _, a := range avoidedWords(t.Avoid) {
		short := true
		for _, w := range strings.FieldsFunc(a, isWordBreak) {
			if !own[w] {
				short = false
			}
		}
		if !short {
			out = append(out, a)
		}
	}
	return out
}

func isWordBreak(r rune) bool { return r == ' ' || r == '-' }

// Resolver maps bold phrases to term ids.
type Resolver struct {
	byContext map[string]map[string]string // context -> lowercase name -> id
	model     *Model
	order     []string // resolution order after a phrase's own context: map order
	lenient   bool     // ADR mode: unresolved bold is emphasis, rendered as <b> and not reported
}

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// Singulars lists the phrase and its singular forms. A phrase shaped "<head> of <rest>" carries
// its plural on the head noun, so that is singularised too.
func Singulars(name string) []string {
	out := []string{name}
	if head, rest, ok := strings.Cut(name, " of "); ok {
		for _, h := range singularWords(head)[1:] {
			out = append(out, h+" of "+rest)
		}
		return out
	}
	return append(out, singularWords(name)[1:]...)
}

// singularWords lists a word and its candidate singulars by suffix.
func singularWords(name string) []string {
	out := []string{name}
	l := strings.ToLower(name)
	switch {
	case strings.HasSuffix(l, "ies"):
		out = append(out, name[:len(name)-3]+"y")
	case strings.HasSuffix(l, "ses"), strings.HasSuffix(l, "xes"), strings.HasSuffix(l, "ches"), strings.HasSuffix(l, "shes"):
		out = append(out, name[:len(name)-2])
	}
	if strings.HasSuffix(l, "s") {
		out = append(out, name[:len(name)-1])
	}
	return out
}

// HeadPlurals lists the written plurals of a phrase shaped "<head> of <rest>", which the
// trailing-suffix match cannot reach. Other phrases yield nothing.
func HeadPlurals(name string) []string {
	head, rest, ok := strings.Cut(name, " of ")
	if !ok {
		return nil
	}
	forms := []string{head + "s", head + "es"}
	if strings.HasSuffix(head, "y") {
		forms = append(forms, head[:len(head)-1]+"ies")
	}
	out := make([]string, 0, len(forms))
	for _, f := range forms {
		out = append(out, f+" of "+rest)
	}
	return out
}

// lookup tries the phrase's own context, then the others in map order.
func (r *Resolver) lookup(phrase, ctx string) string {
	phrase = strings.TrimSpace(phrase)
	phrase = strings.TrimSuffix(phrase, "'s")
	phrase = strings.TrimSuffix(phrase, "’s")
	order := []string{ctx}
	for _, k := range r.order {
		if k != ctx {
			order = append(order, k)
		}
	}
	for _, k := range order {
		m := r.byContext[k]
		if m == nil {
			continue
		}
		for _, cand := range Singulars(phrase) {
			if id, ok := m[strings.ToLower(cand)]; ok {
				return id
			}
		}
	}
	return ""
}

var (
	reBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reCode = regexp.MustCompile("`([^`]+)`")
	reLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	reEm   = regexp.MustCompile(`(^|[\s(])[_*]([^_*]+)[_*]([\s).,;:]|$)`)
)

// inline renders markdown-lite to HTML, linking resolved bold phrases.
// Returns html and the ordered unique list of resolved term ids.
func (r *Resolver) inline(text, ctx, where string) (string, []string) {
	var ids []string
	seen := map[string]bool{}
	// protect code spans first
	codes := []string{}
	text = reCode.ReplaceAllStringFunc(text, func(m string) string {
		codes = append(codes, html.EscapeString(m[1:len(m)-1]))
		return fmt.Sprintf("\x00%d\x00", len(codes)-1)
	})
	text = reLink.ReplaceAllString(text, "$1")
	// bold inside link text already stripped to plain; bold terms:
	text = html.EscapeString(text)
	text = reBold.ReplaceAllStringFunc(text, func(m string) string {
		phrase := m[2 : len(m)-2]
		if strings.Contains(phrase, ":") || strings.HasSuffix(phrase, ".") { // speaker label, "Mission:" line, or a bold sentence
			return "<b>" + phrase + "</b>"
		}
		id := r.lookup(html.UnescapeString(phrase), ctx)
		if id == "" {
			if r.lenient {
				return "<b>" + phrase + "</b>"
			}
			r.model.Unresolved = append(r.model.Unresolved, Unresolved{Phrase: html.UnescapeString(phrase), Context: ctx, Where: where})
			return `<span class="unres">` + phrase + `</span>`
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		return fmt.Sprintf(`<a class="t" href="#%s" data-id="%s">%s</a>`, id, id, phrase)
	})
	text = reEm.ReplaceAllString(text, "$1<em>$2</em>$3")
	for i, c := range codes {
		text = strings.Replace(text, fmt.Sprintf("\x00%d\x00", i), "<code>"+c+"</code>", 1)
	}
	return text, ids
}

// block renders a list of markdown lines (headings, paragraphs, bullets,
// pipe tables) to HTML. Indented lines after a bullet nest inside that bullet.
func (r *Resolver) block(lines []string, ctx, where string) (string, []string) {
	var out strings.Builder
	var ids []string
	seen := map[string]bool{}
	add := func(x []string) {
		for _, id := range x {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	var para []string
	inList := ""
	liOpen := false
	flushPara := func() {
		if len(para) > 0 {
			h, i := r.inline(strings.Join(para, " "), ctx, where)
			add(i)
			out.WriteString("<p>" + h + "</p>")
			para = nil
		}
	}
	closeLi := func() {
		flushPara()
		if liOpen {
			out.WriteString("</li>")
			liOpen = false
		}
	}
	closeList := func() {
		closeLi()
		if inList != "" {
			out.WriteString("</" + inList + ">")
			inList = ""
		}
	}
	reNum := regexp.MustCompile(`^\s*\d+\. (.*)$`)
	reSep := regexp.MustCompile(`^[\s|:-]+$`)
	i := 0
	for i < len(lines) {
		ln := lines[i]
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			flushPara()
			i++
			continue
		}
		if m := reH.FindStringSubmatch(ln); m != nil {
			closeList()
			h, x := r.inline(m[2], ctx, where)
			add(x)
			lvl := len(m[1]) + 1
			if lvl > 6 {
				lvl = 6
			}
			fmt.Fprintf(&out, "<h%d class=\"mdh\">%s</h%d>", lvl, h, lvl)
			i++
			continue
		}
		var item string
		tag := ""
		if m := reBullet.FindStringSubmatch(ln); m != nil && m[1] == "" {
			item, tag = m[2], "ul"
		} else if m := reNum.FindStringSubmatch(ln); m != nil {
			item, tag = m[1], "ol"
		}
		if tag != "" {
			closeLi()
			if inList != tag {
				closeList()
				out.WriteString("<" + tag + ">")
				inList = tag
			}
			i++
			for i < len(lines) && strings.HasPrefix(lines[i], "  ") && strings.TrimSpace(lines[i]) != "" && !strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				item += " " + strings.TrimSpace(lines[i])
				i++
			}
			h, x := r.inline(item, ctx, where)
			add(x)
			out.WriteString("<li>" + h)
			liOpen = true
			continue
		}
		if strings.HasPrefix(trimmed, "|") {
			flushPara()
			if !liOpen {
				closeList()
			}
			out.WriteString("<table class=\"md\">")
			row := 0
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				t := strings.TrimSpace(lines[i])
				i++
				if reSep.MatchString(t) {
					continue
				}
				t = strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|")
				cell := "td"
				if row == 0 {
					cell = "th"
				}
				out.WriteString("<tr>")
				for _, c := range strings.Split(t, "|") {
					h, x := r.inline(strings.TrimSpace(c), ctx, where)
					add(x)
					out.WriteString("<" + cell + ">" + h + "</" + cell + ">")
				}
				out.WriteString("</tr>")
				row++
			}
			out.WriteString("</table>")
			continue
		}
		// paragraph line; an unindented one ends any open list
		if inList != "" && (!strings.HasPrefix(ln, "  ") || !liOpen) {
			closeList()
		}
		para = append(para, trimmed)
		i++
	}
	flushPara()
	closeList()
	return out.String(), ids
}

func build(contexts []*rawContext, adrs []rawADR) *Model {
	m := &Model{Terms: map[string]*Term{}}
	r := &Resolver{byContext: map[string]map[string]string{}, model: m}
	// pass 1: register terms
	for _, rc := range contexts {
		r.order = append(r.order, rc.key)
		r.byContext[rc.key] = map[string]string{}
		for _, cname := range rc.clusterOrder {
			for _, t := range rc.clusters[cname].terms {
				id := rc.key + "/" + slug(t.name)
				r.byContext[rc.key][strings.ToLower(t.name)] = id
				term := &Term{ID: id, Name: t.name, Context: rc.key, Cluster: cname, Avoid: t.avoid, Line: t.line}
				term.Redirects = term.redirects()
				m.Terms[id] = term
			}
		}
	}
	// pass 2: render
	for _, rc := range contexts {
		intro, _ := r.block(rc.intro, rc.key, rc.name+" intro")
		c := Context{Key: rc.key, Name: rc.name, Path: rc.path, Intro: intro}
		for _, cname := range rc.clusterOrder {
			cl := rc.clusters[cname]
			ci, _ := r.block(cl.intro, rc.key, cname+" intro")
			cc := Cluster{Key: slug(cname), Name: cname, Intro: ci}
			for _, t := range cl.terms {
				id := rc.key + "/" + slug(t.name)
				term := m.Terms[id]
				term.Definition, term.Refs = r.inline(t.def, rc.key, t.name)
				cc.TermIDs = append(cc.TermIDs, id)
			}
			if len(cc.TermIDs) > 0 {
				c.Clusters = append(c.Clusters, cc)
			}
		}
		m.Contexts = append(m.Contexts, c)
	}
	m.ADRs = r.renderADRs(adrs)
	sort.Slice(m.Unresolved, func(i, j int) bool { return m.Unresolved[i].Phrase < m.Unresolved[j].Phrase })
	return m
}
