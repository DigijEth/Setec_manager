package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Client wraps the certbot CLI for Let's Encrypt ACME certificate management.
type Client struct {
	Email      string
	Staging    bool
	Webroot    string
	AccountDir string
}

// CertInfo holds parsed certificate metadata.
type CertInfo struct {
	Domain    string    `json:"domain"`
	CertPath  string    `json:"cert_path"`
	KeyPath   string    `json:"key_path"`
	ChainPath string    `json:"chain_path"`
	ExpiresAt time.Time `json:"expires_at"`
	Issuer    string    `json:"issuer"`
	DaysLeft  int       `json:"days_left"`
}

// domainRegex validates domain names (basic RFC 1123 hostname check).
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// NewClient creates a new ACME client.
func NewClient(email string, staging bool, webroot, accountDir string) *Client {
	return &Client{
		Email:      email,
		Staging:    staging,
		Webroot:    webroot,
		AccountDir: accountDir,
	}
}

// validateDomain checks that a domain name is syntactically valid before passing
// it to certbot. This prevents command injection and catches obvious typos.
func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain name is empty")
	}
	if len(domain) > 253 {
		return fmt.Errorf("domain name too long: %d characters (max 253)", len(domain))
	}
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain name: %q", domain)
	}
	return nil
}

// Issue requests a new certificate from Let's Encrypt for the given domain
// using the webroot challenge method.
func (c *Client) Issue(domain string) (*CertInfo, error) {
	if err := validateDomain(domain); err != nil {
		return nil, fmt.Errorf("issue: %w", err)
	}

	if err := c.EnsureCertbotInstalled(); err != nil {
		return nil, fmt.Errorf("issue: %w", err)
	}

	// Ensure webroot directory exists
	if err := os.MkdirAll(c.Webroot, 0755); err != nil {
		return nil, fmt.Errorf("issue: create webroot: %w", err)
	}

	args := []string{
		"certonly", "--webroot",
		"-w", c.Webroot,
		"-d", domain,
		"--non-interactive",
		"--agree-tos",
		"-m", c.Email,
	}
	if c.Staging {
		args = append(args, "--staging")
	}

	cmd := exec.Command("certbot", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("certbot certonly failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return c.GetCertInfo(domain)
}

// Renew renews the certificate for a specific domain.
func (c *Client) Renew(domain string) error {
	if err := validateDomain(domain); err != nil {
		return fmt.Errorf("renew: %w", err)
	}

	if err := c.EnsureCertbotInstalled(); err != nil {
		return fmt.Errorf("renew: %w", err)
	}

	cmd := exec.Command("certbot", "renew",
		"--cert-name", domain,
		"--non-interactive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot renew failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RenewAll renews all certificates managed by certbot that are due for renewal.
func (c *Client) RenewAll() (string, error) {
	if err := c.EnsureCertbotInstalled(); err != nil {
		return "", fmt.Errorf("renew all: %w", err)
	}

	cmd := exec.Command("certbot", "renew", "--non-interactive")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("certbot renew --all failed: %s: %w", strings.TrimSpace(output), err)
	}
	return output, nil
}

// Revoke revokes the certificate for a given domain.
func (c *Client) Revoke(domain string) error {
	if err := validateDomain(domain); err != nil {
		return fmt.Errorf("revoke: %w", err)
	}

	if err := c.EnsureCertbotInstalled(); err != nil {
		return fmt.Errorf("revoke: %w", err)
	}

	cmd := exec.Command("certbot", "revoke",
		"--cert-name", domain,
		"--non-interactive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot revoke failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Delete removes a certificate and its renewal configuration from certbot.
func (c *Client) Delete(domain string) error {
	if err := validateDomain(domain); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	if err := c.EnsureCertbotInstalled(); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	cmd := exec.Command("certbot", "delete",
		"--cert-name", domain,
		"--non-interactive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot delete failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListCerts scans /etc/letsencrypt/live/ and parses each certificate to return
// metadata including expiry dates and issuer information.
func (c *Client) ListCerts() ([]CertInfo, error) {
	liveDir := "/etc/letsencrypt/live"
	entries, err := os.ReadDir(liveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No certs directory yet
		}
		return nil, fmt.Errorf("list certs: read live dir: %w", err)
	}

	var certs []CertInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		domain := entry.Name()
		// Skip the README directory certbot sometimes creates
		if domain == "README" {
			continue
		}

		info, err := c.GetCertInfo(domain)
		if err != nil {
			// Log but skip certs we can't parse
			continue
		}
		certs = append(certs, *info)
	}

	return certs, nil
}

// GetCertInfo reads and parses the X.509 certificate at the standard Let's
// Encrypt live path for a domain, returning structured metadata.
func (c *Client) GetCertInfo(domain string) (*CertInfo, error) {
	if err := validateDomain(domain); err != nil {
		return nil, fmt.Errorf("get cert info: %w", err)
	}

	liveDir := filepath.Join("/etc/letsencrypt/live", domain)

	certPath := filepath.Join(liveDir, "fullchain.pem")
	keyPath := filepath.Join(liveDir, "privkey.pem")
	chainPath := filepath.Join(liveDir, "chain.pem")

	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("get cert info: read cert: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("get cert info: no PEM block found in %s", certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("get cert info: parse x509: %w", err)
	}

	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	return &CertInfo{
		Domain:    domain,
		CertPath:  certPath,
		KeyPath:   keyPath,
		ChainPath: chainPath,
		ExpiresAt: cert.NotAfter,
		Issuer:    cert.Issuer.CommonName,
		DaysLeft:  daysLeft,
	}, nil
}

// EnsureCertbotInstalled checks whether certbot is available in PATH. If not,
// it attempts to install it via apt-get.
func (c *Client) EnsureCertbotInstalled() error {
	if _, err := exec.LookPath("certbot"); err == nil {
		return nil // Already installed
	}

	// Attempt to install via apt-get
	cmd := exec.Command("apt-get", "update", "-qq")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get update failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	cmd = exec.Command("apt-get", "install", "-y", "-qq", "certbot")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get install certbot failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Verify installation succeeded
	if _, err := exec.LookPath("certbot"); err != nil {
		return fmt.Errorf("certbot still not found after installation attempt")
	}

	return nil
}

// GenerateSelfSigned creates a self-signed X.509 certificate and private key
// for testing or as a fallback when Let's Encrypt is unavailable.
func (c *Client) GenerateSelfSigned(domain, certPath, keyPath string) error {
	if err := validateDomain(domain); err != nil {
		return fmt.Errorf("generate self-signed: %w", err)
	}

	// Ensure output directories exist
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("generate self-signed: create cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return fmt.Errorf("generate self-signed: create key dir: %w", err)
	}

	// Generate ECDSA P-256 private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate self-signed: generate key: %w", err)
	}

	// Build the certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate self-signed: serial number: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"Setec Security Labs"},
		},
		DNSNames:              []string{domain},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return fmt.Errorf("generate self-signed: create cert: %w", err)
	}

	// Write certificate PEM
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("generate self-signed: write cert: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("generate self-signed: encode cert PEM: %w", err)
	}

	// Write private key PEM
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("generate self-signed: marshal key: %w", err)
	}

	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("generate self-signed: write key: %w", err)
	}
	defer keyFile.Close()

	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return fmt.Errorf("generate self-signed: encode key PEM: %w", err)
	}

	return nil
}
