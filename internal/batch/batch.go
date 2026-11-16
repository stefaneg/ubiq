// Package batch is the Batch repository: the clone of origin the Server owns, and the open Batch every Change lands
// in. It shells out to the host's git, treats the clone as a cache of origin, and serialises Changes with one lock;
// see docs/server.md and docs/adr/0005-changes-land-in-a-numbered-batch-branch.md.
package batch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/stefaneg/ubiq/internal/prose"
)

// prefix names every Batch: ubiq/<n>.
const prefix = "ubiq/"

// Identity is a git identity: a Person's, as a Change's author, or the Server's, as its committer.
type Identity struct {
	Name  string
	Email string
}

// Repository is the clone at a root, kept as a cache of origin.
type Repository struct {
	root      string
	committer Identity

	log io.Writer

	mu      sync.Mutex // one Change or Refresh at a time
	failing string     // why the open Batch last could not be prepared, empty when it could
}

// Commit is what one Change commits: its subject and the files, relative to the root, it changed.
type Commit struct {
	Subject string
	Paths   []string
}

// Landed is where a Change landed: the Batch and the commit's full id.
type Landed struct {
	Batch  string
	Commit string
}

// Open clones origin into root when root holds no repository and reuses the clone otherwise, refusing a clone of
// another origin, then prepares the open Batch so that reads see it. A Batch that cannot be prepared is logged to
// log and is not fatal: the clone serves as it stands. committer commits every Change.
func Open(root, origin string, committer Identity, log io.Writer) (*Repository, error) {
	if !strings.Contains(origin, "://") {
		abs, err := filepath.Abs(origin)
		if err != nil {
			return nil, err
		}
		origin = abs
	}
	r := &Repository{root: root, committer: committer, log: log}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		url, err := r.git(nil, "config", "--get", "remote.origin.url")
		if err != nil || url != origin {
			return nil, fmt.Errorf("%s is a clone of %q, not of %s; give an empty -root or the clone's own origin",
				root, url, origin)
		}
	} else {
		fmt.Fprintf(log, "cloning %s into %s\n", origin, root)
		if _, err := run("", nil, "clone", "--quiet", "--", origin, root); err != nil {
			return nil, err
		}
	}
	r.Refresh()
	return r, nil
}

// Refresh prepares the open Batch with no Change, so that reads see what has reached origin since, the default
// branch's commits included. A Batch that cannot be prepared is logged, once until the reason changes, and the clone
// serves as it stands.
func (r *Repository) Refresh() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.prepare()
	r.note(err)
}

// note logs why the open Batch could not be prepared, unless it logged the same reason last time.
func (r *Repository) note(err error) {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	if reason != "" && reason != r.failing {
		fmt.Fprintf(r.log, "prepare the open Batch: %s\n", reason)
	}
	r.failing = reason
}

// Change lands one Change in the open Batch: it fetches origin and checks the open Batch out with the default branch
// merged in, runs apply on the clone, and commits what apply changed, authored by author and committed by the Server,
// then pushes. When origin refuses the push because the Batch moved, it fetches and runs apply once more on the
// Batch as it now is. When apply fails, nothing lands and the clone is left as the Batch.
func (r *Repository) Change(author Identity, apply func() (Commit, error)) (Landed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	landed, err := r.land(author, apply)
	var rejected *rejectedPush
	if errors.As(err, &rejected) {
		landed, err = r.land(author, apply)
	}
	return landed, err
}

// rejectedPush is origin refusing a Change's push.
type rejectedPush struct{ err error }

func (e *rejectedPush) Error() string { return e.err.Error() }

// land prepares the open Batch, runs apply and commits and pushes what it changed.
func (r *Repository) land(author Identity, apply func() (Commit, error)) (Landed, error) {
	batch, err := r.prepare()
	r.note(err)
	if err != nil {
		return Landed{}, err
	}
	c, err := apply()
	if err != nil {
		r.discard()
		return Landed{}, err
	}
	commit, err := r.commit(author, c)
	if err != nil {
		r.discard()
		return Landed{}, err
	}
	if _, err := r.git(nil, "push", "--quiet", "origin", "HEAD:refs/heads/"+batch); err != nil {
		return Landed{}, &rejectedPush{err}
	}
	return Landed{Batch: batch, Commit: commit}, nil
}

// prepare fetches origin and checks out the open Batch, ubiq/<n> with the highest n on origin, or ubiq/1 from the
// default branch when origin has none, with the default branch merged in. A Batch that holds a Change and then
// differs from the default branch in nothing a Maintainer has merged, so ubiq/<n+1> opens from the default branch.
// It returns the Batch's name.
func (r *Repository) prepare() (string, error) {
	if _, err := r.git(nil, "fetch", "--quiet", "--prune", "origin"); err != nil {
		return "", err
	}
	main, err := r.defaultBranch()
	if err != nil {
		return "", err
	}
	refs, err := r.git(nil, "for-each-ref", "--format=%(refname:lstrip=3)", "refs/remotes/origin/"+prefix)
	if err != nil {
		return "", err
	}
	var numbers []int
	for _, ref := range strings.Fields(refs) {
		if i, err := strconv.Atoi(strings.TrimPrefix(ref, prefix)); err == nil && i > 0 {
			numbers = append(numbers, i)
		}
	}
	if len(numbers) == 0 {
		return r.checkout(prefix+"1", "origin/"+main)
	}
	n := slices.Max(numbers)
	batch, err := r.checkout(prefix+strconv.Itoa(n), "origin/"+prefix+strconv.Itoa(n))
	if err != nil {
		return "", err
	}
	if _, err := r.git(r.committer.env("COMMITTER", "AUTHOR"), "merge", "--quiet", "--no-edit", "origin/"+main); err != nil {
		files, _ := r.git(nil, "diff", "--name-only", "--diff-filter=U")
		_, _ = r.git(nil, "merge", "--abort")
		if files == "" {
			return "", fmt.Errorf("merge the default branch %s into the Batch %s: %w", main, batch, err)
		}
		return "", fmt.Errorf("The Batch %s conflicts with the default branch %s in %s, so Changes are refused until "+
			"a Maintainer merges %s into %s and pushes.", batch, main, prose.List(strings.Fields(files)), main, batch)
	}
	same, err := r.sameTree("HEAD", "origin/"+main)
	if err != nil || !same {
		return batch, err
	}
	held, err := r.holdsChange(n, numbers)
	if err != nil || !held {
		return batch, err
	}
	return r.checkout(prefix+strconv.Itoa(n+1), "origin/"+main)
}

// sameTree tells whether two commits hold the same files.
func (r *Repository) sameTree(a, b string) (bool, error) {
	trees, err := r.git(nil, "rev-parse", a+"^{tree}", b+"^{tree}")
	if err != nil {
		return false, err
	}
	ids := strings.Fields(trees)
	return len(ids) == 2 && ids[0] == ids[1], nil
}

// holdsChange tells whether the Batch ubiq/<n> on origin has a commit the Server committed that is its own: one no
// earlier Batch among numbers reaches, as a Batch opened by hand from the default branch after a merge would.
func (r *Repository) holdsChange(n int, numbers []int) (bool, error) {
	args := []string{"rev-list", "-n1", "--fixed-strings", "--committer=<" + r.committer.Email + ">",
		"origin/" + prefix + strconv.Itoa(n), "--not"}
	for _, i := range numbers {
		if i < n {
			args = append(args, "origin/"+prefix+strconv.Itoa(i))
		}
	}
	out, err := r.git(nil, args...)
	return out != "", err
}

// defaultBranch is the branch origin's HEAD points at, asked of origin since a fetch does not refresh it.
func (r *Repository) defaultBranch() (string, error) {
	out, err := r.git(nil, "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if ref, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			name, _, _ := strings.Cut(ref, "\t")
			return name, nil
		}
	}
	return "", errors.New("origin's HEAD names no branch")
}

// checkout puts the clone on branch, reset to start, with nothing else in the working tree.
func (r *Repository) checkout(branch, start string) (string, error) {
	if _, err := r.git(nil, "checkout", "--quiet", "--force", "-B", branch, start); err != nil {
		return "", err
	}
	if _, err := r.git(nil, "clean", "--quiet", "--force", "-d"); err != nil {
		return "", err
	}
	return branch, nil
}

// commit commits the paths of c as one commit by author and returns its full id.
func (r *Repository) commit(author Identity, c Commit) (string, error) {
	if _, err := r.git(nil, append([]string{"add", "--all", "--"}, c.Paths...)...); err != nil {
		return "", err
	}
	env := append(author.env("AUTHOR"), r.committer.env("COMMITTER")...)
	if _, err := r.git(env, "commit", "--quiet", "--no-verify", "-m", c.Subject); err != nil {
		return "", err
	}
	return r.git(nil, "rev-parse", "HEAD")
}

// discard drops whatever the working tree holds beyond the commit checked out.
func (r *Repository) discard() {
	_, _ = r.git(nil, "reset", "--quiet", "--hard")
	_, _ = r.git(nil, "clean", "--quiet", "--force", "-d")
}

// env is the git environment variables that make id the given roles, AUTHOR or COMMITTER.
func (id Identity) env(roles ...string) []string {
	var env []string
	for _, role := range roles {
		env = append(env, "GIT_"+role+"_NAME="+id.Name, "GIT_"+role+"_EMAIL="+id.Email)
	}
	return env
}

// git runs git in the clone with env added to the process's environment.
func (r *Repository) git(env []string, args ...string) (string, error) {
	return run(r.root, env, args...)
}

// run runs the host's git in dir and returns its trimmed output. git never prompts: a missing credential is an
// error, not a hang.
func run(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), env...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	return strings.TrimSpace(out.String()), nil
}
