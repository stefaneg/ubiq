package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/stefaneg/ubiq/internal/batch"
	"github.com/stefaneg/ubiq/internal/person"
	"github.com/stefaneg/ubiq/internal/server"
)

// runServe runs the Server until it fails, refusing to start when its flags are inconsistent; see docs/server.md.
func runServe(args []string, status io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(status)
	root := fs.String("root", ".", "the clone the Server owns")
	origin := fs.String("origin", "", "the repository's https URL (required)")
	listen := fs.String("listen", "127.0.0.1:8766", "the address to listen on, loopback behind the proxy")
	keys := fs.String("assertion-keys", "", "URL of the keys that sign the proxy's assertion (required unless -person)")
	ca := fs.String("ca", "", "extra CA certificate file to trust when fetching the assertion keys")
	claim := fs.String("writer-claim", "groups", "the Writer Rule's claim")
	writers := fs.String("writers", "", "the Writer Rule's accepted values, comma-separated (required)")
	committer := fs.String("committer", "ubiq serve <ubiq@localhost>", `the Server's git identity, "Name <email>"`)
	fixed := fs.String("person", "", `dev direct mode: every request is this Writer, "Name <email>", on loopback only`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	accepted := values(*writers)
	if len(accepted) == 0 {
		return errors.New("ubiq serve: -writers is missing, so nobody may write; name the Writer Rule's accepted values.")
	}
	switch {
	case *keys == "" && *fixed == "":
		return errors.New("ubiq serve: give -assertion-keys behind a proxy, or -person in dev direct mode.")
	case *keys != "" && *fixed != "":
		return errors.New("ubiq serve: give -assertion-keys or -person, not both.")
	}
	if *fixed != "" && !loopback(*listen) {
		return fmt.Errorf("ubiq serve: -person is for dev direct mode on loopback, and -listen %q is not loopback.", *listen)
	}
	if err := checkOrigin(*origin); err != nil {
		return err
	}
	bot, err := identity("committer", *committer)
	if err != nil {
		return err
	}
	rule := person.WriterRule{Claim: *claim, Values: accepted}
	people, err := resolver(*keys, *ca, *fixed, rule)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer l.Close()
	repo, err := batch.Open(abs, *origin, batch.Identity{Name: bot.Name, Email: bot.Address}, status)
	if err != nil {
		return fmt.Errorf("ubiq serve: %w.", err)
	}
	h := server.New(server.Options{Root: abs, Version: version(), People: people, Writers: rule, Batches: repo, Log: status})
	return http.Serve(l, h)
}

// resolver is the Person resolver the flags ask for: the assertion verifier for the keys at keys, trusting the CA
// file ca when given, or the fixed Person of dev direct mode.
func resolver(keys, ca, fixed string, rule person.WriterRule) (person.Resolver, error) {
	if keys == "" {
		p, err := identity("person", fixed)
		if err != nil {
			return nil, err
		}
		return person.Fixed(p.Name, p.Address), nil
	}
	var pem []byte
	if ca != "" {
		var err error
		if pem, err = os.ReadFile(ca); err != nil {
			return nil, fmt.Errorf("ubiq serve: -ca %q: %w.", ca, err)
		}
	}
	r, err := person.Assertion(keys, pem, rule)
	if err != nil {
		return nil, fmt.Errorf("ubiq serve: -ca %q: %w.", ca, err)
	}
	return r, nil
}

// values splits a comma-separated flag into its trimmed, non-empty values.
func values(flag string) []string {
	var vs []string
	for _, v := range strings.Split(flag, ",") {
		if v = strings.TrimSpace(v); v != "" {
			vs = append(vs, v)
		}
	}
	return vs
}

// loopback tells whether addr listens on a loopback address only.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkOrigin refuses an origin the Server cannot push to with git's stored HTTPS credential: none, SSH, or any
// scheme but https. A local path or file URL is a development origin and passes.
func checkOrigin(origin string) error {
	if origin == "" {
		return errors.New("ubiq serve: -origin is missing; give the repository's https URL.")
	}
	scheme, _, hasScheme := strings.Cut(origin, "://")
	colon, slash := strings.Index(origin, ":"), strings.Index(origin, "/")
	scpLike := !hasScheme && colon > 0 && (slash < 0 || colon < slash)
	if scpLike || strings.Contains(scheme, "ssh") {
		return fmt.Errorf("ubiq serve: -origin %q is SSH, and the push credential serves HTTPS only; "+
			"give the repository's https URL.", origin)
	}
	if hasScheme && scheme != "https" && scheme != "file" {
		return fmt.Errorf("ubiq serve: -origin %q is not HTTPS; give the repository's https URL, "+
			"or a local path in development.", origin)
	}
	return nil
}

// identity reads a flag's "Name <email>".
func identity(name, value string) (*mail.Address, error) {
	a, err := mail.ParseAddress(value)
	if err != nil || a.Name == "" {
		return nil, fmt.Errorf("ubiq serve: -%s %q is not \"Name <email>\".", name, value)
	}
	return a, nil
}
