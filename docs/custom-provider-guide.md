# Custom Hosting Provider Guide

This guide walks you through creating a new hosting provider integration for Setec Manager. By the end, you will have a provider package that auto-registers with the system and can be used through the same unified API as the built-in Hostinger provider.

---

## Prerequisites

- Go 1.25+ (matching the project's `go.mod`)
- Familiarity with the Go `interface` pattern and HTTP client programming
- An API key or credentials for the hosting provider you are integrating
- A checkout of the `setec-manager` repository

---

## Project Structure

Provider implementations live under `internal/hosting/<provider_name>/`. Each provider is its own Go package.

```
internal/hosting/
  provider.go              -- Provider interface + model types + registry
  store.go                 -- ProviderConfig, ProviderConfigStore
  config.go                -- Legacy config store
  hostinger/               -- Built-in Hostinger provider
    client.go              --   HTTP client, auth, retry logic
    dns.go                 --   DNS record operations
  myprovider/              -- Your new provider (create this)
    provider.go            --   init() registration + interface methods
    client.go              --   HTTP client for the provider's API
    dns.go                 --   (optional) DNS-specific logic
    domains.go             --   (optional) Domain-specific logic
    vms.go                 --   (optional) VPS-specific logic
```

You can organize files however you like within the package; the only requirement is that the package calls `hosting.Register(...)` in an `init()` function.

---

## The Provider Interface

The `Provider` interface is defined in `internal/hosting/provider.go`. Every provider must implement all methods. Methods that your provider does not support should return `ErrNotSupported`.

```go
type Provider interface {
    // Identity
    Name() string
    DisplayName() string

    // Configuration
    Configure(config map[string]string) error
    TestConnection() error

    // DNS
    ListDNSRecords(domain string) ([]DNSRecord, error)
    CreateDNSRecord(domain string, record DNSRecord) error
    UpdateDNSRecords(domain string, records []DNSRecord, overwrite bool) error
    DeleteDNSRecord(domain string, recordName, recordType string) error
    ResetDNSRecords(domain string) error

    // Domains
    ListDomains() ([]Domain, error)
    GetDomain(domain string) (*Domain, error)
    CheckDomainAvailability(domains []string) ([]DomainAvailability, error)
    PurchaseDomain(req DomainPurchaseRequest) (*Domain, error)
    SetNameservers(domain string, nameservers []string) error
    EnableDomainLock(domain string) error
    DisableDomainLock(domain string) error
    EnablePrivacyProtection(domain string) error
    DisablePrivacyProtection(domain string) error

    // VMs / VPS
    ListVMs() ([]VM, error)
    GetVM(id string) (*VM, error)
    CreateVM(req VMCreateRequest) (*VM, error)
    ListDataCenters() ([]DataCenter, error)
    ListSSHKeys() ([]SSHKey, error)
    AddSSHKey(name, publicKey string) (*SSHKey, error)
    DeleteSSHKey(id string) error

    // Billing
    ListSubscriptions() ([]Subscription, error)
    GetCatalog() ([]CatalogItem, error)
}
```

### Method Reference

#### Identity Methods

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `Name()` | - | `string` | Short machine-readable name (lowercase, no spaces). Used as the registry key and in API URLs. Example: `"hostinger"`, `"cloudflare"`. |
| `DisplayName()` | - | `string` | Human-readable name shown in the UI. Example: `"Hostinger"`, `"Cloudflare"`. |

#### Configuration Methods

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `Configure(config)` | `map[string]string` -- key-value config pairs. Common keys: `"api_key"`, `"api_secret"`, `"base_url"`. | `error` | Called when a user saves credentials. Store them in struct fields. Validate format but do not make API calls. |
| `TestConnection()` | - | `error` | Make a lightweight API call (e.g., list domains) to verify credentials are valid. Return `nil` on success. |

#### DNS Methods

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `ListDNSRecords(domain)` | `domain string` -- the FQDN | `([]DNSRecord, error)` | Return all DNS records for the zone. |
| `CreateDNSRecord(domain, record)` | `domain string`, `record DNSRecord` | `error` | Add a single record without affecting existing records. |
| `UpdateDNSRecords(domain, records, overwrite)` | `domain string`, `records []DNSRecord`, `overwrite bool` | `error` | Batch update. If `overwrite` is true, replace all records; otherwise merge. |
| `DeleteDNSRecord(domain, recordName, recordType)` | `domain string`, `recordName string` (subdomain or `@`), `recordType string` (e.g. `"A"`) | `error` | Delete matching records. |
| `ResetDNSRecords(domain)` | `domain string` | `error` | Reset the zone to provider defaults. |

#### Domain Methods

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `ListDomains()` | - | `([]Domain, error)` | Return all domains on the account. |
| `GetDomain(domain)` | `domain string` | `(*Domain, error)` | Return details for a single domain. |
| `CheckDomainAvailability(domains)` | `domains []string` | `([]DomainAvailability, error)` | Check if domains are available for registration and return pricing. |
| `PurchaseDomain(req)` | `req DomainPurchaseRequest` | `(*Domain, error)` | Register a new domain. |
| `SetNameservers(domain, nameservers)` | `domain string`, `nameservers []string` | `error` | Update the authoritative nameservers. |
| `EnableDomainLock(domain)` | `domain string` | `error` | Enable registrar lock (transfer protection). |
| `DisableDomainLock(domain)` | `domain string` | `error` | Disable registrar lock. |
| `EnablePrivacyProtection(domain)` | `domain string` | `error` | Enable WHOIS privacy. |
| `DisablePrivacyProtection(domain)` | `domain string` | `error` | Disable WHOIS privacy. |

#### VM / VPS Methods

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `ListVMs()` | - | `([]VM, error)` | Return all VPS instances on the account. |
| `GetVM(id)` | `id string` | `(*VM, error)` | Return details for a single VM. |
| `CreateVM(req)` | `req VMCreateRequest` | `(*VM, error)` | Provision a new VPS instance. |
| `ListDataCenters()` | - | `([]DataCenter, error)` | Return available regions/data centers. |
| `ListSSHKeys()` | - | `([]SSHKey, error)` | Return all stored SSH public keys. |
| `AddSSHKey(name, publicKey)` | `name string`, `publicKey string` | `(*SSHKey, error)` | Upload a new SSH public key. |
| `DeleteSSHKey(id)` | `id string` | `error` | Remove an SSH key. |

#### Billing Methods

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `ListSubscriptions()` | - | `([]Subscription, error)` | Return all active subscriptions. |
| `GetCatalog()` | - | `([]CatalogItem, error)` | Return purchasable products and plans. |

---

## Type Reference

All model types are defined in `internal/hosting/provider.go`.

### DNSRecord

| Field | Type | JSON | Description |
|---|---|---|---|
| `ID` | `string` | `id` | Provider-assigned identifier. May be synthesized (e.g., `name/type/priority`). Optional on create. |
| `Type` | `string` | `type` | Record type: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `SRV`, `CAA`. |
| `Name` | `string` | `name` | Subdomain label or `@` for the zone apex. |
| `Content` | `string` | `content` | Record value (IP address, hostname, text, etc.). |
| `TTL` | `int` | `ttl` | Time-to-live in seconds. |
| `Priority` | `int` | `priority` | Priority value for MX and SRV records. Zero for other types. |

### Domain

| Field | Type | JSON | Description |
|---|---|---|---|
| `Name` | `string` | `name` | Fully qualified domain name. |
| `Registrar` | `string` | `registrar` | Registrar name (optional). |
| `Status` | `string` | `status` | Registration status (e.g., `"active"`, `"expired"`, `"pending"`). |
| `ExpiresAt` | `time.Time` | `expires_at` | Expiration date. |
| `AutoRenew` | `bool` | `auto_renew` | Whether automatic renewal is enabled. |
| `Locked` | `bool` | `locked` | Whether transfer lock is enabled. |
| `PrivacyProtection` | `bool` | `privacy_protection` | Whether WHOIS privacy is enabled. |
| `Nameservers` | `[]string` | `nameservers` | Current authoritative nameservers. |

### DomainAvailability

| Field | Type | JSON | Description |
|---|---|---|---|
| `Domain` | `string` | `domain` | The queried domain name. |
| `Available` | `bool` | `available` | Whether the domain is available for registration. |
| `Price` | `float64` | `price` | Purchase price (zero if unavailable). |
| `Currency` | `string` | `currency` | Currency code (e.g., `"USD"`). |

### DomainPurchaseRequest

| Field | Type | JSON | Description |
|---|---|---|---|
| `Domain` | `string` | `domain` | Domain to purchase. |
| `Period` | `int` | `period` | Registration period in years. |
| `AutoRenew` | `bool` | `auto_renew` | Enable auto-renewal. |
| `Privacy` | `bool` | `privacy_protection` | Enable WHOIS privacy. |
| `PaymentID` | `string` | `payment_method_id` | Payment method identifier (optional, provider-specific). |

### VM

| Field | Type | JSON | Description |
|---|---|---|---|
| `ID` | `string` | `id` | Provider-assigned VM identifier. |
| `Name` | `string` | `name` | Human-readable VM name / hostname. |
| `Status` | `string` | `status` | Current state: `"running"`, `"stopped"`, `"creating"`, `"error"`. |
| `Plan` | `string` | `plan` | Plan/tier identifier. |
| `Region` | `string` | `region` | Data center / region identifier. |
| `IPv4` | `string` | `ipv4` | Public IPv4 address (optional). |
| `IPv6` | `string` | `ipv6` | Public IPv6 address (optional). |
| `OS` | `string` | `os` | Operating system template name (optional). |
| `CPUs` | `int` | `cpus` | Number of virtual CPUs. |
| `MemoryMB` | `int` | `memory_mb` | RAM in megabytes. |
| `DiskGB` | `int` | `disk_gb` | Disk size in gigabytes. |
| `BandwidthGB` | `int` | `bandwidth_gb` | Monthly bandwidth allowance in gigabytes. |
| `CreatedAt` | `time.Time` | `created_at` | Creation timestamp. |
| `Labels` | `map[string]string` | `labels` | Arbitrary key-value labels (optional). |

### VMCreateRequest

| Field | Type | JSON | Description |
|---|---|---|---|
| `Plan` | `string` | `plan` | Plan/tier identifier from the catalog. |
| `DataCenterID` | `string` | `data_center_id` | Target data center from `ListDataCenters()`. |
| `Template` | `string` | `template` | OS template identifier. |
| `Password` | `string` | `password` | Root/admin password for the VM. |
| `Hostname` | `string` | `hostname` | Desired hostname. |
| `SSHKeyID` | `string` | `ssh_key_id` | SSH key to install (optional). |
| `PaymentID` | `string` | `payment_method_id` | Payment method identifier (optional). |

### DataCenter

| Field | Type | JSON | Description |
|---|---|---|---|
| `ID` | `string` | `id` | Unique identifier used in `VMCreateRequest`. |
| `Name` | `string` | `name` | Short name (e.g., `"US East"`). |
| `Location` | `string` | `location` | City or locality. |
| `Country` | `string` | `country` | ISO country code. |

### SSHKey

| Field | Type | JSON | Description |
|---|---|---|---|
| `ID` | `string` | `id` | Provider-assigned key identifier. |
| `Name` | `string` | `name` | User-assigned label. |
| `Fingerprint` | `string` | `fingerprint` | Key fingerprint (e.g., `"SHA256:..."`). |
| `PublicKey` | `string` | `public_key` | Full public key string. |

### Subscription

| Field | Type | JSON | Description |
|---|---|---|---|
| `ID` | `string` | `id` | Subscription identifier. |
| `Name` | `string` | `name` | Product name. |
| `Status` | `string` | `status` | Status: `"active"`, `"cancelled"`, `"expired"`. |
| `Plan` | `string` | `plan` | Plan identifier. |
| `Price` | `float64` | `price` | Recurring price. |
| `Currency` | `string` | `currency` | Currency code. |
| `RenewsAt` | `time.Time` | `renews_at` | Next renewal date. |
| `CreatedAt` | `time.Time` | `created_at` | Subscription start date. |

### CatalogItem

| Field | Type | JSON | Description |
|---|---|---|---|
| `ID` | `string` | `id` | Product/plan identifier. |
| `Name` | `string` | `name` | Product name. |
| `Category` | `string` | `category` | Category: `"vps"`, `"hosting"`, `"domain"`, etc. |
| `PriceCents` | `int` | `price_cents` | Price in cents (e.g., 1199 = $11.99). |
| `Currency` | `string` | `currency` | Currency code. |
| `Period` | `string` | `period` | Billing period: `"monthly"`, `"yearly"`. |
| `Description` | `string` | `description` | Human-readable description (optional). |

### ProviderConfig

Stored in `internal/hosting/store.go`. This is the credential record persisted to disk.

| Field | Type | JSON | Description |
|---|---|---|---|
| `Provider` | `string` | `provider` | Provider name (must match `Provider.Name()`). |
| `APIKey` | `string` | `api_key` | Primary API key or bearer token. |
| `APISecret` | `string` | `api_secret` | Secondary secret (optional, provider-specific). |
| `Extra` | `map[string]string` | `extra` | Additional provider-specific config values. |
| `Connected` | `bool` | `connected` | Whether the last `TestConnection()` succeeded. |

---

## Implementing the Interface

### Step 1: Create the Package

```bash
mkdir -p internal/hosting/myprovider
```

### Step 2: Implement the Provider

Create `internal/hosting/myprovider/provider.go`:

```go
package myprovider

import (
    "errors"
    "fmt"
    "net/http"
    "time"

    "setec-manager/internal/hosting"
)

// ErrNotSupported is returned by methods this provider does not implement.
var ErrNotSupported = errors.New("myprovider: operation not supported")

// Provider implements hosting.Provider for the MyProvider service.
type Provider struct {
    client  *http.Client
    apiKey  string
    baseURL string
}

// init registers this provider with the hosting registry.
// This runs automatically when the package is imported.
func init() {
    hosting.Register(&Provider{
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
        baseURL: "https://api.myprovider.com",
    })
}

// ── Identity ────────────────────────────────────────────────────────

func (p *Provider) Name() string        { return "myprovider" }
func (p *Provider) DisplayName() string { return "My Provider" }

// ── Configuration ───────────────────────────────────────────────────

func (p *Provider) Configure(config map[string]string) error {
    key, ok := config["api_key"]
    if !ok || key == "" {
        return fmt.Errorf("myprovider: api_key is required")
    }
    p.apiKey = key

    if baseURL, ok := config["base_url"]; ok && baseURL != "" {
        p.baseURL = baseURL
    }
    return nil
}

func (p *Provider) TestConnection() error {
    // Make a lightweight API call to verify credentials.
    // For example, list domains or get account info.
    _, err := p.ListDomains()
    return err
}

// ── DNS ─────────────────────────────────────────────────────────────

func (p *Provider) ListDNSRecords(domain string) ([]hosting.DNSRecord, error) {
    // TODO: Implement API call to list DNS records
    return nil, ErrNotSupported
}

func (p *Provider) CreateDNSRecord(domain string, record hosting.DNSRecord) error {
    return ErrNotSupported
}

func (p *Provider) UpdateDNSRecords(domain string, records []hosting.DNSRecord, overwrite bool) error {
    return ErrNotSupported
}

func (p *Provider) DeleteDNSRecord(domain string, recordName, recordType string) error {
    return ErrNotSupported
}

func (p *Provider) ResetDNSRecords(domain string) error {
    return ErrNotSupported
}

// ── Domains ─────────────────────────────────────────────────────────

func (p *Provider) ListDomains() ([]hosting.Domain, error) {
    return nil, ErrNotSupported
}

func (p *Provider) GetDomain(domain string) (*hosting.Domain, error) {
    return nil, ErrNotSupported
}

func (p *Provider) CheckDomainAvailability(domains []string) ([]hosting.DomainAvailability, error) {
    return nil, ErrNotSupported
}

func (p *Provider) PurchaseDomain(req hosting.DomainPurchaseRequest) (*hosting.Domain, error) {
    return nil, ErrNotSupported
}

func (p *Provider) SetNameservers(domain string, nameservers []string) error {
    return ErrNotSupported
}

func (p *Provider) EnableDomainLock(domain string) error  { return ErrNotSupported }
func (p *Provider) DisableDomainLock(domain string) error { return ErrNotSupported }
func (p *Provider) EnablePrivacyProtection(domain string) error  { return ErrNotSupported }
func (p *Provider) DisablePrivacyProtection(domain string) error { return ErrNotSupported }

// ── VMs / VPS ───────────────────────────────────────────────────────

func (p *Provider) ListVMs() ([]hosting.VM, error)                          { return nil, ErrNotSupported }
func (p *Provider) GetVM(id string) (*hosting.VM, error)                    { return nil, ErrNotSupported }
func (p *Provider) CreateVM(req hosting.VMCreateRequest) (*hosting.VM, error) { return nil, ErrNotSupported }
func (p *Provider) ListDataCenters() ([]hosting.DataCenter, error)          { return nil, ErrNotSupported }
func (p *Provider) ListSSHKeys() ([]hosting.SSHKey, error)                  { return nil, ErrNotSupported }
func (p *Provider) AddSSHKey(name, publicKey string) (*hosting.SSHKey, error) { return nil, ErrNotSupported }
func (p *Provider) DeleteSSHKey(id string) error                            { return ErrNotSupported }

// ── Billing ─────────────────────────────────────────────────────────

func (p *Provider) ListSubscriptions() ([]hosting.Subscription, error) { return nil, ErrNotSupported }
func (p *Provider) GetCatalog() ([]hosting.CatalogItem, error)         { return nil, ErrNotSupported }
```

---

## Registration

Registration happens automatically via Go's `init()` mechanism. When the main binary imports the provider package (even as a side-effect import), the `init()` function runs and calls `hosting.Register()`.

In `cmd/main.go` (or wherever the binary entry point is), add a blank import:

```go
import (
    // Register hosting providers
    _ "setec-manager/internal/hosting/hostinger"
    _ "setec-manager/internal/hosting/myprovider"
)
```

The `hosting.Register()` function stores the provider instance in a global `map[string]Provider` protected by a `sync.RWMutex`:

```go
// From internal/hosting/provider.go
func Register(p Provider) {
    registryMu.Lock()
    defer registryMu.Unlock()
    registry[p.Name()] = p
}
```

After registration, the provider is accessible via `hosting.Get("myprovider")` and appears in `hosting.List()`.

---

## Configuration Storage

When a user configures your provider (via the UI or API), the system:

1. Calls `provider.Configure(map[string]string{"api_key": "..."})` to set credentials in memory.
2. Calls `provider.TestConnection()` to verify the credentials work.
3. Saves a `ProviderConfig` to disk via `ProviderConfigStore.Save()`.

The config file is written to `<config_dir>/<provider_name>.json` with `0600` permissions:

```json
{
  "provider": "myprovider",
  "api_key": "sk-abc123...",
  "api_secret": "",
  "extra": {
    "base_url": "https://api.myprovider.com/v2"
  },
  "connected": true
}
```

On startup, `ProviderConfigStore.loadAll()` reads all JSON files from the config directory, and for each one that matches a registered provider, calls `Configure()` to restore credentials.

---

## Error Handling

### The ErrNotSupported Pattern

Define a sentinel error in your provider package:

```go
var ErrNotSupported = errors.New("myprovider: operation not supported")
```

Return this error from any interface method your provider does not implement. The HTTP handler layer checks for this error and returns HTTP 501 (Not Implemented) to the client.

### API Errors

For errors from the upstream provider API, return a descriptive error with context:

```go
return fmt.Errorf("myprovider: list domains: %w", err)
```

### Rate Limiting

If the provider has rate limits, handle them inside your client. See the Hostinger implementation in `internal/hosting/hostinger/client.go` for a reference pattern:

1. Check for HTTP 429 responses.
2. Read the `Retry-After` header.
3. Sleep and retry (up to a maximum number of retries).
4. Return a clear error if retries are exhausted.

```go
if resp.StatusCode == http.StatusTooManyRequests {
    retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
    if attempt < maxRetries {
        time.Sleep(retryAfter)
        continue
    }
    return fmt.Errorf("myprovider: rate limited after %d retries", maxRetries)
}
```

---

## Testing

### Unit Tests

Create `internal/hosting/myprovider/provider_test.go`:

```go
package myprovider

import (
    "testing"

    "setec-manager/internal/hosting"
)

func TestProviderImplementsInterface(t *testing.T) {
    var _ hosting.Provider = (*Provider)(nil)
}

func TestName(t *testing.T) {
    p := &Provider{}
    if p.Name() != "myprovider" {
        t.Errorf("expected name 'myprovider', got %q", p.Name())
    }
}

func TestConfigure(t *testing.T) {
    p := &Provider{}
    err := p.Configure(map[string]string{})
    if err == nil {
        t.Error("expected error when api_key is missing")
    }

    err = p.Configure(map[string]string{"api_key": "test-key"})
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if p.apiKey != "test-key" {
        t.Errorf("expected apiKey 'test-key', got %q", p.apiKey)
    }
}

func TestUnsupportedMethodsReturnError(t *testing.T) {
    p := &Provider{}

    _, err := p.ListVMs()
    if err != ErrNotSupported {
        t.Errorf("ListVMs: expected ErrNotSupported, got %v", err)
    }

    _, err = p.GetCatalog()
    if err != ErrNotSupported {
        t.Errorf("GetCatalog: expected ErrNotSupported, got %v", err)
    }
}
```

### Integration Tests

For integration tests against the real API, use build tags to prevent them from running in CI:

```go
//go:build integration

package myprovider

import (
    "os"
    "testing"
)

func TestListDomainsIntegration(t *testing.T) {
    key := os.Getenv("MYPROVIDER_API_KEY")
    if key == "" {
        t.Skip("MYPROVIDER_API_KEY not set")
    }

    p := &Provider{}
    p.Configure(map[string]string{"api_key": key})

    domains, err := p.ListDomains()
    if err != nil {
        t.Fatalf("ListDomains failed: %v", err)
    }
    t.Logf("Found %d domains", len(domains))
}
```

Run integration tests:

```bash
go test -tags=integration ./internal/hosting/myprovider/ -v
```

### Registration Test

Verify that importing the package registers the provider:

```go
package myprovider_test

import (
    "testing"

    "setec-manager/internal/hosting"
    _ "setec-manager/internal/hosting/myprovider"
)

func TestRegistration(t *testing.T) {
    p, err := hosting.Get("myprovider")
    if err != nil {
        t.Fatalf("provider not registered: %v", err)
    }
    if p.DisplayName() == "" {
        t.Error("DisplayName is empty")
    }
}
```

---

## Example: Skeleton Provider (DNS Only)

This is a complete, minimal provider that implements only DNS management. All other methods return `ErrNotSupported`. You can copy this file and fill in the DNS methods with real API calls.

```go
package dnsonlyprovider

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "time"

    "setec-manager/internal/hosting"
)

var ErrNotSupported = errors.New("dnsonlyprovider: operation not supported")

type Provider struct {
    client  *http.Client
    apiKey  string
    baseURL string
}

func init() {
    hosting.Register(&Provider{
        client:  &http.Client{Timeout: 30 * time.Second},
        baseURL: "https://api.dns-only.example.com/v1",
    })
}

func (p *Provider) Name() string        { return "dnsonlyprovider" }
func (p *Provider) DisplayName() string { return "DNS-Only Provider" }

func (p *Provider) Configure(config map[string]string) error {
    key, ok := config["api_key"]
    if !ok || key == "" {
        return fmt.Errorf("dnsonlyprovider: api_key is required")
    }
    p.apiKey = key
    return nil
}

func (p *Provider) TestConnection() error {
    // Try listing zones as a health check.
    req, _ := http.NewRequest("GET", p.baseURL+"/zones", nil)
    req.Header.Set("Authorization", "Bearer "+p.apiKey)
    resp, err := p.client.Do(req)
    if err != nil {
        return fmt.Errorf("dnsonlyprovider: connection failed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("dnsonlyprovider: API returned %d: %s", resp.StatusCode, body)
    }
    return nil
}

// ── DNS (implemented) ───────────────────────────────────────────────

func (p *Provider) ListDNSRecords(domain string) ([]hosting.DNSRecord, error) {
    req, _ := http.NewRequest("GET", fmt.Sprintf("%s/zones/%s/records", p.baseURL, domain), nil)
    req.Header.Set("Authorization", "Bearer "+p.apiKey)

    resp, err := p.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("dnsonlyprovider: list records: %w", err)
    }
    defer resp.Body.Close()

    var records []hosting.DNSRecord
    if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
        return nil, fmt.Errorf("dnsonlyprovider: parse records: %w", err)
    }
    return records, nil
}

func (p *Provider) CreateDNSRecord(domain string, record hosting.DNSRecord) error {
    // Implementation: POST to /zones/{domain}/records
    return ErrNotSupported // replace with real implementation
}

func (p *Provider) UpdateDNSRecords(domain string, records []hosting.DNSRecord, overwrite bool) error {
    // Implementation: PUT to /zones/{domain}/records
    return ErrNotSupported // replace with real implementation
}

func (p *Provider) DeleteDNSRecord(domain string, recordName, recordType string) error {
    // Implementation: DELETE /zones/{domain}/records?name=...&type=...
    return ErrNotSupported // replace with real implementation
}

func (p *Provider) ResetDNSRecords(domain string) error {
    return ErrNotSupported
}

// ── Everything else: not supported ──────────────────────────────────

func (p *Provider) ListDomains() ([]hosting.Domain, error)                                    { return nil, ErrNotSupported }
func (p *Provider) GetDomain(domain string) (*hosting.Domain, error)                          { return nil, ErrNotSupported }
func (p *Provider) CheckDomainAvailability(domains []string) ([]hosting.DomainAvailability, error) { return nil, ErrNotSupported }
func (p *Provider) PurchaseDomain(req hosting.DomainPurchaseRequest) (*hosting.Domain, error)  { return nil, ErrNotSupported }
func (p *Provider) SetNameservers(domain string, nameservers []string) error                   { return ErrNotSupported }
func (p *Provider) EnableDomainLock(domain string) error                                       { return ErrNotSupported }
func (p *Provider) DisableDomainLock(domain string) error                                      { return ErrNotSupported }
func (p *Provider) EnablePrivacyProtection(domain string) error                                { return ErrNotSupported }
func (p *Provider) DisablePrivacyProtection(domain string) error                               { return ErrNotSupported }
func (p *Provider) ListVMs() ([]hosting.VM, error)                                             { return nil, ErrNotSupported }
func (p *Provider) GetVM(id string) (*hosting.VM, error)                                       { return nil, ErrNotSupported }
func (p *Provider) CreateVM(req hosting.VMCreateRequest) (*hosting.VM, error)                  { return nil, ErrNotSupported }
func (p *Provider) ListDataCenters() ([]hosting.DataCenter, error)                             { return nil, ErrNotSupported }
func (p *Provider) ListSSHKeys() ([]hosting.SSHKey, error)                                     { return nil, ErrNotSupported }
func (p *Provider) AddSSHKey(name, publicKey string) (*hosting.SSHKey, error)                  { return nil, ErrNotSupported }
func (p *Provider) DeleteSSHKey(id string) error                                               { return ErrNotSupported }
func (p *Provider) ListSubscriptions() ([]hosting.Subscription, error)                         { return nil, ErrNotSupported }
func (p *Provider) GetCatalog() ([]hosting.CatalogItem, error)                                 { return nil, ErrNotSupported }
```

---

## Example: Full Provider Structure

For a provider that implements all capabilities, organize the code across multiple files:

```
internal/hosting/fullprovider/
  provider.go    -- init(), Name(), DisplayName(), Configure(), TestConnection()
  client.go      -- HTTP client with auth, retry, rate-limit handling
  dns.go         -- ListDNSRecords, CreateDNSRecord, UpdateDNSRecords, DeleteDNSRecord, ResetDNSRecords
  domains.go     -- ListDomains, GetDomain, CheckDomainAvailability, PurchaseDomain, nameserver/lock/privacy methods
  vms.go         -- ListVMs, GetVM, CreateVM, ListDataCenters
  ssh.go         -- ListSSHKeys, AddSSHKey, DeleteSSHKey
  billing.go     -- ListSubscriptions, GetCatalog
  types.go       -- Provider-specific API request/response types
```

Each file focuses on a single capability area. The `client.go` file provides a shared `doRequest()` method (similar to the Hostinger client) that handles authentication headers, JSON marshaling, error parsing, and retry logic.

### Key Patterns from the Hostinger Implementation

1. **Separate API types from generic types.** Define provider-specific request/response structs (e.g., `hostingerDNSRecord`) and conversion functions (`toGenericDNSRecord`, `toHostingerDNSRecord`).

2. **Validate before mutating.** The Hostinger DNS implementation calls a `/validate` endpoint before applying updates. If your provider offers similar validation, use it.

3. **Synthesize IDs when the API does not provide them.** Hostinger does not return record IDs in zone listings, so the client synthesizes them from `name/type/priority`.

4. **Handle rate limits transparently.** The client retries on HTTP 429 with exponential back-off, capping at 60 seconds per retry and 3 retries total. This keeps rate-limit handling invisible to the caller.
