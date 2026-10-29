// Command ubiq renders the GLOSSARY.md glossaries, GLOSSARY-MAP.md and docs/adr as one self-contained HTML
// page, and checks free text against the glossary's vocabulary.
//
//	ubiq html [-out glossary.html]
//	ubiq check [file ...]
//	ubiq health
//
// check reads stdin when no file is given. It lists the terms it recognised and the Capitalised Phrases
// that have no entry. health prints the glossary health report as one "path:line: rule: name: note" line per
// finding and exits 1 when there is any. Every command exits 2 on a usage or read error.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed page.html
var page string

const usage = "usage: ubiq html [-root dir] [-out file] | ubiq check [-root dir] [file ...] | " +
	"ubiq health [-root dir]"

// errHealthFindings reports that the health command found something; main exits 1 for it.
var errHealthFindings = errors.New("ubiq health: findings")

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, errHealthFindings) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "html":
		return runHTML(args[1:], stderr)
	case "check":
		return runCheck(args[1:], stdin, stdout)
	case "health":
		return runHealth(args[1:], stdout, stderr)
	}
	return fmt.Errorf("unknown command %q\n%s", args[0], usage)
}

func runHTML(args []string, status io.Writer) error {
	fs := flag.NewFlagSet("html", flag.ContinueOnError)
	fs.SetOutput(status)
	root := fs.String("root", ".", "repository root holding GLOSSARY.md, GLOSSARY-MAP.md and docs/adr")
	out := fs.String("out", "glossary.html", "output html path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	model, err := LoadModel(*root)
	if err != nil {
		return err
	}
	html, err := RenderPage(model)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(status, "wrote %s: %d terms, %d notes, %d map entries, %d ADRs, %d unresolved phrases\n",
		*out, len(model.Terms), len(model.Notes), len(model.MapEntries), len(model.ADRs), len(model.Unresolved))
	return nil
}

// RenderPage embeds the model as JSON in the page template.
func RenderPage(model *Model) (string, error) {
	data, err := json.Marshal(model)
	if err != nil {
		return "", err
	}
	// keep </script> out of the embedded JSON
	js := strings.ReplaceAll(string(data), "</", `<\/`)
	return strings.Replace(page, "/*__DATA__*/null", js, 1), nil
}

func runCheck(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stdout)
	root := fs.String("root", ".", "repository root holding GLOSSARY.md, GLOSSARY-MAP.md and docs/adr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	model, err := LoadModel(*root)
	if err != nil {
		return err
	}
	text, err := readInput(fs.Args(), stdin)
	if err != nil {
		return err
	}
	NewChecker(model).Check(text).Write(stdout)
	return nil
}

// readInput concatenates the named files, or reads stdin when none are named.
func readInput(paths []string, stdin io.Reader) (string, error) {
	if len(paths) == 0 {
		b, err := io.ReadAll(stdin)
		return string(b), err
	}
	var sb strings.Builder
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		sb.Write(b)
		if !strings.HasSuffix(string(b), "\n") {
			sb.WriteByte('\n')
		}
	}
	return sb.String(), nil
}

// runHealth writes the findings to stdout and a per-rule summary to status.
func runHealth(args []string, stdout, status io.Writer) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(status)
	root := fs.String("root", ".", "repository root holding GLOSSARY.md, GLOSSARY-MAP.md and docs/adr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	model, err := LoadModel(*root)
	if err != nil {
		return err
	}
	findings := Health(model)
	WriteHealth(stdout, findings)
	fmt.Fprintln(status, healthSummary(findings, len(model.Terms)))
	if len(findings) > 0 {
		return errHealthFindings
	}
	return nil
}
