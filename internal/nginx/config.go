package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"setec-manager/internal/config"
	"setec-manager/internal/db"
)

const reverseProxyTemplate = `# Managed by Setec App Manager — do not edit manually
server {
    listen 80;
    server_name {{.Domain}}{{if .Aliases}} {{.Aliases}}{{end}};

    location /.well-known/acme-challenge/ {
        root {{.CertbotWebroot}};
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

{{if .SSLEnabled}}server {
    listen 443 ssl http2;
    server_name {{.Domain}}{{if .Aliases}} {{.Aliases}}{{end}};

    ssl_certificate     {{.SSLCertPath}};
    ssl_certificate_key {{.SSLKeyPath}};
    include snippets/ssl-params.conf;

    location / {
        proxy_pass http://127.0.0.1:{{.AppPort}};
        include snippets/proxy-params.conf;
    }

    # WebSocket / SSE support
    location /api/ {
        proxy_pass http://127.0.0.1:{{.AppPort}};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
        include snippets/proxy-params.conf;
    }
}{{end}}
`

const staticSiteTemplate = `# Managed by Setec App Manager — do not edit manually
server {
    listen 80;
    server_name {{.Domain}}{{if .Aliases}} {{.Aliases}}{{end}};

    location /.well-known/acme-challenge/ {
        root {{.CertbotWebroot}};
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

{{if .SSLEnabled}}server {
    listen 443 ssl http2;
    server_name {{.Domain}}{{if .Aliases}} {{.Aliases}}{{end}};
    root {{.AppRoot}};
    index index.html;

    ssl_certificate     {{.SSLCertPath}};
    ssl_certificate_key {{.SSLKeyPath}};
    include snippets/ssl-params.conf;

    location / {
        try_files $uri $uri/ =404;
    }
}{{else}}server {
    listen 80;
    server_name {{.Domain}}{{if .Aliases}} {{.Aliases}}{{end}};
    root {{.AppRoot}};
    index index.html;

    location / {
        try_files $uri $uri/ =404;
    }
}{{end}}
`

type configData struct {
	Domain         string
	Aliases        string
	AppRoot        string
	AppPort        int
	SSLEnabled     bool
	SSLCertPath    string
	SSLKeyPath     string
	CertbotWebroot string
}

func GenerateConfig(cfg *config.Config, site *db.Site) error {
	data := configData{
		Domain:         site.Domain,
		Aliases:        site.Aliases,
		AppRoot:        site.AppRoot,
		AppPort:        site.AppPort,
		SSLEnabled:     site.SSLEnabled,
		SSLCertPath:    site.SSLCertPath,
		SSLKeyPath:     site.SSLKeyPath,
		CertbotWebroot: cfg.Nginx.CertbotWebroot,
	}

	var tmplStr string
	switch site.AppType {
	case "static":
		tmplStr = staticSiteTemplate
	default:
		tmplStr = reverseProxyTemplate
	}

	tmpl, err := template.New("nginx").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	path := filepath.Join(cfg.Nginx.SitesAvailable, site.Domain)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func EnableSite(cfg *config.Config, domain string) error {
	src := filepath.Join(cfg.Nginx.SitesAvailable, domain)
	dst := filepath.Join(cfg.Nginx.SitesEnabled, domain)

	// Remove existing symlink
	os.Remove(dst)

	return os.Symlink(src, dst)
}

func DisableSite(cfg *config.Config, domain string) error {
	dst := filepath.Join(cfg.Nginx.SitesEnabled, domain)
	return os.Remove(dst)
}

func Reload() error {
	return exec.Command("systemctl", "reload", "nginx").Run()
}

func Restart() error {
	return exec.Command("systemctl", "restart", "nginx").Run()
}

func Test() (string, error) {
	out, err := exec.Command("nginx", "-t").CombinedOutput()
	return string(out), err
}

func Status() (string, bool) {
	out, err := exec.Command("systemctl", "is-active", "nginx").Output()
	status := strings.TrimSpace(string(out))
	return status, err == nil && status == "active"
}

func InstallSnippets(cfg *config.Config) error {
	os.MkdirAll(cfg.Nginx.Snippets, 0755)

	sslParams := `# SSL params — managed by Setec App Manager
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
ssl_prefer_server_ciphers off;
ssl_session_timeout 1d;
ssl_session_cache shared:SSL:10m;
ssl_session_tickets off;
ssl_stapling on;
ssl_stapling_verify on;
add_header Strict-Transport-Security "max-age=63072000" always;
`

	proxyParams := `# Proxy params — managed by Setec App Manager
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_buffering off;
proxy_request_buffering off;
`

	if err := os.WriteFile(filepath.Join(cfg.Nginx.Snippets, "ssl-params.conf"), []byte(sslParams), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg.Nginx.Snippets, "proxy-params.conf"), []byte(proxyParams), 0644)
}
