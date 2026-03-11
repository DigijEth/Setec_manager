package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"setec-manager/internal/deploy"
)

type autarchStatus struct {
	Installed   bool   `json:"installed"`
	InstallDir  string `json:"install_dir"`
	GitCommit   string `json:"git_commit"`
	VenvReady   bool   `json:"venv_ready"`
	PipPackages int    `json:"pip_packages"`
	WebRunning  bool   `json:"web_running"`
	WebStatus   string `json:"web_status"`
	DNSRunning  bool   `json:"dns_running"`
	DNSStatus   string `json:"dns_status"`
}

func (h *Handler) AutarchStatus(w http.ResponseWriter, r *http.Request) {
	status := h.getAutarchStatus()
	h.render(w, "autarch.html", status)
}

func (h *Handler) AutarchStatusAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.getAutarchStatus())
}

func (h *Handler) getAutarchStatus() autarchStatus {
	dir := h.Config.Autarch.InstallDir
	status := autarchStatus{InstallDir: dir}

	// Check if installed
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		status.Installed = true
	}

	// Git commit
	if hash, message, err := deploy.CurrentCommit(dir); err == nil {
		status.GitCommit = hash + " " + message
	}

	// Venv
	status.VenvReady = deploy.VenvExists(dir)

	// Pip packages
	venvDir := filepath.Join(dir, "venv")
	if pkgs, err := deploy.ListPackages(venvDir); err == nil {
		status.PipPackages = len(pkgs)
	}

	// Web service
	webActive, _ := deploy.IsActive("autarch-web")
	status.WebRunning = webActive
	if webActive {
		status.WebStatus = "active"
	} else {
		status.WebStatus = "inactive"
	}

	// DNS service
	dnsActive, _ := deploy.IsActive("autarch-dns")
	status.DNSRunning = dnsActive
	if dnsActive {
		status.DNSStatus = "active"
	} else {
		status.DNSStatus = "inactive"
	}

	return status
}

func (h *Handler) AutarchInstall(w http.ResponseWriter, r *http.Request) {
	dir := h.Config.Autarch.InstallDir
	repo := h.Config.Autarch.GitRepo
	branch := h.Config.Autarch.GitBranch

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		writeError(w, http.StatusConflict, "AUTARCH already installed at "+dir)
		return
	}

	depID, _ := h.DB.CreateDeployment(nil, "autarch_install")
	var output strings.Builder

	steps := []struct {
		label string
		fn    func() error
	}{
		{"Clone from GitHub", func() error {
			os.MkdirAll(filepath.Dir(dir), 0755)
			out, err := deploy.Clone(repo, branch, dir)
			output.WriteString(out)
			return err
		}},
		{"Create Python venv", func() error {
			return deploy.CreateVenv(dir)
		}},
		{"Upgrade pip", func() error {
			venvDir := filepath.Join(dir, "venv")
			deploy.UpgradePip(venvDir)
			return nil
		}},
		{"Install pip packages", func() error {
			reqFile := filepath.Join(dir, "requirements.txt")
			if _, err := os.Stat(reqFile); err != nil {
				return nil
			}
			venvDir := filepath.Join(dir, "venv")
			out, err := deploy.InstallRequirements(venvDir, reqFile)
			output.WriteString(out)
			return err
		}},
		{"Install npm packages", func() error {
			out, _ := deploy.NpmInstall(dir)
			output.WriteString(out)
			return nil
		}},
		{"Set permissions", func() error {
			exec.Command("chown", "-R", "root:root", dir).Run()
			exec.Command("chmod", "-R", "755", dir).Run()
			for _, d := range []string{"data", "data/certs", "data/dns", "results", "dossiers", "models"} {
				os.MkdirAll(filepath.Join(dir, d), 0755)
			}
			confPath := filepath.Join(dir, "autarch_settings.conf")
			if _, err := os.Stat(confPath); err == nil {
				exec.Command("chmod", "600", confPath).Run()
			}
			return nil
		}},
		{"Install systemd units", func() error {
			h.installAutarchUnits(dir)
			return nil
		}},
	}

	for _, step := range steps {
		output.WriteString(fmt.Sprintf("\n=== %s ===\n", step.label))
		if err := step.fn(); err != nil {
			h.DB.FinishDeployment(depID, "failed", output.String())
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("%s failed: %v", step.label, err))
			return
		}
	}

	h.DB.FinishDeployment(depID, "success", output.String())
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}

func (h *Handler) AutarchUpdate(w http.ResponseWriter, r *http.Request) {
	dir := h.Config.Autarch.InstallDir

	depID, _ := h.DB.CreateDeployment(nil, "autarch_update")
	var output strings.Builder

	// Git pull
	out, err := deploy.Pull(dir)
	output.WriteString(out)
	if err != nil {
		h.DB.FinishDeployment(depID, "failed", output.String())
		writeError(w, http.StatusInternalServerError, "git pull failed")
		return
	}

	// Reinstall pip packages
	reqFile := filepath.Join(dir, "requirements.txt")
	if _, err := os.Stat(reqFile); err == nil {
		venvDir := filepath.Join(dir, "venv")
		pipOut, _ := deploy.InstallRequirements(venvDir, reqFile)
		output.WriteString(pipOut)
	}

	// Restart services
	deploy.Restart("autarch-web")
	deploy.Restart("autarch-dns")

	h.DB.FinishDeployment(depID, "success", output.String())
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) AutarchStart(w http.ResponseWriter, r *http.Request) {
	deploy.Start("autarch-web")
	deploy.Start("autarch-dns")
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handler) AutarchStop(w http.ResponseWriter, r *http.Request) {
	deploy.Stop("autarch-web")
	deploy.Stop("autarch-dns")
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) AutarchRestart(w http.ResponseWriter, r *http.Request) {
	deploy.Restart("autarch-web")
	deploy.Restart("autarch-dns")
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (h *Handler) AutarchConfig(w http.ResponseWriter, r *http.Request) {
	confPath := filepath.Join(h.Config.Autarch.InstallDir, "autarch_settings.conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "config not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"config": string(data)})
}

func (h *Handler) AutarchConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	confPath := filepath.Join(h.Config.Autarch.InstallDir, "autarch_settings.conf")
	if err := os.WriteFile(confPath, []byte(body.Config), 0600); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (h *Handler) AutarchDNSBuild(w http.ResponseWriter, r *http.Request) {
	dnsDir := filepath.Join(h.Config.Autarch.InstallDir, "services", "dns-server")

	depID, _ := h.DB.CreateDeployment(nil, "dns_build")

	cmd := exec.Command("go", "build", "-o", "autarch-dns", ".")
	cmd.Dir = dnsDir
	out, err := cmd.CombinedOutput()

	if err != nil {
		h.DB.FinishDeployment(depID, "failed", string(out))
		writeError(w, http.StatusInternalServerError, "build failed: "+string(out))
		return
	}

	h.DB.FinishDeployment(depID, "success", string(out))
	writeJSON(w, http.StatusOK, map[string]string{"status": "built"})
}

func (h *Handler) installAutarchUnits(dir string) {
	webUnit := deploy.GenerateUnit(deploy.UnitConfig{
		Name:             "autarch-web",
		Description:      "AUTARCH Web Dashboard",
		ExecStart:        filepath.Join(dir, "venv", "bin", "python3") + " " + filepath.Join(dir, "autarch_web.py"),
		WorkingDirectory: dir,
		User:             "root",
		Environment:      map[string]string{"PYTHONUNBUFFERED": "1"},
	})

	dnsUnit := deploy.GenerateUnit(deploy.UnitConfig{
		Name:             "autarch-dns",
		Description:      "AUTARCH DNS Server",
		ExecStart:        filepath.Join(dir, "services", "dns-server", "autarch-dns") + " --config " + filepath.Join(dir, "data", "dns", "config.json"),
		WorkingDirectory: dir,
		User:             "root",
	})

	deploy.InstallUnit("autarch-web", webUnit)
	deploy.InstallUnit("autarch-dns", dnsUnit)
}
