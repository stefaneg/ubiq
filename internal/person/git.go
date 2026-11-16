package person

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
)

// Git resolves every request to whoever holds the working tree at root: git's user.name and user.email there, with
// role Maintainer. It reads them on every call, so a change to git's config shows at once.
func Git(root string) Resolver { return gitResolver(root) }

type gitResolver string

func (root gitResolver) Resolve(http.Header) (Person, error) {
	name, err := gitConfig(string(root), "user.name")
	if err != nil {
		return Person{}, err
	}
	email, err := gitConfig(string(root), "user.email")
	if err != nil {
		return Person{}, err
	}
	if name == "" && email == "" {
		return Person{}, errors.New("git has no user.name or user.email for this working tree, so ubiq cannot tell " +
			"who you are; set them with git config.")
	}
	return Person{Name: name, Email: email, Role: Maintainer}, nil
}

// gitConfig is the value of key in the working tree at root, or empty when git has none.
func gitConfig(root, key string) (string, error) {
	out, err := exec.Command("git", "-C", root, "config", "--get", key).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read git's %s under %s: %w", key, root, err)
	}
	return strings.TrimSpace(string(out)), nil
}
