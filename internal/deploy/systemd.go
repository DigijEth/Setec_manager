package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// UnitConfig holds the parameters needed to generate a systemd unit file.
type UnitConfig struct {
	Name             string
	Description      string
	ExecStart        string
	WorkingDirectory string
	User             string
	Environment      map[string]string
	After            string
	RestartPolicy    string
}

// GenerateUnit produces the contents of a systemd service unit file from cfg.
func GenerateUnit(cfg UnitConfig) string {
	var b strings.Builder

	// [Unit]
	b.WriteString("[Unit]\n")
	if cfg.Description != "" {
		fmt.Fprintf(&b, "Description=%s\n", cfg.Description)
	}
	after := cfg.After
	if after == "" {
		after = "network.target"
	}
	fmt.Fprintf(&b, "After=%s\n", after)

	// [Service]
	b.WriteString("\n[Service]\n")
	b.WriteString("Type=simple\n")
	if cfg.User != "" {
		fmt.Fprintf(&b, "User=%s\n", cfg.User)
	}
	if cfg.WorkingDirectory != "" {
		fmt.Fprintf(&b, "WorkingDirectory=%s\n", cfg.WorkingDirectory)
	}
	fmt.Fprintf(&b, "ExecStart=%s\n", cfg.ExecStart)

	restart := cfg.RestartPolicy
	if restart == "" {
		restart = "on-failure"
	}
	fmt.Fprintf(&b, "Restart=%s\n", restart)
	b.WriteString("RestartSec=5\n")

	// Environment variables — sorted for deterministic output.
	if len(cfg.Environment) > 0 {
		keys := make([]string, 0, len(cfg.Environment))
		for k := range cfg.Environment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "Environment=%s=%s\n", k, cfg.Environment[k])
		}
	}

	// [Install]
	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")

	return b.String()
}

// InstallUnit writes a systemd unit file and reloads the daemon.
func InstallUnit(name, content string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	unitPath := filepath.Join("/etc/systemd/system", name+".service")
	if err := os.WriteFile(unitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	out, err := exec.Command(systemctl, "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RemoveUnit stops, disables, and removes a systemd unit file, then reloads.
func RemoveUnit(name string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	unit := name + ".service"

	// Best-effort stop and disable — ignore errors if already stopped/disabled.
	exec.Command(systemctl, "stop", unit).Run()
	exec.Command(systemctl, "disable", unit).Run()

	unitPath := filepath.Join("/etc/systemd/system", unit)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit file: %w", err)
	}

	out, err := exec.Command(systemctl, "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Start starts a systemd unit.
func Start(unit string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "start", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("start %s: %s: %w", unit, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Stop stops a systemd unit.
func Stop(unit string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "stop", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop %s: %s: %w", unit, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Restart restarts a systemd unit.
func Restart(unit string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "restart", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart %s: %s: %w", unit, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Enable enables a systemd unit to start on boot.
func Enable(unit string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "enable", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("enable %s: %s: %w", unit, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Disable disables a systemd unit from starting on boot.
func Disable(unit string) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "disable", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("disable %s: %s: %w", unit, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// IsActive returns true if the given systemd unit is currently active.
func IsActive(unit string) (bool, error) {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return false, fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "is-active", unit).Output()
	status := strings.TrimSpace(string(out))
	if status == "active" {
		return true, nil
	}
	// is-active exits non-zero for inactive/failed — that is not an error
	// in our context, just means the unit is not active.
	return false, nil
}

// Status returns the full systemctl status output for a unit.
func Status(unit string) (string, error) {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return "", fmt.Errorf("systemctl not found: %w", err)
	}

	// systemctl status exits non-zero for stopped services, so we use
	// CombinedOutput and only treat missing-binary as a real error.
	out, _ := exec.Command(systemctl, "status", unit).CombinedOutput()
	return string(out), nil
}

// Logs returns the last n lines of journal output for a systemd unit.
func Logs(unit string, lines int) (string, error) {
	journalctl, err := exec.LookPath("journalctl")
	if err != nil {
		return "", fmt.Errorf("journalctl not found: %w", err)
	}

	out, err := exec.Command(journalctl, "-u", unit, "-n", fmt.Sprintf("%d", lines), "--no-pager").CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("journalctl: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// DaemonReload runs systemctl daemon-reload.
func DaemonReload() error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}

	out, err := exec.Command(systemctl, "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
