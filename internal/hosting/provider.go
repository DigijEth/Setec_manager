package hosting

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrNotSupported is returned when a provider does not support a given operation.
var ErrNotSupported = errors.New("operation not supported by this provider")

// Provider is the interface all hosting service integrations must implement.
// Not all providers support all features -- methods should return ErrNotSupported
// for unsupported operations.
type Provider interface {
	// Name returns the provider identifier (e.g. "hostinger", "digitalocean").
	Name() string

	// DisplayName returns a human-readable provider name.
	DisplayName() string

	// --- Authentication ---

	// Configure applies the given configuration to the provider.
	Configure(cfg ProviderConfig) error

	// TestConnection verifies that the provider credentials are valid.
	TestConnection() error

	// --- DNS Management ---

	// ListDNSRecords returns all DNS records for a domain.
	ListDNSRecords(domain string) ([]DNSRecord, error)

	// CreateDNSRecord adds a single DNS record to a domain.
	CreateDNSRecord(domain string, record DNSRecord) error

	// UpdateDNSRecords replaces DNS records for a domain. If overwrite is true,
	// all existing records are removed first.
	UpdateDNSRecords(domain string, records []DNSRecord, overwrite bool) error

	// DeleteDNSRecord removes DNS records matching the filter.
	DeleteDNSRecord(domain string, filter DNSRecordFilter) error

	// ResetDNSRecords restores the default DNS records for a domain.
	ResetDNSRecords(domain string) error

	// --- Domain Management ---

	// ListDomains returns all domains on the account.
	ListDomains() ([]Domain, error)

	// GetDomain returns detailed information about a single domain.
	GetDomain(domain string) (*DomainDetail, error)

	// CheckDomainAvailability checks registration availability across TLDs.
	CheckDomainAvailability(domain string, tlds []string) ([]DomainAvailability, error)

	// PurchaseDomain registers a new domain.
	PurchaseDomain(req DomainPurchaseRequest) (*OrderResult, error)

	// SetNameservers configures the nameservers for a domain.
	SetNameservers(domain string, nameservers []string) error

	// EnableDomainLock enables the registrar lock on a domain.
	EnableDomainLock(domain string) error

	// DisableDomainLock disables the registrar lock on a domain.
	DisableDomainLock(domain string) error

	// EnablePrivacyProtection enables WHOIS privacy for a domain.
	EnablePrivacyProtection(domain string) error

	// DisablePrivacyProtection disables WHOIS privacy for a domain.
	DisablePrivacyProtection(domain string) error

	// --- VPS Management ---

	// ListVMs returns all virtual machines on the account.
	ListVMs() ([]VirtualMachine, error)

	// GetVM returns details for a single virtual machine.
	GetVM(id string) (*VirtualMachine, error)

	// CreateVM provisions a new virtual machine.
	CreateVM(req VMCreateRequest) (*OrderResult, error)

	// ListDataCenters returns available data center locations.
	ListDataCenters() ([]DataCenter, error)

	// --- SSH Keys ---

	// ListSSHKeys returns all SSH keys on the account.
	ListSSHKeys() ([]SSHKey, error)

	// AddSSHKey uploads a new SSH public key.
	AddSSHKey(name, publicKey string) (*SSHKey, error)

	// DeleteSSHKey removes an SSH key by ID.
	DeleteSSHKey(id string) error

	// --- Billing ---

	// ListSubscriptions returns all active subscriptions.
	ListSubscriptions() ([]Subscription, error)

	// GetCatalog returns available products in a category.
	GetCatalog(category string) ([]CatalogItem, error)
}

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

// ProviderConfig holds the credentials and settings needed to connect to a
// hosting provider.
type ProviderConfig struct {
	APIKey    string            `json:"api_key"`
	APISecret string            `json:"api_secret,omitempty"`
	BaseURL   string            `json:"base_url,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// DNSRecord represents a single DNS record.
type DNSRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`              // A, AAAA, CNAME, MX, TXT, etc.
	Content  string `json:"content"`
	TTL      int    `json:"ttl,omitempty"`      // seconds; 0 means provider default
	Priority int    `json:"priority,omitempty"` // used by MX, SRV
}

// DNSRecordFilter identifies DNS records to match for deletion or lookup.
type DNSRecordFilter struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// Domain is a summary of a domain on the account.
type Domain struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DomainDetail contains full information about a domain registration.
type DomainDetail struct {
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	Registrar         string    `json:"registrar"`
	RegisteredAt      time.Time `json:"registered_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	AutoRenew         bool      `json:"auto_renew"`
	Locked            bool      `json:"locked"`
	PrivacyProtection bool      `json:"privacy_protection"`
	Nameservers       []string  `json:"nameservers"`
}

// DomainAvailability reports whether a domain + TLD combination can be
// registered and its price.
type DomainAvailability struct {
	Domain    string  `json:"domain"`
	TLD       string  `json:"tld"`
	Available bool    `json:"available"`
	Price     float64 `json:"price"`    // in the provider's default currency
	Currency  string  `json:"currency"`
}

// DomainPurchaseRequest contains everything needed to register a domain.
type DomainPurchaseRequest struct {
	Domain        string `json:"domain"`
	Years         int    `json:"years"`
	AutoRenew     bool   `json:"auto_renew"`
	Privacy       bool   `json:"privacy"`
	PaymentMethod string `json:"payment_method,omitempty"`
}

// OrderResult is returned after a purchase or provisioning request.
type OrderResult struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"` // e.g. "pending", "completed", "failed"
	Message string `json:"message,omitempty"`
}

// VirtualMachine represents a VPS instance.
type VirtualMachine struct {
	ID         string    `json:"id"`
	Hostname   string    `json:"hostname"`
	IPAddress  string    `json:"ip_address"`
	IPv6       string    `json:"ipv6,omitempty"`
	Status     string    `json:"status"` // running, stopped, provisioning, etc.
	Plan       string    `json:"plan"`
	DataCenter string    `json:"data_center"`
	OS         string    `json:"os"`
	CPUs       int       `json:"cpus"`
	RAMBytes   int64     `json:"ram_bytes"`
	DiskBytes  int64     `json:"disk_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

// VMCreateRequest contains everything needed to provision a new VPS.
type VMCreateRequest struct {
	Hostname     string `json:"hostname"`
	Plan         string `json:"plan"`
	DataCenterID string `json:"data_center_id"`
	OS           string `json:"os"`
	SSHKeyID     string `json:"ssh_key_id,omitempty"`
	Password     string `json:"password,omitempty"`
}

// DataCenter represents a physical hosting location.
type DataCenter struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"` // city or region
	Country  string `json:"country"`
}

// SSHKey is a stored SSH public key.
type SSHKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
}

// Subscription represents a billing subscription.
type Subscription struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Status   string    `json:"status"` // active, cancelled, expired
	RenewsAt time.Time `json:"renews_at"`
	Price    float64   `json:"price"`
	Currency string    `json:"currency"`
}

// CatalogItem is a purchasable product or plan.
type CatalogItem struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Category string            `json:"category"`
	Price    float64           `json:"price"`
	Currency string            `json:"currency"`
	Features map[string]string `json:"features,omitempty"`
}

// ---------------------------------------------------------------------------
// Provider registry
// ---------------------------------------------------------------------------

var (
	mu        sync.RWMutex
	providers = map[string]Provider{}
)

// Register adds a provider to the global registry. It panics if a provider
// with the same name is already registered.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	name := p.Name()
	if _, exists := providers[name]; exists {
		panic("hosting: provider already registered: " + name)
	}
	providers[name] = p
}

// Get returns a registered provider by name.
func Get(name string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[name]
	return p, ok
}

// List returns the names of all registered providers, sorted alphabetically.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
