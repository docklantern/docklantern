// Package layout provides the shared HTML application shell used by all UI pages.
package layout

import (
    "embed"
    "html/template"
)

//go:embed shell.html
var files embed.FS

// WithShell installs the shared sidebar and page wrapper templates on a page
// template. Page data must provide ActivePage and MainClass fields.
func WithShell(t *template.Template) (*template.Template, error) {
    markup, err := files.ReadFile("shell.html")
    if err != nil {
        return nil, err
    }
    return t.Parse(string(markup))
}
