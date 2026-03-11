package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NpmInstall runs npm install in the given directory.
func NpmInstall(dir string) (string, error) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return "", fmt.Errorf("npm not found: %w", err)
	}

	cmd := exec.Command(npm, "install")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("npm install: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// NpmBuild runs npm run build in the given directory.
func NpmBuild(dir string) (string, error) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return "", fmt.Errorf("npm not found: %w", err)
	}

	cmd := exec.Command(npm, "run", "build")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("npm run build: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// NpmAudit runs npm audit in the given directory and returns the report.
func NpmAudit(dir string) (string, error) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return "", fmt.Errorf("npm not found: %w", err)
	}

	cmd := exec.Command(npm, "audit")
	cmd.Dir = dir
	// npm audit exits non-zero when vulnerabilities are found, which is not
	// an execution error — we still want the output.
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Return the output even on non-zero exit; the caller can inspect it.
		return string(out), fmt.Errorf("npm audit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// HasPackageJSON returns true if a package.json file exists in dir.
func HasPackageJSON(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil && !info.IsDir()
}

// HasNodeModules returns true if a node_modules directory exists in dir.
func HasNodeModules(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "node_modules"))
	return err == nil && info.IsDir()
}

// NodeVersion returns the installed Node.js version string.
func NodeVersion() (string, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("node not found: %w", err)
	}

	out, err := exec.Command(node, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("node --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// NpmVersion returns the installed npm version string.
func NpmVersion() (string, error) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return "", fmt.Errorf("npm not found: %w", err)
	}

	out, err := exec.Command(npm, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("npm --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
