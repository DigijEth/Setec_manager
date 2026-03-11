package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"setec-manager/internal/gitea"
)

// ── Helpers ─────────────────────────────────────────────────────────

func (h *Handler) giteaInstalled() bool {
	return h.Config.Gitea.Installed && h.GiteaClient != nil
}

func (h *Handler) requireGitea(w http.ResponseWriter) bool {
	if !h.giteaInstalled() {
		writeError(w, http.StatusServiceUnavailable, "Gitea is not installed")
		return false
	}
	return true
}

// ── Setup / Status ──────────────────────────────────────────────────

// GitSetup renders the git management page (setup wizard or management UI).
func (h *Handler) GitSetup(w http.ResponseWriter, r *http.Request) {
	sites, _ := h.DB.ListSites()
	var domains []string
	for _, s := range sites {
		domains = append(domains, s.Domain)
	}
	data := map[string]interface{}{
		"Installed": h.giteaInstalled(),
		"Domains":   domains,
		"Config":    h.Config.Gitea,
	}
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, data)
		return
	}
	h.render(w, "git.html", data)
}

// GitSetupWizard renders the setup wizard page.
func (h *Handler) GitSetupWizard(w http.ResponseWriter, r *http.Request) {
	sites, _ := h.DB.ListSites()
	var domains []string
	for _, s := range sites {
		domains = append(domains, s.Domain)
	}
	h.render(w, "git.html", map[string]interface{}{
		"Installed": false,
		"Domains":   domains,
		"Config":    h.Config.Gitea,
	})
}

// GitInstall installs Gitea with the provided configuration.
func (h *Handler) GitInstall(w http.ResponseWriter, r *http.Request) {
	if h.giteaInstalled() {
		writeError(w, http.StatusConflict, "Gitea is already installed")
		return
	}

	var body struct {
		URLStyle      string `json:"url_style"`
		Subdomain     string `json:"subdomain"`
		Domain        string `json:"domain"`
		SSHPort       int    `json:"ssh_port"`
		AdminUsername string `json:"admin_username"`
		AdminPassword string `json:"admin_password"`
		AdminEmail    string `json:"admin_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Domain == "" || body.AdminUsername == "" || body.AdminPassword == "" || body.AdminEmail == "" {
		writeError(w, http.StatusBadRequest, "domain, admin_username, admin_password, and admin_email are required")
		return
	}
	if body.URLStyle == "" {
		body.URLStyle = "subdomain"
	}
	if body.Subdomain == "" {
		body.Subdomain = "git"
	}
	if body.SSHPort == 0 {
		body.SSHPort = 2222
	}

	// Determine the Gitea URL
	var giteaURL string
	var serverDomain string
	if body.URLStyle == "subdomain" {
		serverDomain = body.Subdomain + "." + body.Domain
		giteaURL = "https://" + serverDomain
	} else {
		serverDomain = body.Domain
		giteaURL = "https://" + body.Domain + "/" + body.Subdomain
	}

	dataDir := h.Config.Gitea.DataDir
	binaryPath := h.Config.Gitea.BinaryPath

	// Step 1: Create directories
	for _, dir := range []string{dataDir, filepath.Join(dataDir, "custom/conf"), filepath.Join(dataDir, "data"),
		filepath.Join(dataDir, "log"), filepath.Join(dataDir, "repositories")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("create directory %s: %v", dir, err))
			return
		}
	}

	// Step 2: Download Gitea binary if not present
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "amd64"
		}
		downloadURL := fmt.Sprintf("https://dl.gitea.com/gitea/1.22.6/gitea-1.22.6-linux-%s", arch)

		resp, err := http.Get(downloadURL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("download gitea: %v", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("download gitea: HTTP %d", resp.StatusCode))
			return
		}

		os.MkdirAll(filepath.Dir(binaryPath), 0755)
		f, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write binary: %v", err))
			return
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			f.Close()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write binary: %v", err))
			return
		}
		f.Close()
	}

	// Step 3: Write app.ini
	appIniPath := filepath.Join(dataDir, "custom/conf/app.ini")
	appIni := fmt.Sprintf(`[server]
APP_DATA_PATH    = %s/data
DOMAIN           = %s
ROOT_URL         = %s/
HTTP_PORT        = 3000
SSH_DOMAIN       = %s
SSH_PORT         = %d
START_SSH_SERVER = true
LFS_START_SERVER = true

[database]
DB_TYPE  = sqlite3
PATH     = %s/data/gitea.db

[repository]
ROOT = %s/repositories

[log]
ROOT_PATH = %s/log

[security]
INSTALL_LOCK = true
SECRET_KEY   = auto

[service]
DISABLE_REGISTRATION = true
REQUIRE_SIGNIN_VIEW  = true
`,
		dataDir, serverDomain, giteaURL, serverDomain, body.SSHPort,
		dataDir, dataDir, dataDir)

	if err := os.WriteFile(appIniPath, []byte(appIni), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write app.ini: %v", err))
		return
	}

	// Step 4: Create systemd unit
	unitContent := fmt.Sprintf(`[Unit]
Description=Gitea (Git with a cup of tea)
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s web --config %s
Restart=always
Environment=GITEA_WORK_DIR=%s

[Install]
WantedBy=multi-user.target
`, dataDir, binaryPath, appIniPath, dataDir)

	unitPath := "/etc/systemd/system/gitea.service"
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write systemd unit: %v", err))
		return
	}

	// Reload systemd and start Gitea
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "gitea").Run()
	if err := exec.Command("systemctl", "start", "gitea").Run(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("start gitea: %v", err))
		return
	}

	// Wait for Gitea to become ready
	localURL := "http://localhost:3000"
	ready := false
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		resp, err := http.Get(localURL + "/api/v1/version")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
	}
	if !ready {
		writeError(w, http.StatusInternalServerError, "Gitea did not start within 30 seconds")
		return
	}

	// Step 5: Create admin user via CLI
	cmd := exec.Command(binaryPath, "admin", "user", "create",
		"--config", appIniPath,
		"--username", body.AdminUsername,
		"--password", body.AdminPassword,
		"--email", body.AdminEmail,
		"--admin",
	)
	cmd.Dir = dataDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// If user already exists, that's ok
		if !strings.Contains(string(out), "already exists") {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("create admin user: %s", string(out)))
			return
		}
	}

	// Step 6: Generate API token
	tmpClient := gitea.New(localURL, "")
	token, err := tmpClient.CreateToken(body.AdminUsername, body.AdminPassword, "setec-manager")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create API token: %v", err))
		return
	}

	// Step 7: Generate Nginx config
	var nginxConf string
	if body.URLStyle == "subdomain" {
		nginxConf = fmt.Sprintf(`server {
    listen 80;
    server_name %s;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        client_max_body_size 100m;
    }
}
`, serverDomain)
	} else {
		nginxConf = fmt.Sprintf(`location /%s/ {
    proxy_pass http://127.0.0.1:3000/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    client_max_body_size 100m;
}
`, body.Subdomain)
	}

	nginxConfPath := filepath.Join(h.Config.Nginx.SitesAvailable, "gitea")
	if err := os.WriteFile(nginxConfPath, []byte(nginxConf), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write nginx config: %v", err))
		return
	}

	// Enable the nginx site
	enabledPath := filepath.Join(h.Config.Nginx.SitesEnabled, "gitea")
	os.Remove(enabledPath)
	os.Symlink(nginxConfPath, enabledPath)
	exec.Command("nginx", "-s", "reload").Run()

	// Step 8: Save config
	h.Config.Gitea.Installed = true
	h.Config.Gitea.BaseURL = localURL
	h.Config.Gitea.AdminToken = token
	h.Config.Gitea.Domain = serverDomain
	h.Config.Gitea.URLStyle = body.URLStyle
	h.Config.Gitea.SSHPort = body.SSHPort

	cfgPath := filepath.Join(filepath.Dir(h.Config.Database.Path), "..", "config.yaml")
	h.Config.Save(cfgPath)

	// Initialize the Gitea client
	h.GiteaClient = gitea.New(localURL, token)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "installed",
		"url":     giteaURL,
		"message": "Gitea installed successfully",
	})
}

// GitStatus returns installation status as JSON.
func (h *Handler) GitStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"installed": h.Config.Gitea.Installed,
	}

	if h.giteaInstalled() {
		version, err := h.GiteaClient.GetVersion()
		if err != nil {
			status["running"] = false
			status["error"] = err.Error()
		} else {
			status["running"] = true
			status["version"] = version
		}
		status["url"] = h.Config.Gitea.Domain
		status["url_style"] = h.Config.Gitea.URLStyle
		status["ssh_port"] = h.Config.Gitea.SSHPort

		// Quick stats
		users, _ := h.GiteaClient.ListUsers()
		repos, _ := h.GiteaClient.ListRepos()
		orgs, _ := h.GiteaClient.ListOrgs()
		status["total_users"] = len(users)
		status["total_repos"] = len(repos)
		status["total_orgs"] = len(orgs)

		// Disk usage
		if info, err := dirSize(h.Config.Gitea.DataDir); err == nil {
			status["disk_usage"] = info
		}
	}

	writeJSON(w, http.StatusOK, status)
}

// GitUninstall removes Gitea (keeps data).
func (h *Handler) GitUninstall(w http.ResponseWriter, r *http.Request) {
	if !h.giteaInstalled() {
		writeError(w, http.StatusBadRequest, "Gitea is not installed")
		return
	}

	// Stop and disable service
	exec.Command("systemctl", "stop", "gitea").Run()
	exec.Command("systemctl", "disable", "gitea").Run()
	os.Remove("/etc/systemd/system/gitea.service")
	exec.Command("systemctl", "daemon-reload").Run()

	// Remove nginx config
	os.Remove(filepath.Join(h.Config.Nginx.SitesEnabled, "gitea"))
	os.Remove(filepath.Join(h.Config.Nginx.SitesAvailable, "gitea"))
	exec.Command("nginx", "-s", "reload").Run()

	// Update config (keep data dir intact)
	h.Config.Gitea.Installed = false
	h.Config.Gitea.AdminToken = ""
	h.Config.Gitea.BaseURL = ""
	cfgPath := filepath.Join(filepath.Dir(h.Config.Database.Path), "..", "config.yaml")
	h.Config.Save(cfgPath)

	h.GiteaClient = nil

	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}

// ── Repositories ────────────────────────────────────────────────────

// GitRepos lists all repositories.
func (h *Handler) GitRepos(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	repos, err := h.GiteaClient.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

// GitRepoCreate creates a new repository.
func (h *Handler) GitRepoCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	var body struct {
		Owner       string `json:"owner"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
		AutoInit    bool   `json:"auto_init"`
		License     string `json:"license"`
		Gitignores  string `json:"gitignores"`
		Readme      string `json:"readme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Owner == "" || body.Name == "" {
		writeError(w, http.StatusBadRequest, "owner and name are required")
		return
	}

	opts := gitea.CreateRepoOpts{
		Name:        body.Name,
		Description: body.Description,
		Private:     body.Private,
		AutoInit:    body.AutoInit,
		License:     body.License,
		Gitignores:  body.Gitignores,
		Readme:      body.Readme,
	}

	repo, err := h.GiteaClient.CreateRepo(body.Owner, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

// GitRepoDelete deletes a repository.
func (h *Handler) GitRepoDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	owner := paramStr(r, "owner")
	repo := paramStr(r, "repo")
	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}
	if err := h.GiteaClient.DeleteRepo(owner, repo); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GitRepoGet returns details for a single repository.
func (h *Handler) GitRepoGet(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	owner := paramStr(r, "owner")
	repo := paramStr(r, "repo")
	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}
	result, err := h.GiteaClient.GetRepo(owner, repo)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GitRepoMirror creates a mirror from an external URL.
func (h *Handler) GitRepoMirror(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	var body struct {
		CloneURL    string `json:"clone_url"`
		Name        string `json:"name"`
		Owner       string `json:"owner"`
		Private     bool   `json:"private"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.CloneURL == "" || body.Name == "" || body.Owner == "" {
		writeError(w, http.StatusBadRequest, "clone_url, name, and owner are required")
		return
	}

	// Validate URL
	if _, err := url.ParseRequestURI(body.CloneURL); err != nil {
		writeError(w, http.StatusBadRequest, "invalid clone URL")
		return
	}

	opts := gitea.MirrorRepoOpts{
		CloneAddr:   body.CloneURL,
		RepoName:    body.Name,
		RepoOwner:   body.Owner,
		Private:     body.Private,
		Description: body.Description,
	}

	repo, err := h.GiteaClient.MirrorRepo(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

// ── Gitea Users ─────────────────────────────────────────────────────

// GitUsers lists all Gitea users.
func (h *Handler) GitUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	users, err := h.GiteaClient.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// GitUserCreate creates a new Gitea user.
func (h *Handler) GitUserCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	var body struct {
		Username           string `json:"username"`
		Password           string `json:"password"`
		Email              string `json:"email"`
		FullName           string `json:"full_name"`
		MustChangePassword bool   `json:"must_change_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Username == "" || body.Password == "" || body.Email == "" {
		writeError(w, http.StatusBadRequest, "username, password, and email are required")
		return
	}

	user, err := h.GiteaClient.CreateUser(gitea.CreateUserOpts{
		Username:           body.Username,
		Password:           body.Password,
		Email:              body.Email,
		FullName:           body.FullName,
		MustChangePassword: body.MustChangePassword,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

// GitUserDelete deletes a Gitea user.
func (h *Handler) GitUserDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	username := paramStr(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if err := h.GiteaClient.DeleteUser(username); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GitUserResetPassword resets a Gitea user's password.
func (h *Handler) GitUserResetPassword(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	username := paramStr(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if err := h.GiteaClient.ResetPassword(username, body.Password); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

// GitUserUpdate updates a Gitea user.
func (h *Handler) GitUserUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	username := paramStr(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	var body struct {
		Email    *string `json:"email,omitempty"`
		FullName *string `json:"full_name,omitempty"`
		IsAdmin  *bool   `json:"is_admin,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	opts := gitea.UpdateUserOpts{
		Email:    body.Email,
		FullName: body.FullName,
		IsAdmin:  body.IsAdmin,
	}
	if err := h.GiteaClient.UpdateUser(username, opts); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ── Organizations ───────────────────────────────────────────────────

// GitOrgs lists all Gitea organizations.
func (h *Handler) GitOrgs(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	orgs, err := h.GiteaClient.ListOrgs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

// GitOrgCreate creates a new Gitea organization.
func (h *Handler) GitOrgCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	var body struct {
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.Visibility == "" {
		body.Visibility = "private"
	}
	org, err := h.GiteaClient.CreateOrg(gitea.CreateOrgOpts{
		UserName:    body.Name,
		FullName:    body.FullName,
		Description: body.Description,
		Visibility:  body.Visibility,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

// GitOrgDelete deletes a Gitea organization.
func (h *Handler) GitOrgDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.GiteaClient.DeleteOrg(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GitOrgMembers lists members of a Gitea organization.
func (h *Handler) GitOrgMembers(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	members, err := h.GiteaClient.ListOrgMembers(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// GitOrgAddMember adds a user to a Gitea organization.
func (h *Handler) GitOrgAddMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	name := paramStr(r, "name")
	username := paramStr(r, "username")
	if name == "" || username == "" {
		writeError(w, http.StatusBadRequest, "name and username are required")
		return
	}
	if err := h.GiteaClient.AddOrgMember(name, username); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// GitOrgRemoveMember removes a user from a Gitea organization.
func (h *Handler) GitOrgRemoveMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireGitea(w) {
		return
	}
	name := paramStr(r, "name")
	username := paramStr(r, "username")
	if name == "" || username == "" {
		writeError(w, http.StatusBadRequest, "name and username are required")
		return
	}
	if err := h.GiteaClient.RemoveOrgMember(name, username); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── Utilities ───────────────────────────────────────────────────────

// dirSize calculates the total size of a directory in bytes.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

