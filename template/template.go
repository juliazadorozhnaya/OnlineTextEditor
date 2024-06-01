package template

import (
	"html/template"
	"net/http"
	"path/filepath"
)

// TemplateStore holds all parsed static
type TemplateStore struct {
	templates *template.Template
}

// NewTemplateStore creates a new TemplateStore that loads static from the specified base directory.
func NewTemplateStore(basePath string) (*TemplateStore, error) {
	// Parse all static in the given directory
	templates, err := template.ParseGlob(filepath.Join(basePath, "*.html"))
	if err != nil {
		return nil, err
	}
	return &TemplateStore{templates: templates}, nil
}

// Render executes a specific template with the provided data
func (ts *TemplateStore) Render(w http.ResponseWriter, tmpl string, data interface{}) error {
	// Execute the template with the given data
	return ts.templates.ExecuteTemplate(w, tmpl, data)
}
