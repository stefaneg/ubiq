// Package page renders the glossary model as one self-contained HTML page.
package page

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/health"
)

//go:embed page.html
var template string

// Viewer is the Person a served page is shown to, by name and role.
type Viewer struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// Render embeds the model and its health findings as JSON in the page template.
func Render(model *glossary.Model) (string, error) { return render(model, nil) }

// RenderFor is Render with the viewer's name and role at the top of the page.
func RenderFor(model *glossary.Model, v Viewer) (string, error) { return render(model, &v) }

func render(model *glossary.Model, v *Viewer) (string, error) {
	data, err := json.Marshal(struct {
		*glossary.Model
		Health []health.Finding `json:"health"`
		Person *Viewer          `json:"person,omitempty"`
	}{model, health.Check(model), v})
	if err != nil {
		return "", err
	}
	// keep </script> out of the embedded JSON
	js := strings.ReplaceAll(string(data), "</", `<\/`)
	return strings.Replace(template, "/*__DATA__*/null", js, 1), nil
}
