package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"setec-manager/internal/db"
	"setec-manager/internal/deploy"
	"setec-manager/internal/nginx"
)

var validDomainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

func isValidDomain(domain string) bool {
	if len(domain) > 253 {
		return false
	}
	if net.ParseIP(domain) != nil {
		return true
	}
	return validDomainRegex.MatchString(domain)
}

func (h *Handler) SiteList(w http.ResponseWriter, r *http.Request) {
	sites, err := h.DB.ListSites()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich with running status
	type siteView struct {
		db.Site
		Running bool   `json:"running"`
		Status  string `json:"status"`
	}

	var views []siteView
	for _, s := range sites {
		sv := siteView{Site: s}
		if s.AppType != "static" && s.AppPort > 0 {
			unitName := fmt.Sprintf("app-%s", s.Domain)
			active, _ := deploy.IsActive(unitName)
			sv.Running = active
			if active {
				sv.Status = "active"
			} else {
				sv.Status = "inactive"
			}
		} else {
			sv.Status = "static"
			sv.Running = s.Enabled
		}
		views = append(views, sv)
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, views)
		return
	}
	h.render(w, "sites.html", views)
}

func (h *Handler) SiteNewForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "site_new.html", nil)
}

func (h *Handler) SiteCreate(w http.ResponseWriter, r *http.Request) {
	var site db.Site
	if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
		// Try form values
		site.Domain = r.FormValue("domain")
		site.Aliases = r.FormValue("aliases")
		site.AppType = r.FormValue("app_type")
		site.AppRoot = r.FormValue("app_root")
		site.GitRepo = r.FormValue("git_repo")
		site.GitBranch = r.FormValue("git_branch")
		site.AppEntry = r.FormValue("app_entry")
	}

	if site.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if !isValidDomain(site.Domain) {
		writeError(w, http.StatusBadRequest, "invalid domain name")
		return
	}
	if site.AppType == "" {
		site.AppType = "static"
	}
	if site.AppRoot == "" {
		site.AppRoot = filepath.Join(h.Config.Nginx.Webroot, site.Domain)
	}
	if site.GitBranch == "" {
		site.GitBranch = "main"
	}
	site.Enabled = true

	// Check for duplicate
	existing, _ := h.DB.GetSiteByDomain(site.Domain)
	if existing != nil {
		writeError(w, http.StatusConflict, "domain already exists")
		return
	}

	// Create directory
	os.MkdirAll(site.AppRoot, 0755)

	// Clone repo if provided
	if site.GitRepo != "" {
		depID, _ := h.DB.CreateDeployment(nil, "clone")
		out, err := deploy.Clone(site.GitRepo, site.GitBranch, site.AppRoot)
		if err != nil {
			h.DB.FinishDeployment(depID, "failed", out)
			writeError(w, http.StatusInternalServerError, "git clone failed: "+out)
			return
		}
		h.DB.FinishDeployment(depID, "success", out)
	}

	// Save to DB
	id, err := h.DB.CreateSite(&site)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	site.ID = id

	// Generate nginx config
	if err := nginx.GenerateConfig(h.Config, &site); err != nil {
		writeError(w, http.StatusInternalServerError, "nginx config: "+err.Error())
		return
	}

	// Enable site
	nginx.EnableSite(h.Config, site.Domain)
	nginx.Reload()

	// Generate systemd unit for non-static apps
	if site.AppType != "static" && site.AppEntry != "" {
		h.generateAppUnit(&site)
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusCreated, site)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/sites/%d", id), http.StatusSeeOther)
}

func (h *Handler) SiteDetail(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	site, err := h.DB.GetSite(id)
	if err != nil || site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	// Get deployment history
	deps, _ := h.DB.ListDeployments(&id, 10)

	data := map[string]interface{}{
		"Site":        site,
		"Deployments": deps,
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, data)
		return
	}
	h.render(w, "site_detail.html", data)
}

func (h *Handler) SiteUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	site, err := h.DB.GetSite(id)
	if err != nil || site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	var update db.Site
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Apply updates
	if update.Domain != "" {
		site.Domain = update.Domain
	}
	site.Aliases = update.Aliases
	if update.AppType != "" {
		site.AppType = update.AppType
	}
	if update.AppPort > 0 {
		site.AppPort = update.AppPort
	}
	site.AppEntry = update.AppEntry
	site.GitRepo = update.GitRepo
	site.GitBranch = update.GitBranch
	site.SSLEnabled = update.SSLEnabled
	site.Enabled = update.Enabled

	if err := h.DB.UpdateSite(site); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Regenerate nginx config
	nginx.GenerateConfig(h.Config, site)
	nginx.Reload()

	writeJSON(w, http.StatusOK, site)
}

func (h *Handler) SiteDelete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	site, err := h.DB.GetSite(id)
	if err != nil || site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	// Disable nginx
	nginx.DisableSite(h.Config, site.Domain)
	nginx.Reload()

	// Stop, disable, and remove the systemd unit
	unitName := fmt.Sprintf("app-%s", site.Domain)
	deploy.RemoveUnit(unitName)

	if err := h.DB.DeleteSite(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) SiteDeploy(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	site, err := h.DB.GetSite(id)
	if err != nil || site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	depID, _ := h.DB.CreateDeployment(&id, "deploy")

	var output strings.Builder

	// Git pull
	if site.GitRepo != "" {
		out, err := deploy.Pull(site.AppRoot)
		output.WriteString(out)
		if err != nil {
			h.DB.FinishDeployment(depID, "failed", output.String())
			writeError(w, http.StatusInternalServerError, "git pull failed")
			return
		}
	}

	// Reinstall deps based on app type
	switch site.AppType {
	case "python", "autarch":
		venvDir := filepath.Join(site.AppRoot, "venv")
		reqFile := filepath.Join(site.AppRoot, "requirements.txt")
		if _, err := os.Stat(reqFile); err == nil {
			out, _ := deploy.InstallRequirements(venvDir, reqFile)
			output.WriteString(out)
		}
	case "node":
		out, _ := deploy.NpmInstall(site.AppRoot)
		output.WriteString(out)
	}

	// Restart service
	unitName := fmt.Sprintf("app-%s", site.Domain)
	deploy.Restart(unitName)

	h.DB.FinishDeployment(depID, "success", output.String())
	writeJSON(w, http.StatusOK, map[string]string{"status": "deployed"})
}

func (h *Handler) SiteRestart(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	site, _ := h.DB.GetSite(id)
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}
	unitName := fmt.Sprintf("app-%s", site.Domain)
	deploy.Restart(unitName)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (h *Handler) SiteStop(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	site, _ := h.DB.GetSite(id)
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}
	unitName := fmt.Sprintf("app-%s", site.Domain)
	deploy.Stop(unitName)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) SiteStart(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	site, _ := h.DB.GetSite(id)
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}
	unitName := fmt.Sprintf("app-%s", site.Domain)
	deploy.Start(unitName)
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handler) SiteLogs(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	site, _ := h.DB.GetSite(id)
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	unitName := fmt.Sprintf("app-%s", site.Domain)
	out, _ := deploy.Logs(unitName, 100)

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{"logs": out})
		return
	}
	h.render(w, "site_detail.html", map[string]interface{}{
		"Site": site,
		"Logs": out,
	})
}

func (h *Handler) SiteLogStream(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	site, _ := h.DB.GetSite(id)
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	unitName := fmt.Sprintf("app-%s", site.Domain)
	streamJournalctl(w, r, unitName)
}

func (h *Handler) generateAppUnit(site *db.Site) {
	var execStart string

	switch site.AppType {
	case "python":
		venvPython := filepath.Join(site.AppRoot, "venv", "bin", "python3")
		execStart = fmt.Sprintf("%s %s", venvPython, filepath.Join(site.AppRoot, site.AppEntry))
	case "node":
		execStart = fmt.Sprintf("/usr/bin/node %s", filepath.Join(site.AppRoot, site.AppEntry))
	case "autarch":
		venvPython := filepath.Join(site.AppRoot, "venv", "bin", "python3")
		execStart = fmt.Sprintf("%s %s", venvPython, filepath.Join(site.AppRoot, "autarch_web.py"))
	default:
		return
	}

	unitName := fmt.Sprintf("app-%s", site.Domain)
	unitContent := deploy.GenerateUnit(deploy.UnitConfig{
		Name:             unitName,
		Description:      fmt.Sprintf("%s (%s)", site.Domain, site.AppType),
		ExecStart:        execStart,
		WorkingDirectory: site.AppRoot,
		User:             "root",
		Environment:      map[string]string{"PYTHONUNBUFFERED": "1"},
	})

	deploy.InstallUnit(unitName, unitContent)
	deploy.Enable(unitName)
}

func acceptsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func streamJournalctl(w http.ResponseWriter, r *http.Request, unit string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	cmd := exec.Command("journalctl", "-u", unit, "-f", "-n", "50", "--no-pager", "-o", "short-iso")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Start()
	defer cmd.Process.Kill()

	buf := make([]byte, 4096)
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			n, err := stdout.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(string(buf[:n])))
				flusher.Flush()
			}
		}
	}
}
