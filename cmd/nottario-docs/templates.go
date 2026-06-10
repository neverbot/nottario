package main

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// docsTemplates is the parsed template set shared by every rendered
// page. Parsed once at startup; cheap to execute per page.
var docsTemplates = mustParseTemplates()

func mustParseTemplates() *template.Template {
	t, err := template.New("docs").Funcs(template.FuncMap{
		"base": withBase,
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		panic(fmt.Sprintf("parse templates: %v", err))
	}
	return t
}
