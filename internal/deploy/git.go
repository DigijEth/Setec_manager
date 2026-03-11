package deploy

import (
	"fmt"
	"os/exec"
	"strings"
)

// CommitInfo holds metadata for a single git commit.
type CommitInfo struct {
	Hash    string
	Author  string
	Date    string
	Message string
}

// Clone clones a git repository into dest, checking out the given branch.
func Clone(repo, branch, dest string) (string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found: %w", err)
	}

	args := []string{"clone", "--branch", branch, "--progress", repo, dest}
	out, err := exec.Command(git, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git clone: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// Pull performs a fast-forward-only pull in the given directory.
func Pull(dir string) (string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found: %w", err)
	}

	cmd := exec.Command(git, "pull", "--ff-only")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git pull: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// CurrentCommit returns the hash and message of the latest commit in dir.
func CurrentCommit(dir string) (hash string, message string, err error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", "", fmt.Errorf("git not found: %w", err)
	}

	cmd := exec.Command(git, "log", "--oneline", "-1")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git log: %w", err)
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", "", fmt.Errorf("git log: no commits found")
	}

	parts := strings.SplitN(line, " ", 2)
	hash = parts[0]
	if len(parts) > 1 {
		message = parts[1]
	}
	return hash, message, nil
}

// GetBranch returns the current branch name for the repository in dir.
func GetBranch(dir string) (string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found: %w", err)
	}

	cmd := exec.Command(git, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// HasChanges returns true if the working tree in dir has uncommitted changes.
func HasChanges(dir string) (bool, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return false, fmt.Errorf("git not found: %w", err)
	}

	cmd := exec.Command(git, "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// Log returns the last n commits from the repository in dir.
func Log(dir string, n int) ([]CommitInfo, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found: %w", err)
	}

	// Use a delimiter unlikely to appear in commit messages.
	const sep = "||SETEC||"
	format := fmt.Sprintf("%%h%s%%an%s%%ai%s%%s", sep, sep, sep)

	cmd := exec.Command(git, "log", fmt.Sprintf("-n%d", n), fmt.Sprintf("--format=%s", format))
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var commits []CommitInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, sep, 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, CommitInfo{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
	}
	return commits, nil
}
