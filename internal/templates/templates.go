package templates

import (
	"embed"
	"html/template"
)

//go:embed templates/*
var resources embed.FS

var TemplateExecutor = template.Must(template.ParseFS(resources, "templates/*"))
