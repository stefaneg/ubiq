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

// Render embeds the model and its health findings as JSON in the page template.
func Render(model *glossary.Model) (string, error) {
	data, err := json.Marshal(struct {
		*glossary.Model
		Health []health.Finding `json:"health"`
	}{model, health.Check(model)})
	if err != nil {
		return "", err
	}
	// keep </script> out of the embedded JSON
	js := strings.ReplaceAll(string(data), "</", `<\/`)
	return strings.Replace(template, "/*__DATA__*/null", js, 1), nil
}
