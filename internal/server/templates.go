package server

import (
	"html/template"
	"io"
	"log"
	"net/http"
	"sync"

	"setec-manager/web"
)

var (
	tmplOnce sync.Once
	tmpl     *template.Template
)

func (s *Server) getTemplates() *template.Template {
	tmplOnce.Do(func() {
		funcMap := template.FuncMap{
			"eq":      func(a, b interface{}) bool { return a == b },
			"ne":      func(a, b interface{}) bool { return a != b },
			"default": func(val, def interface{}) interface{} {
				if val == nil || val == "" || val == 0 || val == false {
					return def
				}
				return val
			},
		}

		var err error
		tmpl, err = template.New("").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/*.html")
		if err != nil {
			log.Fatalf("Failed to parse templates: %v", err)
		}
	})
	return tmpl
}

type templateData struct {
	Title   string
	Claims  *Claims
	Data    interface{}
	Flash   string
	Config  interface{}
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	td := templateData{
		Data:   data,
		Config: s.Config,
	}

	t := s.getTemplates().Lookup(name)
	if t == nil {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, td); err != nil {
		log.Printf("Template render error (%s): %v", name, err)
	}
}

func (s *Server) renderTemplateWithClaims(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	td := templateData{
		Claims: getClaimsFromContext(r.Context()),
		Data:   data,
		Config: s.Config,
	}

	t := s.getTemplates().Lookup(name)
	if t == nil {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, td); err != nil {
		log.Printf("Template render error (%s): %v", name, err)
	}
}

// renderError sends an error response - HTML for browsers, JSON for API calls.
func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if acceptsHTML(r) {
		w.WriteHeader(status)
		io.WriteString(w, message)
		return
	}
	writeJSON(w, status, map[string]string{"error": message})
}
