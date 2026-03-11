package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PipPackage holds the name and version of an installed pip package.
type PipPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CreateVenv creates a Python virtual environment at <dir>/venv.
func CreateVenv(dir string) error {
	python, err := exec.LookPath("python3")
	if err != nil {
		return fmt.Errorf("python3 not found: %w", err)
	}

	venvPath := filepath.Join(dir, "venv")
	out, err := exec.Command(python, "-m", "venv", venvPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create venv: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// UpgradePip upgrades pip, setuptools, and wheel inside the virtual environment
// rooted at venvDir.
func UpgradePip(venvDir string) error {
	pip := filepath.Join(venvDir, "bin", "pip")
	if _, err := os.Stat(pip); err != nil {
		return fmt.Errorf("pip not found at %s: %w", pip, err)
	}

	out, err := exec.Command(pip, "install", "--upgrade", "pip", "setuptools", "wheel").CombinedOutput()
	if err != nil {
		return fmt.Errorf("upgrade pip: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// InstallRequirements installs packages from a requirements file into the
// virtual environment rooted at venvDir.
func InstallRequirements(venvDir, reqFile string) (string, error) {
	pip := filepath.Join(venvDir, "bin", "pip")
	if _, err := os.Stat(pip); err != nil {
		return "", fmt.Errorf("pip not found at %s: %w", pip, err)
	}

	if _, err := os.Stat(reqFile); err != nil {
		return "", fmt.Errorf("requirements file not found: %w", err)
	}

	out, err := exec.Command(pip, "install", "-r", reqFile).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("pip install: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// ListPackages returns all installed packages in the virtual environment
// rooted at venvDir.
func ListPackages(venvDir string) ([]PipPackage, error) {
	pip := filepath.Join(venvDir, "bin", "pip")
	if _, err := os.Stat(pip); err != nil {
		return nil, fmt.Errorf("pip not found at %s: %w", pip, err)
	}

	out, err := exec.Command(pip, "list", "--format=json").Output()
	if err != nil {
		return nil, fmt.Errorf("pip list: %w", err)
	}

	var packages []PipPackage
	if err := json.Unmarshal(out, &packages); err != nil {
		return nil, fmt.Errorf("parse pip list output: %w", err)
	}
	return packages, nil
}

// VenvExists returns true if a virtual environment with a working python3
// binary exists at <dir>/venv.
func VenvExists(dir string) bool {
	python := filepath.Join(dir, "venv", "bin", "python3")
	info, err := os.Stat(python)
	return err == nil && !info.IsDir()
}
