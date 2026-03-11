package system

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ── Types ───────────────────────────────────────────────────────────

type SystemUser struct {
	Username string `json:"username"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Comment  string `json:"comment"`
	HomeDir  string `json:"home_dir"`
	Shell    string `json:"shell"`
}

type QuotaInfo struct {
	Username  string `json:"username"`
	UsedBytes uint64 `json:"used_bytes"`
	UsedHuman string `json:"used_human"`
	HomeDir   string `json:"home_dir"`
}

// ── Protected accounts ──────────────────────────────────────────────

var protectedUsers = map[string]bool{
	"root":    true,
	"autarch": true,
}

// ── User Management ─────────────────────────────────────────────────

// ListUsers reads /etc/passwd and returns all users with UID >= 1000 and < 65534.
func ListUsers() ([]SystemUser, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, fmt.Errorf("opening /etc/passwd: %w", err)
	}
	defer f.Close()

	var users []SystemUser
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Format: username:x:uid:gid:comment:home:shell
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}

		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}

		// Only normal user accounts (UID 1000-65533)
		if uid < 1000 || uid >= 65534 {
			continue
		}

		gid, _ := strconv.Atoi(fields[3])

		users = append(users, SystemUser{
			Username: fields[0],
			UID:      uid,
			GID:      gid,
			Comment:  fields[4],
			HomeDir:  fields[5],
			Shell:    fields[6],
		})
	}

	return users, scanner.Err()
}

// CreateUser creates a new system user with the given username, password, and shell.
func CreateUser(username, password, shell string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	if shell == "" {
		shell = "/bin/bash"
	}

	// Sanitize: only allow alphanumeric, underscore, hyphen, and dot
	for _, c := range username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return fmt.Errorf("invalid character %q in username", c)
		}
	}
	if len(username) > 32 {
		return fmt.Errorf("username too long (max 32 characters)")
	}

	// Verify the shell exists
	if _, err := os.Stat(shell); err != nil {
		return fmt.Errorf("shell %q does not exist: %w", shell, err)
	}

	// Create the user
	out, err := exec.Command("useradd", "--create-home", "--shell", shell, username).CombinedOutput()
	if err != nil {
		return fmt.Errorf("useradd failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Set the password via chpasswd
	if err := setPasswordViaChpasswd(username, password); err != nil {
		// Attempt cleanup on password failure
		exec.Command("userdel", "--remove", username).Run()
		return fmt.Errorf("user created but password set failed (user removed): %w", err)
	}

	return nil
}

// DeleteUser removes a system user and their home directory.
func DeleteUser(username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}

	if protectedUsers[username] {
		return fmt.Errorf("cannot delete protected account %q", username)
	}

	// Verify the user actually exists before attempting deletion
	_, err := exec.Command("id", username).CombinedOutput()
	if err != nil {
		return fmt.Errorf("user %q does not exist", username)
	}

	// Kill any running processes owned by the user (best effort)
	exec.Command("pkill", "-u", username).Run()

	out, err := exec.Command("userdel", "--remove", username).CombinedOutput()
	if err != nil {
		return fmt.Errorf("userdel failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// SetPassword changes the password for an existing user.
func SetPassword(username, password string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	// Verify user exists
	_, err := exec.Command("id", username).CombinedOutput()
	if err != nil {
		return fmt.Errorf("user %q does not exist", username)
	}

	return setPasswordViaChpasswd(username, password)
}

// setPasswordViaChpasswd pipes "user:password" to chpasswd.
func setPasswordViaChpasswd(username, password string) error {
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", username, password))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chpasswd failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// AddSSHKey appends a public key to the user's ~/.ssh/authorized_keys file.
func AddSSHKey(username, pubkey string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if pubkey == "" {
		return fmt.Errorf("public key is required")
	}

	// Basic validation: SSH keys should start with a recognized prefix
	pubkey = strings.TrimSpace(pubkey)
	validPrefixes := []string{"ssh-rsa", "ssh-ed25519", "ssh-dss", "ecdsa-sha2-", "sk-ssh-ed25519", "sk-ecdsa-sha2-"}
	valid := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(pubkey, prefix) {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid SSH public key format")
	}

	// Look up the user's home directory from /etc/passwd
	homeDir, err := getUserHome(username)
	if err != nil {
		return err
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	authKeysPath := filepath.Join(sshDir, "authorized_keys")

	// Create .ssh directory if it doesn't exist
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("creating .ssh directory: %w", err)
	}

	// Check for duplicate keys
	if existing, err := os.ReadFile(authKeysPath); err == nil {
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(line) == pubkey {
				return fmt.Errorf("SSH key already exists in authorized_keys")
			}
		}
	}

	// Append the key
	f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening authorized_keys: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", pubkey); err != nil {
		return fmt.Errorf("writing authorized_keys: %w", err)
	}

	// Fix ownership: chown user:user .ssh and authorized_keys
	exec.Command("chown", "-R", username+":"+username, sshDir).Run()

	return nil
}

// GetUserQuota returns disk usage for a user's home directory.
func GetUserQuota(username string) (QuotaInfo, error) {
	info := QuotaInfo{Username: username}

	homeDir, err := getUserHome(username)
	if err != nil {
		return info, err
	}
	info.HomeDir = homeDir

	// Use du -sb for total bytes used in home directory
	out, err := exec.Command("du", "-sb", homeDir).CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("du failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 1 {
		info.UsedBytes, _ = strconv.ParseUint(fields[0], 10, 64)
	}

	info.UsedHuman = humanBytes(info.UsedBytes)

	return info, nil
}

// getUserHome looks up a user's home directory from /etc/passwd.
func getUserHome(username string) (string, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return "", fmt.Errorf("opening /etc/passwd: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) >= 6 && fields[0] == username {
			return fields[5], nil
		}
	}

	return "", fmt.Errorf("user %q not found in /etc/passwd", username)
}
