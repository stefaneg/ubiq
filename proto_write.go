// PROTOTYPE, throwaway: the write side of `ubiq serve`. Answers "how does a write behave, end to end": a Writer
// calls define_term over MCP, the Server lands it as a commit authored by the Person in the open Batch of a clone
// it owns, pushes, and the next lookup sees the Term. Shells out to git. No tests, not for main.
//
// Batch rules followed (ticket "Which branch writes land on"): origin is truth and the clone a cache; fetch before
// every Change; the open Batch is ubiq/<n> with the highest n on origin; the default branch is merged into it first
// and a conflict refuses the Change; a Batch that has Changes and no longer differs from the default branch counts
// as merged, so the next Change opens ubiq/<n+1>; a Batch without Changes is never merged.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
)

// batchRepo is the clone the Server owns.
type batchRepo struct {
	dir, origin, defaultBranch, prefix string
	committer                          string // "Name <email>", the Server's own identity
	status                             io.Writer
	mu                                 sync.Mutex // Changes are serialised
}

func (b *batchRepo) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = b.dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		return strings.TrimSpace(out.String()), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// ensureClone clones origin into dir when dir has no repository yet.
func (b *batchRepo) ensureClone() error {
	if _, err := os.Stat(filepath.Join(b.dir, ".git")); err == nil {
		return nil
	}
	fmt.Fprintf(b.status, "cloning %s into %s\n", b.origin, b.dir)
	cmd := exec.Command("git", "clone", "--quiet", b.origin, b.dir)
	cmd.Stderr = b.status
	return cmd.Run()
}

type batchConflict struct{ batch, detail string }

func (e *batchConflict) Error() string {
	return fmt.Sprintf("Batch %s is in conflict with %s; a Maintainer must resolve it on the branch before Changes "+
		"can land. %s", e.batch, "the default branch", e.detail)
}

// prepare fetches origin, picks the open Batch, checks it out with the default branch merged in, and reports its
// name. The working tree is then the Batch as origin has it plus the default branch's latest.
func (b *batchRepo) prepare() (string, error) {
	if _, err := b.git("fetch", "--quiet", "--prune", "origin"); err != nil {
		return "", err
	}
	main := "origin/" + b.defaultBranch
	list, err := b.git("for-each-ref", "--format=%(refname:short)", "refs/remotes/origin/"+b.prefix)
	if err != nil {
		return "", err
	}
	n := 0
	for _, ref := range strings.Split(list, "\n") {
		if i, err := strconv.Atoi(strings.TrimPrefix(ref, "origin/"+b.prefix)); err == nil && i > n {
			n = i
		}
	}
	if n == 0 {
		return b.openBatch(1, main)
	}
	name := b.prefix + strconv.Itoa(n)
	if _, err := b.git("checkout", "--quiet", "-B", name, "origin/"+name); err != nil {
		return "", err
	}
	changes, err := b.git("rev-list", "--count", main+"..HEAD")
	if err != nil {
		return "", err
	}
	if changes == "0" {
		// Nothing ahead of the default branch. Either the Batch was merged with its history kept (fast-forward or
		// merge commit), so the next Batch opens, or it never had a Change and is taken as it is. "Has Changes" is
		// read as "has a commit the Server committed": a Batch opened by hand with no Changes is never merged.
		if landed, _ := b.git("rev-list", "-n1", "--committer="+b.committerEmail(), "HEAD"); landed != "" {
			return b.openBatch(n+1, main)
		}
		if _, err := b.git("reset", "--quiet", "--hard", main); err != nil {
			return "", err
		}
		return name, nil
	}
	if _, err := b.git("-c", "user.name="+b.committerName(), "-c", "user.email="+b.committerEmail(),
		"merge", "--quiet", "--no-edit", main); err != nil {
		conflicts, _ := b.git("diff", "--name-only", "--diff-filter=U")
		b.git("merge", "--abort")
		return "", &batchConflict{batch: name, detail: "Conflicting files: " + strings.ReplaceAll(conflicts, "\n", ", ")}
	}
	if _, err := b.git("diff", "--quiet", main); err == nil {
		// Changes, and no difference to the default branch: this Batch has been merged. Open the next.
		return b.openBatch(n+1, main)
	}
	return name, nil
}

func (b *batchRepo) openBatch(n int, from string) (string, error) {
	name := b.prefix + strconv.Itoa(n)
	fmt.Fprintf(b.status, "opening Batch %s from %s\n", name, from)
	_, err := b.git("checkout", "--quiet", "-B", name, from)
	return name, err
}

func (b *batchRepo) committerName() string {
	return strings.TrimSpace(strings.Split(b.committer, "<")[0])
}
func (b *batchRepo) committerEmail() string {
	return strings.Trim(strings.SplitN(b.committer+"<", "<", 2)[1], "<> ")
}

// commit records the working tree as one Change authored by the Person and pushes the Batch.
func (b *batchRepo) commit(batch, subject string, p *person) (string, error) {
	if _, err := b.git("add", "-A"); err != nil {
		return "", err
	}
	author := fmt.Sprintf("%s <%s>", p.Name, p.Email)
	if _, err := b.git("-c", "user.name="+b.committerName(), "-c", "user.email="+b.committerEmail(),
		"commit", "--quiet", "--author="+author, "-m", subject); err != nil {
		return "", err
	}
	id, err := b.git("rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	if _, err := b.git("push", "--quiet", "origin", "HEAD:refs/heads/"+batch); err != nil {
		// Someone pushed to the Batch in between; the resolution says fetch and retry. The prototype reports it.
		return "", fmt.Errorf("push %s rejected, the Change is committed locally only: %w", batch, err)
	}
	return id, nil
}

// --- define_term ---

type defineTermInput struct {
	Name       string   `json:"name" jsonschema:"the Term's name, as it should be written"`
	Definition string   `json:"definition" jsonschema:"plain text, one or two sentences; existing Term names in it become References"`
	Avoid      []string `json:"avoid,omitempty" jsonschema:"words the Term rules out in favour of itself"`
	Cluster    string   `json:"cluster,omitempty" jsonschema:"the Cluster heading to put the Term under; an unknown one is created at the end of the Glossary"`
}

const defineTermDescription = `Define a new Term in this repository's glossary. The Term lands as a commit authored ` +
	`by you in the open Batch branch and the next lookup sees it. Refused when you are a Reader, when a Term of that ` +
	`name exists (use revise_term), when name or definition is empty, or when the Batch is in conflict with the ` +
	`default branch. Findings the Term introduces (an orphan, an unresolved phrase, a long definition) do not refuse ` +
	`it; they come back as warnings. Look the words up first so the definition reuses existing Terms.`

func addDefineTerm(srv *mcp.Server, repo *batchRepo, who func(context.Context, http.Header) (*person, error)) {
	mcp.AddTool(srv, &mcp.Tool{Name: "define_term", Description: defineTermDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(bool), IdempotentHint: false, OpenWorldHint: new(bool)}},
		func(ctx context.Context, req *mcp.CallToolRequest, in defineTermInput) (*mcp.CallToolResult, any, error) {
			p, err := who(ctx, req.Extra.Header)
			if err != nil {
				return nil, nil, err
			}
			if p.Role != "Writer" {
				return nil, nil, fmt.Errorf("%s is a Reader here and may not change the glossary", p.Email)
			}
			if repo == nil {
				return nil, nil, errors.New("this Server has no origin configured, so it cannot land Changes")
			}
			out, err := defineTerm(repo, p, in)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
		})
}

func defineTerm(repo *batchRepo, p *person, in defineTermInput) (string, error) {
	in.Name, in.Definition = strings.TrimSpace(in.Name), strings.TrimSpace(in.Definition)
	if in.Name == "" || in.Definition == "" {
		return "", errors.New("name and definition must not be empty")
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	started := time.Now()
	batch, err := repo.prepare()
	if err != nil {
		return "", err
	}
	before, err := glossary.Load(repo.dir)
	if err != nil {
		return "", err
	}
	if len(before.Contexts) != 1 {
		return "", fmt.Errorf("the prototype writes single-Context repositories only; this one has %d", len(before.Contexts))
	}
	for _, t := range before.Terms {
		if strings.EqualFold(t.Name, in.Name) {
			return "", fmt.Errorf("a Term named %s exists already (%s:%d); use revise_term to change it", t.Name, t.Context, t.Line)
		}
	}
	path := filepath.Join(repo.dir, before.Contexts[0].Path)
	entry := renderEntry(in, before)
	if err := insertEntry(path, in.Cluster, entry); err != nil {
		return "", err
	}
	after, err := glossary.Load(repo.dir)
	if err != nil {
		return "", fmt.Errorf("the glossary no longer reads after the edit: %w", err)
	}
	id, err := repo.commit(batch, "Define Term "+in.Name, p)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(repo.status, "%s Change: Define Term %s by %s, %s in %s, %s\n", time.Now().Format("15:04:05"),
		in.Name, p.Email, id, batch, time.Since(started).Round(time.Millisecond))
	var b strings.Builder
	fmt.Fprintf(&b, "Defined Term %s: commit %s in Batch %s, authored by %s <%s>.\n\n%s\n", in.Name, id, batch, p.Name, p.Email, entry)
	introduced := newFindings(health.Check(before), health.Check(after))
	if len(introduced) == 0 {
		b.WriteString("\nNo Findings introduced.\n")
	} else {
		b.WriteString("\nFindings this Change introduced:\n")
		health.Write(&b, introduced)
	}
	return b.String(), nil
}

// renderEntry writes the Term entry in GLOSSARY.md form, bolding every exact mention of an existing Term in the
// definition and leaving everything else as typed.
func renderEntry(in defineTermInput, model *glossary.Model) string {
	names := make([]string, 0, len(model.Terms))
	for _, t := range model.Terms {
		names = append(names, t.Name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) }) // longest first
	def := in.Definition
	for _, n := range names {
		re := regexp.MustCompile(`(^|[^\pL\pN*])(` + regexp.QuoteMeta(n) + `s?)([^\pL\pN*]|$)`)
		def = re.ReplaceAllString(def, "$1**$2**$3")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**:\n%s\n", in.Name, def)
	if len(in.Avoid) > 0 {
		fmt.Fprintf(&b, "_Avoid_: %s\n", strings.Join(in.Avoid, ", "))
	}
	return b.String()
}

// insertEntry puts the entry at the end of the named Cluster, or at the end of the Glossary's Terms when no
// Cluster is named; an unknown Cluster is created after the last Term, before whatever section follows the Terms.
func insertEntry(path, cluster, entry string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	at := afterLastTerm(lines)
	if cluster != "" {
		found := false
		for i, l := range lines {
			if !found {
				if h, ok := heading(l); ok && strings.EqualFold(h, cluster) {
					found = true
					at = len(lines)
				}
				continue
			}
			if _, ok := heading(l); ok {
				at = i
				break
			}
		}
		if !found {
			entry = "### " + cluster + "\n\n" + entry
		}
	}
	for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	block := append([]string{""}, strings.Split(strings.TrimRight(entry, "\n"), "\n")...)
	if at < len(lines) && strings.TrimSpace(lines[at]) != "" {
		block = append(block, "")
	}
	lines = append(lines[:at], append(block, lines[at:]...)...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// afterLastTerm is the index of the first "##" heading after the last Term entry, or the end of the file.
func afterLastTerm(lines []string) int {
	last := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "**") && strings.HasSuffix(strings.TrimSpace(l), "**:") {
			last = i
		}
	}
	for i := last + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			return i
		}
	}
	return len(lines)
}

func heading(line string) (string, bool) {
	if !strings.HasPrefix(line, "##") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(line, "#")), true
}

// newFindings is the Findings in after that before did not have, matched by rule and name since lines move.
func newFindings(before, after []health.Finding) []health.Finding {
	seen := map[string]bool{}
	for _, f := range before {
		seen[f.Rule+"|"+f.Name] = true
	}
	var out []health.Finding
	for _, f := range after {
		if !seen[f.Rule+"|"+f.Name] {
			out = append(out, f)
		}
	}
	return out
}
