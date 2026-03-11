package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ── Types ───────────────────────────────────────────────────────────

type PackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Size    int64  `json:"size"`    // installed size in bytes
	SizeStr string `json:"size_str"` // human-readable
	Status  string `json:"status,omitempty"`
}

// ── APT Operations ──────────────────────────────────────────────────

// PackageUpdate runs `apt-get update` to refresh the package index.
func PackageUpdate() (string, error) {
	out, err := exec.Command("apt-get", "update", "-qq").CombinedOutput()
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("apt-get update failed: %w (%s)", err, output)
	}
	return output, nil
}

// PackageInstall installs one or more packages with apt-get install -y.
// Package names are passed as separate arguments to avoid shell injection.
func PackageInstall(packages ...string) (string, error) {
	if len(packages) == 0 {
		return "", fmt.Errorf("no packages specified")
	}

	for _, pkg := range packages {
		if err := validatePackageName(pkg); err != nil {
			return "", err
		}
	}

	args := append([]string{"install", "-y"}, packages...)
	out, err := exec.Command("apt-get", args...).CombinedOutput()
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("apt-get install failed: %w (%s)", err, output)
	}
	return output, nil
}

// PackageRemove removes one or more packages with apt-get remove -y.
func PackageRemove(packages ...string) (string, error) {
	if len(packages) == 0 {
		return "", fmt.Errorf("no packages specified")
	}

	for _, pkg := range packages {
		if err := validatePackageName(pkg); err != nil {
			return "", err
		}
	}

	args := append([]string{"remove", "-y"}, packages...)
	out, err := exec.Command("apt-get", args...).CombinedOutput()
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("apt-get remove failed: %w (%s)", err, output)
	}
	return output, nil
}

// PackageListInstalled returns all installed packages via dpkg-query.
func PackageListInstalled() ([]PackageInfo, error) {
	// dpkg-query format: name\tversion\tinstalled-size (in kB)
	out, err := exec.Command(
		"dpkg-query",
		"--show",
		"--showformat=${Package}\t${Version}\t${Installed-Size}\n",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("dpkg-query failed: %w (%s)", err, string(out))
	}

	var packages []PackageInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		// Installed-Size from dpkg is in kibibytes
		sizeKB, _ := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		sizeBytes := sizeKB * 1024

		packages = append(packages, PackageInfo{
			Name:    fields[0],
			Version: fields[1],
			Size:    sizeBytes,
			SizeStr: humanBytes(uint64(sizeBytes)),
		})
	}

	return packages, nil
}

// PackageIsInstalled checks if a single package is installed using dpkg -l.
func PackageIsInstalled(pkg string) bool {
	if err := validatePackageName(pkg); err != nil {
		return false
	}

	out, err := exec.Command("dpkg", "-l", pkg).CombinedOutput()
	if err != nil {
		return false
	}

	// dpkg -l output has lines starting with "ii" for installed packages
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "ii" && fields[1] == pkg {
			return true
		}
	}

	return false
}

// PackageUpgrade runs `apt-get upgrade -y` to upgrade all packages.
func PackageUpgrade() (string, error) {
	out, err := exec.Command("apt-get", "upgrade", "-y").CombinedOutput()
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("apt-get upgrade failed: %w (%s)", err, output)
	}
	return output, nil
}

// PackageSecurityUpdates returns a list of packages with available security updates.
func PackageSecurityUpdates() ([]PackageInfo, error) {
	// apt list --upgradable outputs lines like:
	//   package/suite version arch [upgradable from: old-version]
	out, err := exec.Command("apt", "list", "--upgradable").CombinedOutput()
	if err != nil {
		// apt list may return exit code 1 even with valid output
		if len(out) == 0 {
			return nil, fmt.Errorf("apt list --upgradable failed: %w", err)
		}
	}

	var securityPkgs []PackageInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		// Skip header/warning lines
		if strings.HasPrefix(line, "Listing") || strings.HasPrefix(line, "WARNING") || strings.TrimSpace(line) == "" {
			continue
		}

		// Filter for security updates: look for "-security" in the suite name
		if !strings.Contains(line, "-security") {
			continue
		}

		// Parse: "name/suite version arch [upgradable from: old]"
		slashIdx := strings.Index(line, "/")
		if slashIdx < 0 {
			continue
		}
		name := line[:slashIdx]

		// Get version from the fields after the suite
		rest := line[slashIdx+1:]
		fields := strings.Fields(rest)
		var version string
		if len(fields) >= 2 {
			version = fields[1]
		}

		securityPkgs = append(securityPkgs, PackageInfo{
			Name:    name,
			Version: version,
			Status:  "security-update",
		})
	}

	return securityPkgs, nil
}

// ── Helpers ─────────────────────────────────────────────────────────

// validatePackageName does basic validation to prevent obvious injection attempts.
// Package names in Debian must consist of lowercase alphanumerics, +, -, . and
// must be at least 2 characters long.
func validatePackageName(pkg string) error {
	if len(pkg) < 2 {
		return fmt.Errorf("invalid package name %q: too short", pkg)
	}
	if len(pkg) > 128 {
		return fmt.Errorf("invalid package name %q: too long", pkg)
	}
	for _, c := range pkg {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.' || c == ':') {
			return fmt.Errorf("invalid character %q in package name %q", c, pkg)
		}
	}
	return nil
}
