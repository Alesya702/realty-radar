// Package web is the UI shell: shared layout, static assets and rendering
// helpers for server-rendered pages. Business pages belong to module HTTP
// adapters; this package must not import business modules.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
)

//go:embed templates
var templateFS embed.FS

// Link is a navigation item of the UI shell.
type Link struct {
	Title string
	Href  string
}

// Page is the data passed to the layout. Page templates receive it as dot
// and read their own data from .Data.
type Page struct {
	Title string
	Nav   []Link
	// Error is a user-facing message shown above the page content.
	Error string
	Data  any
}

// Renderer renders pages inside the shared layout. Each page template must
// define the "content" template.
type Renderer struct {
	pages map[string]*template.Template
	nav   []Link
}

// NewRenderer parses every template matching pattern in pages, each combined
// with the shared layout and funcs.
func NewRenderer(pages fs.FS, pattern string, funcs template.FuncMap, nav []Link) (*Renderer, error) {
	base, err := template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	files, err := fs.Glob(pages, pattern)
	if err != nil {
		return nil, fmt.Errorf("list page templates: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no page templates match %q", pattern)
	}
	r := &Renderer{pages: make(map[string]*template.Template, len(files)), nav: nav}
	for _, file := range files {
		t, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone layout: %w", err)
		}
		if _, err := t.ParseFS(pages, file); err != nil {
			return nil, fmt.Errorf("parse page %s: %w", file, err)
		}
		r.pages[path.Base(file)] = t
	}
	return r, nil
}

// Render writes page name with the given status. The page is executed into a
// buffer first so that a template error never produces a half-written page.
func (r *Renderer) Render(w http.ResponseWriter, status int, name string, p Page) {
	t, ok := r.pages[name]
	if !ok {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	p.Nav = r.nav
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", p); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// Info is technical information shown on the start page.
type Info struct {
	Version string
	Env     string
	// Nav lists the module pages available in the UI.
	Nav []Link
}

// Register registers the UI shell routes.
func Register(mux *http.ServeMux, info Info) {
	pages, err := fs.Sub(templateFS, "templates/pages")
	if err != nil {
		panic(err)
	}
	renderer, err := NewRenderer(pages, "*.html", nil, info.Nav)
	if err != nil {
		// Templates are embedded: a parse error is a programming error.
		panic(err)
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		renderer.Render(w, http.StatusOK, "index.html", Page{Title: "Realty Radar", Data: info})
	})
}
