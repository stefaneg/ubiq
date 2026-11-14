package glossary

import (
	"fmt"
	"html"
	"regexp"
	"slices"
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
	file      string   // the file being rendered, recorded on unresolved phrases
	mentions  *mentions
	contexts  *contextNames
	lenient   bool // ADR mode: unresolved bold is emphasis, rendered as <b> and not reported
	quiet     bool // unresolved bold is marked but not reported, for text whose Statements report it
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
		plain := html.UnescapeString(phrase)
		id := r.lookup(plain, ctx)
		if id == "" {
			if r.lenient || r.contexts.key(plain) != "" {
				return "<b>" + phrase + "</b>"
			}
			if parts := r.contexts.label(plain); parts != nil {
				for _, p := range parts {
					if r.contexts.key(p) == "" {
						r.report(Unresolved{Phrase: p, Path: r.file, Where: where})
					}
				}
				return "<b>" + phrase + "</b>"
			}
			r.report(Unresolved{Phrase: plain, Path: r.file, Context: ctx, Where: where})
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

// intro renders an intro without reporting its Unresolved Phrases, which its Statements report.
func (r *Resolver) intro(lines []string, ctx string) string {
	r.quiet = true
	defer func() { r.quiet = false }()
	html, _ := r.block(lines, ctx, "")
	return html
}

// headingTerms resolves the Terms a heading names without reporting its Unresolved Phrases, which renderStatements
// reports once per heading.
func (r *Resolver) headingTerms(heading, ctx, where string) []string {
	r.quiet = true
	defer func() { r.quiet = false }()
	_, ids := r.inline(r.mentions.bold(heading, ""), ctx, where)
	return ids
}

// report records an Unresolved Phrase, unless the resolver is quiet.
func (r *Resolver) report(u Unresolved) {
	if !r.quiet {
		r.model.Unresolved = append(r.model.Unresolved, u)
	}
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

func build(contexts []*rawContext, mapStatements []rawStatement, mapHeadings []rawHeading, adrs []rawADR) *Model {
	m := &Model{Terms: map[string]*Term{}}
	r := &Resolver{byContext: map[string]map[string]string{}, model: m, contexts: newContextNames(contexts)}
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
	// pass 2: render, linking capitalised mentions as if they were bold
	r.mentions = newMentions(m.Terms)
	for _, rc := range contexts {
		r.file = rc.path
		c := Context{Key: rc.key, Name: rc.name, Path: rc.path, Intro: r.intro(rc.intro, rc.key)}
		for _, cname := range rc.clusterOrder {
			cl := rc.clusters[cname]
			cc := Cluster{Key: slug(cname), Name: cname, Intro: r.intro(cl.intro, rc.key)}
			for _, t := range cl.terms {
				id := rc.key + "/" + slug(t.name)
				term := m.Terms[id]
				term.Definition, term.Refs = r.inline(r.mentions.bold(t.def, t.name), rc.key, t.name)
				term.Refs = without(term.Refs, id)
				cc.TermIDs = append(cc.TermIDs, id)
			}
			if len(cc.TermIDs) > 0 {
				c.Clusters = append(c.Clusters, cc)
			}
		}
		m.Contexts = append(m.Contexts, c)
	}
	for _, rc := range contexts {
		r.file = rc.path
		m.Statements = append(m.Statements, r.renderStatements(rc.key, rc.statements, rc.headings)...)
	}
	r.file = mapFile
	m.Statements = append(m.Statements, r.renderStatements("", mapStatements, mapHeadings)...)
	m.ADRs = r.renderADRs(adrs)
	sort.Slice(m.Unresolved, func(i, j int) bool { return m.Unresolved[i].Phrase < m.Unresolved[j].Phrase })
	return m
}

// renderStatements resolves the phrases of each Statement of r.file, adding the Terms and Contexts its nearest
// heading names, and keeps those that mention a Term or name a Context. A Statement resolves in own, the Context of
// the Glossary it is in, or in the Context Map in the first Context it names. Each heading's own phrases are reported
// once.
func (r *Resolver) renderStatements(own string, raws []rawStatement, headings []rawHeading) []Statement {
	for _, h := range headings {
		ctx := own
		if ctx == "" {
			ctx = first(r.contexts.named(h.text))
		}
		r.inline(r.mentions.bold(h.text, ""), ctx, h.label)
	}
	var out []Statement
	for _, s := range raws {
		named := union(r.contexts.named(s.text), r.contexts.named(s.heading))
		ctx := own
		if ctx == "" {
			ctx = first(named)
		}
		h, ids := r.inline(r.mentions.bold(s.text, ""), ctx, s.label)
		if s.heading != "" {
			ids = union(ids, r.headingTerms(s.heading, ctx, s.label))
		}
		if len(ids) == 0 && len(named) == 0 {
			continue
		}
		out = append(out, Statement{Label: s.label, Path: r.file, Line: s.line, HTML: h, TermIDs: ids, Contexts: named})
	}
	return out
}

func first(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// union is a followed by the ids of b that a lacks.
func union(a, b []string) []string {
	for _, id := range b {
		if !slices.Contains(a, id) {
			a = append(a, id)
		}
	}
	return a
}

// without returns ids less id.
func without(ids []string, id string) []string {
	out := ids[:0]
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}
