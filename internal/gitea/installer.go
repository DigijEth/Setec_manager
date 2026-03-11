package gitea

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"setec-manager/internal/deploy"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// InstallerConfig holds all parameters needed to install and configure Gitea.
type InstallerConfig struct {
	Version     string // e.g. "1.22.6"
	InstallDir  string // e.g. /opt/gitea
	DataDir     string // e.g. /opt/gitea/data
	RunUser     string // e.g. "git"
	HTTPPort    int    // e.g. 3000 (internal, behind Nginx)
	SSHPort     int    // e.g. 2222
	Domain      string // e.g. "git.example.com"
	URLStyle    string // "subdomain" or "subpath"
	ExternalURL string // e.g. "https://git.example.com" or "https://example.com/git"
	DBType      string // "sqlite3"
	DBPath      string // overrides default if set
}

func (ic *InstallerConfig) defaults() {
	if ic.Version == "" {
		ic.Version = "1.22.6"
	}
	if ic.InstallDir == "" {
		ic.InstallDir = "/opt/gitea"
	}
	if ic.DataDir == "" {
		ic.DataDir = filepath.Join(ic.InstallDir, "data")
	}
	if ic.RunUser == "" {
		ic.RunUser = "git"
	}
	if ic.HTTPPort == 0 {
		ic.HTTPPort = 3000
	}
	if ic.SSHPort == 0 {
		ic.SSHPort = 2222
	}
	if ic.DBType == "" {
		ic.DBType = "sqlite3"
	}
	if ic.DBPath == "" {
		ic.DBPath = filepath.Join(ic.DataDir, "gitea.db")
	}
	if ic.URLStyle == "" {
		ic.URLStyle = "subdomain"
	}
}

// binaryPath returns the full path to the gitea binary.
func (ic *InstallerConfig) binaryPath() string {
	return filepath.Join(ic.InstallDir, "gitea")
}

// rootURL returns the ROOT_URL for app.ini based on the URL style.
func (ic *InstallerConfig) rootURL() string {
	if ic.ExternalURL != "" {
		return strings.TrimRight(ic.ExternalURL, "/") + "/"
	}
	if ic.URLStyle == "subpath" {
		return fmt.Sprintf("https://%s/git/", ic.Domain)
	}
	return fmt.Sprintf("https://%s/", ic.Domain)
}

// downloadURL returns the GitHub release download URL for the configured version.
func (ic *InstallerConfig) downloadURL() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	}
	// Gitea release naming: gitea-{version}-linux-{arch}
	return fmt.Sprintf(
		"https://github.com/go-gitea/gitea/releases/download/v%s/gitea-%s-linux-%s",
		ic.Version, ic.Version, arch,
	)
}

// ---------------------------------------------------------------------------
// Install
// ---------------------------------------------------------------------------

// Install downloads the Gitea binary, creates the system user and directory
// structure, writes the app.ini, installs a systemd unit, and starts the
// service.
func Install(cfg InstallerConfig) error {
	cfg.defaults()

	// 1. Create the git system user (ignore error if already exists).
	if err := ensureSystemUser(cfg.RunUser, cfg.InstallDir); err != nil {
		return fmt.Errorf("create system user: %w", err)
	}

	// 2. Create directories.
	dirs := []string{
		cfg.InstallDir,
		cfg.DataDir,
		filepath.Join(cfg.DataDir, "repositories"),
		filepath.Join(cfg.DataDir, "lfs"),
		filepath.Join(cfg.DataDir, "log"),
		filepath.Join(cfg.InstallDir, "custom", "conf"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0750); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// 3. Download the binary.
	if err := downloadBinary(cfg.downloadURL(), cfg.binaryPath()); err != nil {
		return fmt.Errorf("download gitea: %w", err)
	}

	// 4. Write app.ini.
	if err := WriteAppIni(cfg); err != nil {
		return fmt.Errorf("write app.ini: %w", err)
	}

	// 5. Chown everything to the run user.
	if err := chownRecursive(cfg.InstallDir, cfg.RunUser); err != nil {
		return fmt.Errorf("chown install dir: %w", err)
	}

	// 6. Create and install systemd unit.
	unit := deploy.GenerateUnit(deploy.UnitConfig{
		Name:             "gitea",
		Description:      "Gitea — Git with a cup of tea",
		ExecStart:        cfg.binaryPath() + " web --config " + filepath.Join(cfg.InstallDir, "custom", "conf", "app.ini"),
		WorkingDirectory: cfg.InstallDir,
		User:             cfg.RunUser,
		After:            "network.target",
		RestartPolicy:    "always",
		Environment: map[string]string{
			"USER":          cfg.RunUser,
			"HOME":          cfg.InstallDir,
			"GITEA_WORK_DIR": cfg.InstallDir,
		},
	})

	if err := deploy.InstallUnit("gitea", unit); err != nil {
		return fmt.Errorf("install systemd unit: %w", err)
	}

	// 7. Enable and start.
	if err := deploy.Enable("gitea.service"); err != nil {
		return fmt.Errorf("enable gitea: %w", err)
	}
	if err := deploy.Start("gitea.service"); err != nil {
		return fmt.Errorf("start gitea: %w", err)
	}

	return nil
}

// IsInstalled checks whether the Gitea binary exists at the expected path.
func IsInstalled() bool {
	_, err := os.Stat("/opt/gitea/gitea")
	return err == nil
}

// IsInstalledAt checks whether the Gitea binary exists at a specific install dir.
func IsInstalledAt(installDir string) bool {
	_, err := os.Stat(filepath.Join(installDir, "gitea"))
	return err == nil
}

// GetStatus returns the systemd service status for the gitea unit.
func GetStatus() (string, error) {
	return deploy.Status("gitea.service")
}

// GetVersion runs `gitea --version` and returns the output.
func GetVersion(installDir string) (string, error) {
	if installDir == "" {
		installDir = "/opt/gitea"
	}
	bin := filepath.Join(installDir, "gitea")
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gitea --version: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Uninstall stops the service, removes the systemd unit, and deletes the
// binary.  Data directories are intentionally preserved.
func Uninstall(installDir string) error {
	if installDir == "" {
		installDir = "/opt/gitea"
	}

	// Stop and remove systemd unit.
	if err := deploy.RemoveUnit("gitea"); err != nil {
		return fmt.Errorf("remove systemd unit: %w", err)
	}

	// Remove binary only — keep data.
	bin := filepath.Join(installDir, "gitea")
	if err := os.Remove(bin); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove binary: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// app.ini generation
// ---------------------------------------------------------------------------

const appIniTemplate = `; Gitea configuration — managed by Setec Manager
; https://docs.gitea.com/administration/config-cheat-sheet

[database]
DB_TYPE  = {{.DBType}}
PATH     = {{.DBPath}}

[server]
HTTP_ADDR        = 127.0.0.1
HTTP_PORT        = {{.HTTPPort}}
DOMAIN           = {{.Domain}}
ROOT_URL         = {{.RootURL}}
DISABLE_SSH      = false
SSH_PORT         = {{.SSHPort}}
START_SSH_SERVER = true
LFS_START_SERVER = true
LFS_JWT_SECRET   =
OFFLINE_MODE     = false

[repository]
ROOT              = {{.DataDir}}/repositories
DEFAULT_BRANCH    = main
DEFAULT_PRIVATE   = last
ENABLE_PUSH_CREATE_USER = true
ENABLE_PUSH_CREATE_ORG  = true

[lfs]
PATH = {{.DataDir}}/lfs

[log]
MODE      = file
LEVEL     = Info
ROOT_PATH = {{.DataDir}}/log

[service]
DISABLE_REGISTRATION              = true
REQUIRE_SIGNIN_VIEW               = false
REGISTER_EMAIL_CONFIRM            = false
ENABLE_NOTIFY_MAIL                = false
ALLOW_ONLY_EXTERNAL_REGISTRATION  = false
DEFAULT_ALLOW_CREATE_ORGANIZATION = true
NO_REPLY_ADDRESS                  = noreply.{{.Domain}}

[api]
ENABLE_SWAGGER = true
MAX_RESPONSE_ITEMS = 50

[security]
INSTALL_LOCK   = true
SECRET_KEY     =
INTERNAL_TOKEN =

[session]
PROVIDER = file

[picture]
DISABLE_GRAVATAR        = false
ENABLE_FEDERATED_AVATAR = false

[openid]
ENABLE_OPENID_SIGNIN = false
ENABLE_OPENID_SIGNUP = false

[mailer]
ENABLED = false

[indexer]
ISSUE_INDEXER_TYPE = bleve

[markup.sanitizer.1]
ELEMENT   = span
ALLOW_ATTR = class
REGEXP    = ^.*$

[git.timeout]
DEFAULT = 360
MIGRATE = 600
MIRROR  = 300
CLONE   = 300
PULL    = 300
GC      = 60
`

// WriteAppIni generates Gitea's app.ini configuration file.
func WriteAppIni(cfg InstallerConfig) error {
	cfg.defaults()

	tmpl, err := template.New("app.ini").Parse(appIniTemplate)
	if err != nil {
		return fmt.Errorf("parse app.ini template: %w", err)
	}

	confDir := filepath.Join(cfg.InstallDir, "custom", "conf")
	if err := os.MkdirAll(confDir, 0750); err != nil {
		return fmt.Errorf("create conf dir: %w", err)
	}

	iniPath := filepath.Join(confDir, "app.ini")
	f, err := os.OpenFile(iniPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("create app.ini: %w", err)
	}
	defer f.Close()

	data := struct {
		DBType   string
		DBPath   string
		HTTPPort int
		SSHPort  int
		Domain   string
		RootURL  string
		DataDir  string
	}{
		DBType:   cfg.DBType,
		DBPath:   cfg.DBPath,
		HTTPPort: cfg.HTTPPort,
		SSHPort:  cfg.SSHPort,
		Domain:   cfg.Domain,
		RootURL:  cfg.rootURL(),
		DataDir:  cfg.DataDir,
	}

	return tmpl.Execute(f, data)
}

// ---------------------------------------------------------------------------
// Nginx configuration generation
// ---------------------------------------------------------------------------

const nginxSubdomainTemplate = `# Gitea reverse proxy — managed by Setec Manager
server {
    listen 80;
    server_name {{.Domain}};

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl http2;
    server_name {{.Domain}};

    ssl_certificate     /etc/letsencrypt/live/{{.Domain}}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{{.Domain}}/privkey.pem;
    include snippets/ssl-params.conf;

    client_max_body_size 512M;

    location / {
        proxy_pass http://127.0.0.1:{{.HTTPPort}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 900;
    }
}
`

const nginxSubpathTemplate = `# Gitea reverse proxy (subpath) — managed by Setec Manager
# Add this inside the existing server block for {{.Domain}}

location /git/ {
    proxy_pass http://127.0.0.1:{{.HTTPPort}}/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 900;
    client_max_body_size 512M;
}
`

// GenerateNginxConfig produces the Nginx reverse proxy configuration for Gitea.
// The output depends on URLStyle: "subdomain" generates a full server block;
// "subpath" generates a location block to embed in an existing server.
func GenerateNginxConfig(cfg InstallerConfig) (string, error) {
	cfg.defaults()

	var tmplStr string
	switch cfg.URLStyle {
	case "subpath":
		tmplStr = nginxSubpathTemplate
	default:
		tmplStr = nginxSubdomainTemplate
	}

	tmpl, err := template.New("nginx-gitea").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse nginx template: %w", err)
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, cfg); err != nil {
		return "", fmt.Errorf("render nginx template: %w", err)
	}
	return b.String(), nil
}

// ---------------------------------------------------------------------------
// Admin user + token bootstrapping
// ---------------------------------------------------------------------------

// CreateAdminUser runs the Gitea CLI to create an initial admin user.
func CreateAdminUser(installDir, username, password, email string) error {
	if installDir == "" {
		installDir = "/opt/gitea"
	}
	bin := filepath.Join(installDir, "gitea")
	confPath := filepath.Join(installDir, "custom", "conf", "app.ini")

	out, err := exec.Command(
		bin, "admin", "user", "create",
		"--config", confPath,
		"--username", username,
		"--password", password,
		"--email", email,
		"--admin",
		"--must-change-password=false",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("gitea admin user create: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// GenerateAdminToken creates an API token for the given user by calling the
// Gitea API with basic auth.  The Gitea instance must be running.  It retries
// up to 10 times with a 2-second delay to give Gitea time to start.
func GenerateAdminToken(baseURL, username, password string) (string, error) {
	c := New(baseURL, "")

	tokenName := fmt.Sprintf("setec-manager-%d", time.Now().Unix())

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		tok, err := c.CreateToken(username, password, tokenName)
		if err == nil {
			return tok, nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("generate admin token after retries: %w", lastErr)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ensureSystemUser creates a system user if it does not already exist.
func ensureSystemUser(username, homeDir string) error {
	// Check if user already exists.
	if _, err := exec.Command("id", username).Output(); err == nil {
		return nil // user exists
	}

	out, err := exec.Command(
		"useradd",
		"--system",
		"--shell", "/bin/bash",
		"--home-dir", homeDir,
		"--create-home",
		"--comment", "Gitea service account",
		username,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("useradd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// downloadBinary downloads a file from url and saves it to dest with 0755
// permissions.
func downloadBinary(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	// Write to a temp file first, then rename for atomicity.
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := copyWithProgress(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write binary: %w", err)
	}
	f.Close()

	if err := os.Chmod(tmp, 0755); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// copyWithProgress copies src to dst.  Currently a simple wrapper around
// io.Copy, but structured to allow progress reporting in the future.
func copyWithProgress(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr != nil {
			if readErr.Error() == "EOF" {
				break
			}
			return total, readErr
		}
	}
	return total, nil
}

// chownRecursive changes ownership of a directory tree to the given user.
func chownRecursive(dir, user string) error {
	out, err := exec.Command("chown", "-R", user+":"+user, dir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("chown -R %s:%s %s: %s: %w", user, user, dir, strings.TrimSpace(string(out)), err)
	}
	return nil
}
