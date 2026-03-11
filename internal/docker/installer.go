package docker

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ---------------------------------------------------------------------------
// Docker CE Installation (Debian/Ubuntu)
// ---------------------------------------------------------------------------

// IsInstalled checks if the docker binary exists on the system PATH.
func IsInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// Install installs Docker CE from the official Docker repository on
// Debian/Ubuntu systems. It performs the following steps:
//  1. Updates the apt package index.
//  2. Installs prerequisites (ca-certificates, curl, gnupg).
//  3. Adds Docker's official GPG key.
//  4. Adds the Docker apt repository.
//  5. Updates the package index again.
//  6. Installs docker-ce, docker-ce-cli, containerd.io, docker-compose-plugin.
//  7. Enables and starts the docker service.
func Install() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("docker installation is only supported on Linux")
	}

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"update package index", aptUpdate},
		{"install prerequisites", installPrereqs},
		{"add Docker GPG key", addDockerGPGKey},
		{"add Docker apt repository", addDockerRepo},
		{"update package index (post-repo)", aptUpdate},
		{"install Docker CE packages", installDockerPackages},
		{"enable docker service", EnableService},
		{"start docker service", StartService},
	}

	for _, step := range steps {
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.desc, err)
		}
	}

	return nil
}

func aptUpdate() error {
	return runCmd("apt-get", "update", "-y")
}

func installPrereqs() error {
	return runCmd("apt-get", "install", "-y",
		"ca-certificates",
		"curl",
		"gnupg",
		"lsb-release",
	)
}

func addDockerGPGKey() error {
	// Create the keyrings directory.
	if err := os.MkdirAll("/etc/apt/keyrings", 0755); err != nil {
		return fmt.Errorf("create keyrings dir: %w", err)
	}

	// Remove old key file if it exists.
	os.Remove("/etc/apt/keyrings/docker.gpg")

	// Download and dearmor the GPG key.
	// curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
	curlCmd := exec.Command("curl", "-fsSL", "https://download.docker.com/linux/ubuntu/gpg")
	gpgCmd := exec.Command("gpg", "--dearmor", "-o", "/etc/apt/keyrings/docker.gpg")

	pipe, err := curlCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	gpgCmd.Stdin = pipe

	if err := curlCmd.Start(); err != nil {
		return fmt.Errorf("curl start: %w", err)
	}

	gpgOut, err := gpgCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gpg dearmor: %s: %w", strings.TrimSpace(string(gpgOut)), err)
	}

	if err := curlCmd.Wait(); err != nil {
		return fmt.Errorf("curl download: %w", err)
	}

	// Set permissions on the key file.
	return os.Chmod("/etc/apt/keyrings/docker.gpg", 0644)
}

func addDockerRepo() error {
	// Detect the distribution. Try /etc/os-release first.
	distro := detectDistro()
	codename := detectCodename()

	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		// already correct
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "armhf"
	default:
		arch = "amd64"
	}

	// Determine the base URL (Ubuntu vs Debian).
	baseURL := "https://download.docker.com/linux/" + distro

	repoLine := fmt.Sprintf(
		"deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] %s %s stable",
		arch, baseURL, codename,
	)

	return os.WriteFile("/etc/apt/sources.list.d/docker.list", []byte(repoLine+"\n"), 0644)
}

func installDockerPackages() error {
	return runCmd("apt-get", "install", "-y",
		"docker-ce",
		"docker-ce-cli",
		"containerd.io",
		"docker-buildx-plugin",
		"docker-compose-plugin",
	)
}

// ---------------------------------------------------------------------------
// Service management
// ---------------------------------------------------------------------------

// EnableService enables the docker systemd service to start on boot.
func EnableService() error {
	return runCmd("systemctl", "enable", "docker")
}

// StartService starts the docker systemd service.
func StartService() error {
	return runCmd("systemctl", "start", "docker")
}

// StopService stops the docker systemd service.
func StopService() error {
	return runCmd("systemctl", "stop", "docker")
}

// RestartService restarts the docker systemd service.
func RestartService() error {
	return runCmd("systemctl", "restart", "docker")
}

// ServiceStatus returns the current status of the docker systemd service as
// a human-readable string (e.g. "active (running)").
func ServiceStatus() (string, error) {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return "unknown", fmt.Errorf("systemctl not found: %w", err)
	}

	// systemctl status exits non-zero for stopped services, so we use
	// CombinedOutput and only treat missing-binary as a real error.
	out, _ := exec.Command(systemctl, "is-active", "docker").Output()
	status := strings.TrimSpace(string(out))
	if status == "" {
		status = "unknown"
	}
	return status, nil
}

// Uninstall removes Docker CE packages and cleans up.
func Uninstall() error {
	if err := StopService(); err != nil {
		// Best-effort stop.
		_ = err
	}

	if err := runCmd("apt-get", "purge", "-y",
		"docker-ce",
		"docker-ce-cli",
		"containerd.io",
		"docker-buildx-plugin",
		"docker-compose-plugin",
	); err != nil {
		return fmt.Errorf("remove packages: %w", err)
	}

	// Clean up repo and key.
	os.Remove("/etc/apt/sources.list.d/docker.list")
	os.Remove("/etc/apt/keyrings/docker.gpg")

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s: %w",
			name, strings.Join(args, " "),
			strings.TrimSpace(string(out)), err)
	}
	return nil
}

// detectDistro returns "ubuntu" or "debian" based on /etc/os-release.
func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "ubuntu"
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "ID=") {
			val := strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			switch val {
			case "debian":
				return "debian"
			case "ubuntu":
				return "ubuntu"
			default:
				// For derivatives (e.g. linuxmint), check ID_LIKE.
			}
		}
	}
	// Check ID_LIKE as fallback.
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "ID_LIKE=") {
			val := strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), "\"")
			if strings.Contains(val, "debian") {
				return "debian"
			}
			if strings.Contains(val, "ubuntu") {
				return "ubuntu"
			}
		}
	}
	return "ubuntu"
}

// detectCodename returns the OS version codename (e.g. "jammy", "bookworm").
func detectCodename() string {
	// Try lsb_release first.
	out, err := exec.Command("lsb_release", "-cs").Output()
	if err == nil {
		codename := strings.TrimSpace(string(out))
		if codename != "" {
			return codename
		}
	}

	// Fall back to parsing /etc/os-release.
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "jammy"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VERSION_CODENAME=") {
			val := strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), "\"")
			if val != "" {
				return val
			}
		}
	}
	// For Ubuntu, try UBUNTU_CODENAME.
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "UBUNTU_CODENAME=") {
			val := strings.Trim(strings.TrimPrefix(line, "UBUNTU_CODENAME="), "\"")
			if val != "" {
				return val
			}
		}
	}
	return "jammy"
}
