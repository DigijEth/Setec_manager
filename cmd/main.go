package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"setec-manager/internal/config"
	"setec-manager/internal/db"
	"setec-manager/internal/deploy"
	"setec-manager/internal/nginx"
	"setec-manager/internal/scheduler"
	"setec-manager/internal/server"
)

const banner = `
    ███████╗███████╗████████╗███████╗ ██████╗
    ██╔════╝██╔════╝╚══██╔══╝██╔════╝██╔════╝
    ███████╗█████╗     ██║   █████╗  ██║
    ╚════██║██╔══╝     ██║   ██╔══╝  ██║
    ███████║███████╗   ██║   ███████╗╚██████╗
    ╚══════╝╚══════╝   ╚═╝   ╚══════╝ ╚═════╝
         A P P   M A N A G E R   v1.0
    darkHal Security Group & Setec Security Labs
`

func main() {
	configPath := flag.String("config", "/opt/setec-manager/config.yaml", "Path to config file")
	setup := flag.Bool("setup", false, "Run first-time setup")
	flag.Parse()

	fmt.Print(banner)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[setec] Failed to load config: %v", err)
	}

	// Open database
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("[setec] Failed to open database: %v", err)
	}
	defer database.Close()

	// First-time setup
	if *setup {
		runSetup(cfg, database, *configPath)
		return
	}

	// Check if any admin users exist
	count, _ := database.ManagerUserCount()
	if count == 0 {
		log.Println("[setec] No admin users found. Creating default admin account.")
		log.Println("[setec]   Username: admin")
		log.Println("[setec]   Password: autarch")
		log.Println("[setec]   ** CHANGE THIS IMMEDIATELY **")
		database.CreateManagerUser("admin", "autarch", "admin")
	}

	// Load or create persistent JWT key
	dataDir := filepath.Dir(cfg.Database.Path)
	jwtKey, err := server.LoadOrCreateJWTKey(dataDir)
	if err != nil {
		log.Fatalf("[setec] Failed to load JWT key: %v", err)
	}

	// Create and start server
	srv := server.New(cfg, database, jwtKey)

	// Start scheduler
	sched := scheduler.New(database)
	sched.RegisterHandler(scheduler.JobSSLRenew, func(siteID *int64) error {
		log.Println("[scheduler] Running SSL renewal")
		_, err := exec.Command("certbot", "renew", "--non-interactive").CombinedOutput()
		return err
	})
	sched.RegisterHandler(scheduler.JobCleanup, func(siteID *int64) error {
		log.Println("[scheduler] Running cleanup")
		return nil
	})
	sched.RegisterHandler(scheduler.JobBackup, func(siteID *int64) error {
		if siteID == nil {
			log.Println("[scheduler] Backup job requires a site ID, skipping")
			return fmt.Errorf("backup job requires a site ID")
		}
		site, err := database.GetSite(*siteID)
		if err != nil || site == nil {
			return fmt.Errorf("backup: site %d not found", *siteID)
		}

		backupDir := cfg.Backups.Dir
		os.MkdirAll(backupDir, 0755)

		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("site-%s-%s.tar.gz", site.Domain, timestamp)
		backupPath := filepath.Join(backupDir, filename)

		cmd := exec.Command("tar", "-czf", backupPath, "-C", filepath.Dir(site.AppRoot), filepath.Base(site.AppRoot))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("backup tar failed: %s: %w", string(out), err)
		}

		info, _ := os.Stat(backupPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}

		database.CreateBackup(siteID, "site", backupPath, size)
		log.Printf("[scheduler] Backup complete for site %s: %s (%d bytes)", site.Domain, backupPath, size)
		return nil
	})
	sched.RegisterHandler(scheduler.JobGitPull, func(siteID *int64) error {
		if siteID == nil {
			return fmt.Errorf("git_pull job requires a site ID")
		}
		site, err := database.GetSite(*siteID)
		if err != nil || site == nil {
			return fmt.Errorf("git_pull: site %d not found", *siteID)
		}
		if site.GitRepo == "" {
			return fmt.Errorf("git_pull: site %s has no git repo configured", site.Domain)
		}

		output, err := deploy.Pull(site.AppRoot)
		if err != nil {
			return fmt.Errorf("git_pull %s: %w", site.Domain, err)
		}
		log.Printf("[scheduler] Git pull for site %s: %s", site.Domain, strings.TrimSpace(output))
		return nil
	})
	sched.RegisterHandler(scheduler.JobRestart, func(siteID *int64) error {
		if siteID == nil {
			return fmt.Errorf("restart job requires a site ID")
		}
		site, err := database.GetSite(*siteID)
		if err != nil || site == nil {
			return fmt.Errorf("restart: site %d not found", *siteID)
		}

		unitName := fmt.Sprintf("app-%s", site.Domain)
		if err := deploy.Restart(unitName); err != nil {
			return fmt.Errorf("restart %s: %w", site.Domain, err)
		}
		log.Printf("[scheduler] Restarted service for site %s (unit: %s)", site.Domain, unitName)
		return nil
	})
	sched.Start()

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("[setec] Server error: %v", err)
		}
	}()

	log.Printf("[setec] Dashboard: https://%s:%d", cfg.Server.Host, cfg.Server.Port)

	<-done
	log.Println("[setec] Shutting down...")
	sched.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func runSetup(cfg *config.Config, database *db.DB, configPath string) {
	log.Println("[setup] Starting first-time setup...")

	// Ensure directories exist
	dirs := []string{
		"/opt/setec-manager/data",
		"/opt/setec-manager/data/acme",
		"/opt/setec-manager/data/backups",
		cfg.Nginx.Webroot,
		cfg.Nginx.CertbotWebroot,
		cfg.Nginx.SitesAvailable,
		cfg.Nginx.SitesEnabled,
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}

	// Install Nginx if needed
	log.Println("[setup] Installing nginx...")
	execQuiet("apt-get", "update", "-qq")
	execQuiet("apt-get", "install", "-y", "nginx", "certbot", "ufw")

	// Install nginx snippets
	log.Println("[setup] Configuring nginx snippets...")
	nginx.InstallSnippets(cfg)

	// Create admin user
	count, _ := database.ManagerUserCount()
	if count == 0 {
		log.Println("[setup] Creating default admin user (admin / autarch)")
		database.CreateManagerUser("admin", "autarch", "admin")
	}

	// Save config
	cfg.Save(configPath)

	// Generate self-signed cert for manager if none exists
	if _, err := os.Stat(cfg.Server.Cert); os.IsNotExist(err) {
		log.Println("[setup] Generating self-signed TLS cert for manager...")
		os.MkdirAll(cfg.ACME.AccountDir, 0755)
		execQuiet("openssl", "req", "-x509", "-newkey", "rsa:2048",
			"-keyout", cfg.Server.Key, "-out", cfg.Server.Cert,
			"-days", "3650", "-nodes",
			"-subj", "/CN=setec-manager/O=Setec Security Labs")
	}

	// Install systemd unit for setec-manager
	unit := `[Unit]
Description=Setec App Manager
After=network.target

[Service]
Type=simple
User=root
ExecStart=/opt/setec-manager/setec-manager --config /opt/setec-manager/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	os.WriteFile("/etc/systemd/system/setec-manager.service", []byte(unit), 0644)
	execQuiet("systemctl", "daemon-reload")

	log.Println("[setup] Setup complete!")
	log.Println("[setup] Start with: systemctl start setec-manager")
	log.Printf("[setup] Dashboard will be at: https://<your-ip>:%d\n", cfg.Server.Port)
}

func execQuiet(name string, args ...string) {
	log.Printf("[setup] $ %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[setup]   Warning: %v\n%s", err, string(out))
	}
}
