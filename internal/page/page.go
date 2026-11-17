// Package page renders the glossary model as one self-contained HTML page.
package page

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
	"github.com/stefaneg/ubiq/internal/person"
)

//go:embed page.html
var template string

// shown is how the page shows the Person it is served to: by name, or by email without one, and role.
type shown struct {
	Name string      `json:"name"`
	Role person.Role `json:"role"`
}

// Render embeds the model and its health findings as JSON in the page template.
func Render(model *glossary.Model) (string, error) { return render(model, nil) }

// RenderFor is Render with the name and role of p, the Person the page is served to, at the top of the page.
func RenderFor(model *glossary.Model, p person.Person) (string, error) {
	name := p.Name
	if name == "" {
		name = p.Email
	}
	return render(model, &shown{Name: name, Role: p.Role})
}

func render(model *glossary.Model, p *shown) (string, error) {
	data, err := json.Marshal(struct {
		*glossary.Model
		Health []health.Finding `json:"health"`
		Person *shown           `json:"person,omitempty"`
	}{model, health.Check(model), p})
	if err != nil {
		return "", err
	}
	// keep </script> out of the embedded JSON
	js := strings.ReplaceAll(string(data), "</", `<\/`)
	return strings.Replace(template, "/*__DATA__*/null", js, 1), nil
}
