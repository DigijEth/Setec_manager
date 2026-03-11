package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Nginx    NginxConfig    `yaml:"nginx"`
	ACME     ACMEConfig     `yaml:"acme"`
	Autarch  AutarchConfig  `yaml:"autarch"`
	Gitea    GiteaConfig    `yaml:"gitea"`
	Float    FloatConfig    `yaml:"float"`
	Backups  BackupsConfig  `yaml:"backups"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type GiteaConfig struct {
	Installed  bool   `yaml:"installed"`
	BaseURL    string `yaml:"base_url"`
	AdminToken string `yaml:"admin_token"`
	Domain     string `yaml:"domain"`
	URLStyle   string `yaml:"url_style"`
	SSHPort    int    `yaml:"ssh_port"`
	DataDir    string `yaml:"data_dir"`
	BinaryPath string `yaml:"binary_path"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	TLS  bool   `yaml:"tls"`
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type NginxConfig struct {
	SitesAvailable string `yaml:"sites_available"`
	SitesEnabled   string `yaml:"sites_enabled"`
	Snippets       string `yaml:"snippets"`
	Webroot        string `yaml:"webroot"`
	CertbotWebroot string `yaml:"certbot_webroot"`
}

type ACMEConfig struct {
	Email      string `yaml:"email"`
	Staging    bool   `yaml:"staging"`
	AccountDir string `yaml:"account_dir"`
}

type AutarchConfig struct {
	InstallDir string `yaml:"install_dir"`
	GitRepo    string `yaml:"git_repo"`
	GitBranch  string `yaml:"git_branch"`
	WebPort    int    `yaml:"web_port"`
	DNSPort    int    `yaml:"dns_port"`
}

type FloatConfig struct {
	Enabled     bool   `yaml:"enabled"`
	MaxSessions int    `yaml:"max_sessions"`
	SessionTTL  string `yaml:"session_ttl"`
}

type BackupsConfig struct {
	Dir        string `yaml:"dir"`
	MaxAgeDays int    `yaml:"max_age_days"`
	MaxCount   int    `yaml:"max_count"`
}

type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 9090,
			TLS:  true,
			Cert: "/opt/setec-manager/data/acme/manager.crt",
			Key:  "/opt/setec-manager/data/acme/manager.key",
		},
		Database: DatabaseConfig{
			Path: "/opt/setec-manager/data/setec.db",
		},
		Nginx: NginxConfig{
			SitesAvailable: "/etc/nginx/sites-available",
			SitesEnabled:   "/etc/nginx/sites-enabled",
			Snippets:       "/etc/nginx/snippets",
			Webroot:        "/var/www",
			CertbotWebroot: "/var/www/certbot",
		},
		ACME: ACMEConfig{
			Email:      "",
			Staging:    false,
			AccountDir: "/opt/setec-manager/data/acme",
		},
		Autarch: AutarchConfig{
			InstallDir: "/var/www/autarch",
			GitRepo:    "https://github.com/DigijEth/autarch.git",
			GitBranch:  "main",
			WebPort:    8181,
			DNSPort:    53,
		},
		Gitea: GiteaConfig{
			Installed:  false,
			SSHPort:    2222,
			DataDir:    "/opt/gitea",
			BinaryPath: "/usr/local/bin/gitea",
		},
		Float: FloatConfig{
			Enabled:     false,
			MaxSessions: 10,
			SessionTTL:  "24h",
		},
		Backups: BackupsConfig{
			Dir:        "/opt/setec-manager/data/backups",
			MaxAgeDays: 30,
			MaxCount:   50,
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "/var/log/setec-manager.log",
			MaxSizeMB:  100,
			MaxBackups: 3,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
