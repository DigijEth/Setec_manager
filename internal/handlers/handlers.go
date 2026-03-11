package handlers

import (
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"sync"

	"setec-manager/internal/config"
	"setec-manager/internal/db"
	"setec-manager/internal/gitea"
	"setec-manager/internal/hosting"
	"setec-manager/web"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Config         *config.Config
	DB             *db.DB
	HostingConfigs *hosting.ProviderConfigStore
	GiteaClient    *gitea.Client
	tmpl           *template.Template
	once           sync.Once
}

func New(cfg *config.Config, database *db.DB, hostingConfigs *hosting.ProviderConfigStore) *Handler {
	h := &Handler{
		Config:         cfg,
		DB:             database,
		HostingConfigs: hostingConfigs,
	}
	// Initialize Gitea client if configured
	if cfg.Gitea.Installed && cfg.Gitea.BaseURL != "" && cfg.Gitea.AdminToken != "" {
		h.GiteaClient = gitea.New(cfg.Gitea.BaseURL, cfg.Gitea.AdminToken)
	}
	return h
}

func (h *Handler) getTemplates() *template.Template {
	h.once.Do(func() {
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
		h.tmpl, err = template.New("").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/*.html")
		if err != nil {
			log.Fatalf("Failed to parse templates: %v", err)
		}

		// Also parse from the static FS to make sure it's available
		_ = fs.WalkDir(web.StaticFS, ".", func(path string, d fs.DirEntry, err error) error {
			return nil
		})
	})
	return h.tmpl
}

type pageData struct {
	Title  string
	Data   interface{}
	Config *config.Config
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	pd := pageData{
		Data:   data,
		Config: h.Config,
	}

	t := h.getTemplates().Lookup(name)
	if t == nil {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, pd); err != nil {
		log.Printf("[template] %s: %v", name, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func paramInt(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func paramStr(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}
