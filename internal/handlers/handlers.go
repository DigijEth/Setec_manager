package handlers

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"

	"setec-manager/internal/config"
	"setec-manager/internal/db"
	"setec-manager/internal/docker"
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
	DockerClient   *docker.Client
	DockerDeployer *docker.Deployer
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
	// Initialize Docker client (always available; checks installation at runtime)
	h.DockerClient = docker.New()
	stateDir := filepath.Dir(cfg.Database.Path)
	h.DockerDeployer = docker.NewDeployer(h.DockerClient, stateDir, cfg.Nginx.SitesAvailable)
	return h
}

func (h *Handler) getFuncMap() template.FuncMap {
	return template.FuncMap{
		"eq":      func(a, b interface{}) bool { return a == b },
		"ne":      func(a, b interface{}) bool { return a != b },
		"default": func(val, def interface{}) interface{} {
			if val == nil || val == "" || val == 0 || val == false {
				return def
			}
			return val
		},
	}
}

type pageData struct {
	Title  string
	Claims interface{}
	Data   interface{}
	Flash  string
	Config *config.Config
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	pd := pageData{
		Data:   data,
		Config: h.Config,
	}

	// Parse base.html + the specific page template as a pair.
	// Each page template defines {{define "content"}}...{{end}} and
	// base.html calls {{template "content" .}} to render it.
	// We parse them together so there's exactly one "content" definition.
	t, err := template.New("base.html").Funcs(h.getFuncMap()).ParseFS(
		web.TemplateFS, "templates/base.html", "templates/"+name,
	)
	if err != nil {
		log.Printf("[template] parse %s: %v", name, err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, pd); err != nil {
		log.Printf("[template] render %s: %v (data type: %T)", name, err, data)
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
