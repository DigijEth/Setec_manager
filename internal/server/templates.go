package server

import (
	"html/template"
	"io"
	"log"
	"net/http"

	"setec-manager/web"
)

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

	// Login page is standalone (full HTML doc), not wrapped in base.html
	t, err := template.New(name).ParseFS(web.TemplateFS, "templates/"+name)
	if err != nil {
		log.Printf("Template parse error (%s): %v", name, err)
		http.Error(w, "Template error", http.StatusInternalServerError)
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

	t, err := template.New("base.html").ParseFS(web.TemplateFS, "templates/base.html", "templates/"+name)
	if err != nil {
		log.Printf("Template parse error (%s): %v", name, err)
		http.Error(w, "Template error", http.StatusInternalServerError)
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
