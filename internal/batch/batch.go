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
	"strconv"
	"strings"
	"sync"
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

	mu sync.Mutex // one Change at a time
}

// Edit is what a Change wrote to the clone: the commit's subject and the files, relative to the root, it changed.
type Edit struct {
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
	r := &Repository{root: root, committer: committer}
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
	if _, err := r.prepare(); err != nil {
		fmt.Fprintf(log, "prepare the open Batch: %s\n", err)
	}
	return r, nil
}

// Change lands one Change in the open Batch: it fetches origin and checks the open Batch out with the default branch
// merged in, runs edit on the clone, and commits what edit wrote, authored by author and committed by the Server,
// then pushes. When origin refuses the push because the Batch moved, it fetches and runs edit once more on the
// Batch as it now is. When edit fails, nothing lands and the clone is left as the Batch.
func (r *Repository) Change(author Identity, edit func() (Edit, error)) (Landed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	landed, err := r.land(author, edit)
	var rejected *rejectedPush
	if errors.As(err, &rejected) {
		landed, err = r.land(author, edit)
	}
	return landed, err
}

// rejectedPush is origin refusing a Change's push.
type rejectedPush struct{ err error }

func (e *rejectedPush) Error() string { return e.err.Error() }

// land prepares the open Batch, runs edit and commits and pushes what it wrote.
func (r *Repository) land(author Identity, edit func() (Edit, error)) (Landed, error) {
	batch, err := r.prepare()
	if err != nil {
		return Landed{}, err
	}
	e, err := edit()
	if err != nil {
		r.discard()
		return Landed{}, err
	}
	commit, err := r.commit(author, e)
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
// default branch when origin has none, with the default branch merged in. It returns the Batch's name.
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
	n := 0
	for _, ref := range strings.Fields(refs) {
		if i, err := strconv.Atoi(strings.TrimPrefix(ref, prefix)); err == nil && i > n {
			n = i
		}
	}
	if n == 0 {
		return r.checkout(prefix+"1", "origin/"+main)
	}
	batch, err := r.checkout(prefix+strconv.Itoa(n), "origin/"+prefix+strconv.Itoa(n))
	if err != nil {
		return "", err
	}
	if _, err := r.git(r.committer.env("COMMITTER", "AUTHOR"), "merge", "--quiet", "--no-edit", "origin/"+main); err != nil {
		_, _ = r.git(nil, "merge", "--abort")
		return "", fmt.Errorf("merge the default branch %s into the Batch %s: %w", main, batch, err)
	}
	return batch, nil
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

// commit commits the paths of e as one commit by author and returns its full id.
func (r *Repository) commit(author Identity, e Edit) (string, error) {
	if _, err := r.git(nil, append([]string{"add", "--all", "--"}, e.Paths...)...); err != nil {
		return "", err
	}
	env := append(author.env("AUTHOR"), r.committer.env("COMMITTER")...)
	if _, err := r.git(env, "commit", "--quiet", "--no-verify", "-m", e.Subject); err != nil {
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
