package handlers

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"setec-manager/internal/nginx"
	"setec-manager/internal/system"
)

type nginxStatus struct {
	Running    bool   `json:"running"`
	Status     string `json:"status"`
	ConfigTest string `json:"config_test"`
	ConfigOK   bool   `json:"config_ok"`
}

func (h *Handler) NginxStatus(w http.ResponseWriter, r *http.Request) {
	status := nginxStatus{}
	status.Status, status.Running = nginx.Status()

	testOut, testErr := nginx.Test()
	status.ConfigTest = testOut
	status.ConfigOK = testErr == nil

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, status)
		return
	}
	h.render(w, "nginx.html", status)
}

func (h *Handler) NginxReload(w http.ResponseWriter, r *http.Request) {
	// Validate config first
	if _, err := nginx.Test(); err != nil {
		writeError(w, http.StatusBadRequest, "nginx config test failed — not reloading")
		return
	}
	if err := nginx.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

func (h *Handler) NginxRestart(w http.ResponseWriter, r *http.Request) {
	if err := nginx.Restart(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (h *Handler) NginxConfigView(w http.ResponseWriter, r *http.Request) {
	domain := paramStr(r, "domain")
	path := filepath.Join(h.Config.Nginx.SitesAvailable, domain)

	data, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "config not found for "+domain)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"domain": domain, "config": string(data)})
}

func (h *Handler) NginxTest(w http.ResponseWriter, r *http.Request) {
	out, err := nginx.Test()
	ok := err == nil
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"output": strings.TrimSpace(out),
		"valid":  ok,
	})
}

func (h *Handler) NginxInstallBase(w http.ResponseWriter, r *http.Request) {
	// Install nginx if not present
	if _, err := exec.LookPath("nginx"); err != nil {
		if _, installErr := system.PackageInstall("nginx"); installErr != nil {
			writeError(w, http.StatusInternalServerError, installErr.Error())
			return
		}
	}

	// Install snippets
	if err := nginx.InstallSnippets(h.Config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Ensure certbot webroot exists
	os.MkdirAll(h.Config.Nginx.CertbotWebroot, 0755)

	writeJSON(w, http.StatusOK, map[string]string{"status": "nginx configured"})
}
