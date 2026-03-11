# Hosting Provider Integration System

## Overview

Setec Manager includes a pluggable hosting provider architecture that lets you manage DNS records, domains, VPS instances, SSH keys, and billing subscriptions through a unified interface. The system is built around a Go `Provider` interface defined in `internal/hosting/provider.go`. Each hosting provider (e.g., Hostinger) implements this interface and auto-registers itself at import time via an `init()` function.

### Architecture

```
internal/hosting/
  provider.go    -- Provider interface, model types, global registry
  store.go       -- ProviderConfig type, ProviderConfigStore (disk persistence)
  config.go      -- Legacy config store (being superseded by store.go)
  hostinger/
    client.go    -- Hostinger HTTP client with retry/rate-limit handling
    dns.go       -- Hostinger DNS implementation
```

The registry is a process-global `map[string]Provider` guarded by a `sync.RWMutex`. Providers call `hosting.Register(&Provider{})` inside their package `init()` function. The main binary imports the provider package (e.g., `_ "setec-manager/internal/hosting/hostinger"`) to trigger registration.

Provider credentials are stored as individual JSON files in a protected directory (`0700` directory, `0600` files) managed by `ProviderConfigStore`. Each file is named `<provider>.json` and contains the `ProviderConfig` struct:

```json
{
  "provider": "hostinger",
  "api_key": "Bearer ...",
  "api_secret": "",
  "extra": {},
  "connected": true
}
```

---

## Supported Providers

### Hostinger (Built-in)

| Capability | Supported | Notes |
|---|---|---|
| DNS Management | Yes | Full CRUD, validation before writes, zone reset |
| Domain Management | Yes | List, lookup, availability check, purchase, nameservers, lock, privacy |
| VPS Management | Yes | List, create, get details, data center listing |
| SSH Key Management | Yes | Add, list, delete |
| Billing | Yes | Subscriptions and catalog |

The Hostinger provider communicates with `https://developers.hostinger.com` using a Bearer token. It includes automatic retry with back-off on HTTP 429 (rate limit) responses, up to 3 retries per request.

---

## Configuration

### Via the UI

1. Navigate to the Hosting Providers section in the Setec Manager dashboard.
2. Select "Hostinger" from the provider list.
3. Enter your API token (obtained from hPanel -- see [Hostinger Setup Guide](hostinger-setup.md)).
4. Click "Test Connection" to verify the token is valid.
5. Click "Save" to persist the configuration.

### Via Config Files

Provider configurations are stored as JSON files in the config directory (typically `/opt/setec-manager/data/hosting/`).

Create or edit the file directly:

```bash
mkdir -p /opt/setec-manager/data/hosting
cat > /opt/setec-manager/data/hosting/hostinger.json << 'EOF'
{
  "provider": "hostinger",
  "api_key": "YOUR_BEARER_TOKEN_HERE",
  "api_secret": "",
  "extra": {},
  "connected": true
}
EOF
chmod 600 /opt/setec-manager/data/hosting/hostinger.json
```

### Via API

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/configure \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "YOUR_HOSTINGER_API_TOKEN"
  }'
```

---

## API Reference

All hosting endpoints require authentication via JWT (cookie or `Authorization: Bearer` header). The base URL is `https://your-server:9090`.

### Provider Management

#### List Providers

```
GET /api/hosting/providers
```

Returns all registered hosting providers and their connection status.

```bash
curl -s https://your-server:9090/api/hosting/providers \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "name": "hostinger",
    "display_name": "Hostinger",
    "connected": true
  }
]
```

#### Configure Provider

```
POST /api/hosting/providers/{provider}/configure
```

Sets the API credentials for a provider.

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/configure \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "YOUR_API_TOKEN"
  }'
```

**Response:**
```json
{
  "status": "configured"
}
```

#### Test Connection

```
POST /api/hosting/providers/{provider}/test
```

Verifies that the saved credentials are valid by making a test API call.

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/test \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "ok",
  "message": "Connection successful"
}
```

#### Remove Provider Configuration

```
DELETE /api/hosting/providers/{provider}
```

Deletes saved credentials for a provider.

```bash
curl -X DELETE https://your-server:9090/api/hosting/providers/hostinger \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "deleted"
}
```

---

## DNS Management

### List DNS Records

```
GET /api/hosting/providers/{provider}/dns/{domain}
```

Returns all DNS records for the specified domain.

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/dns/example.com \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "id": "@/A/0",
    "type": "A",
    "name": "@",
    "content": "93.184.216.34",
    "ttl": 14400,
    "priority": 0
  },
  {
    "id": "www/CNAME/0",
    "type": "CNAME",
    "name": "www",
    "content": "example.com",
    "ttl": 14400,
    "priority": 0
  },
  {
    "id": "@/MX/10",
    "type": "MX",
    "name": "@",
    "content": "mail.example.com",
    "ttl": 14400,
    "priority": 10
  }
]
```

### Create DNS Record

```
POST /api/hosting/providers/{provider}/dns/{domain}
```

Adds a new DNS record without overwriting existing records.

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/dns/example.com \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "A",
    "name": "api",
    "content": "93.184.216.35",
    "ttl": 3600
  }'
```

**Response:**
```json
{
  "status": "created"
}
```

### Update DNS Records (Batch)

```
PUT /api/hosting/providers/{provider}/dns/{domain}
```

Updates DNS records for a domain. If `overwrite` is `true`, all existing records are replaced; otherwise the records are merged.

The Hostinger provider validates records against the API before applying changes.

```bash
curl -X PUT https://your-server:9090/api/hosting/providers/hostinger/dns/example.com \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "records": [
      {
        "type": "A",
        "name": "@",
        "content": "93.184.216.34",
        "ttl": 14400
      },
      {
        "type": "CNAME",
        "name": "www",
        "content": "example.com",
        "ttl": 14400
      }
    ],
    "overwrite": false
  }'
```

**Response:**
```json
{
  "status": "updated"
}
```

### Delete DNS Record

```
DELETE /api/hosting/providers/{provider}/dns/{domain}?name={name}&type={type}
```

Removes DNS records matching the given name and type.

```bash
curl -X DELETE "https://your-server:9090/api/hosting/providers/hostinger/dns/example.com?name=api&type=A" \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "deleted"
}
```

### Reset DNS Zone

```
POST /api/hosting/providers/{provider}/dns/{domain}/reset
```

Resets the domain's DNS zone to the provider's default records. This is destructive and removes all custom records.

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/dns/example.com/reset \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "reset"
}
```

### Supported DNS Record Types

| Type | Description | Priority Field |
|---|---|---|
| A | IPv4 address | No |
| AAAA | IPv6 address | No |
| CNAME | Canonical name / alias | No |
| MX | Mail exchange | Yes |
| TXT | Text record (SPF, DKIM, etc.) | No |
| NS | Name server | No |
| SRV | Service record | Yes |
| CAA | Certificate Authority Authorization | No |

---

## Domain Management

### List Domains

```
GET /api/hosting/providers/{provider}/domains
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/domains \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "name": "example.com",
    "registrar": "Hostinger",
    "status": "active",
    "expires_at": "2027-03-15T00:00:00Z",
    "auto_renew": true,
    "locked": true,
    "privacy_protection": true,
    "nameservers": ["ns1.dns-parking.com", "ns2.dns-parking.com"]
  }
]
```

### Get Domain Details

```
GET /api/hosting/providers/{provider}/domains/{domain}
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/domains/example.com \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "name": "example.com",
  "registrar": "Hostinger",
  "status": "active",
  "expires_at": "2027-03-15T00:00:00Z",
  "auto_renew": true,
  "locked": true,
  "privacy_protection": true,
  "nameservers": ["ns1.dns-parking.com", "ns2.dns-parking.com"]
}
```

### Check Domain Availability

```
POST /api/hosting/providers/{provider}/domains/check
```

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/check \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "domains": ["cool-project.com", "cool-project.io", "cool-project.dev"]
  }'
```

**Response:**
```json
[
  {
    "domain": "cool-project.com",
    "available": true,
    "price": 9.99,
    "currency": "USD"
  },
  {
    "domain": "cool-project.io",
    "available": false
  },
  {
    "domain": "cool-project.dev",
    "available": true,
    "price": 14.99,
    "currency": "USD"
  }
]
```

### Purchase Domain

```
POST /api/hosting/providers/{provider}/domains/purchase
```

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/purchase \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "cool-project.com",
    "period": 1,
    "auto_renew": true,
    "privacy_protection": true,
    "payment_method_id": "pm_abc123"
  }'
```

**Response:**
```json
{
  "name": "cool-project.com",
  "status": "active",
  "expires_at": "2027-03-11T00:00:00Z",
  "auto_renew": true,
  "locked": false,
  "privacy_protection": true,
  "nameservers": ["ns1.dns-parking.com", "ns2.dns-parking.com"]
}
```

### Set Nameservers

```
PUT /api/hosting/providers/{provider}/domains/{domain}/nameservers
```

```bash
curl -X PUT https://your-server:9090/api/hosting/providers/hostinger/domains/example.com/nameservers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "nameservers": ["ns1.cloudflare.com", "ns2.cloudflare.com"]
  }'
```

**Response:**
```json
{
  "status": "updated"
}
```

### Enable Domain Lock

```
POST /api/hosting/providers/{provider}/domains/{domain}/lock
```

Prevents unauthorized domain transfers.

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/example.com/lock \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "locked"
}
```

### Disable Domain Lock

```
DELETE /api/hosting/providers/{provider}/domains/{domain}/lock
```

```bash
curl -X DELETE https://your-server:9090/api/hosting/providers/hostinger/domains/example.com/lock \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "unlocked"
}
```

### Enable Privacy Protection

```
POST /api/hosting/providers/{provider}/domains/{domain}/privacy
```

Enables WHOIS privacy protection to hide registrant details.

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/example.com/privacy \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "enabled"
}
```

### Disable Privacy Protection

```
DELETE /api/hosting/providers/{provider}/domains/{domain}/privacy
```

```bash
curl -X DELETE https://your-server:9090/api/hosting/providers/hostinger/domains/example.com/privacy \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "disabled"
}
```

---

## VPS Management

### List Virtual Machines

```
GET /api/hosting/providers/{provider}/vms
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/vms \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "id": "vm-abc123",
    "name": "production-1",
    "status": "running",
    "plan": "kvm-2",
    "region": "us-east-1",
    "ipv4": "93.184.216.34",
    "ipv6": "2606:2800:220:1:248:1893:25c8:1946",
    "os": "Ubuntu 22.04",
    "cpus": 2,
    "memory_mb": 4096,
    "disk_gb": 80,
    "bandwidth_gb": 4000,
    "created_at": "2025-01-15T10:30:00Z",
    "labels": {
      "env": "production"
    }
  }
]
```

### Get VM Details

```
GET /api/hosting/providers/{provider}/vms/{id}
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/vms/vm-abc123 \
  -H "Authorization: Bearer $TOKEN"
```

**Response:** Same shape as a single item from the list response.

### Create VM

```
POST /api/hosting/providers/{provider}/vms
```

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/vms \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "plan": "kvm-2",
    "data_center_id": "us-east-1",
    "template": "ubuntu-22.04",
    "password": "SecurePassword123!",
    "hostname": "web-server-2",
    "ssh_key_id": "key-abc123",
    "payment_method_id": "pm_abc123"
  }'
```

**Response:**
```json
{
  "id": "vm-def456",
  "name": "web-server-2",
  "status": "creating",
  "plan": "kvm-2",
  "region": "us-east-1",
  "os": "Ubuntu 22.04",
  "cpus": 2,
  "memory_mb": 4096,
  "disk_gb": 80,
  "bandwidth_gb": 4000,
  "created_at": "2026-03-11T14:00:00Z"
}
```

### List Data Centers

```
GET /api/hosting/providers/{provider}/datacenters
```

Returns available regions/data centers for VM creation.

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/datacenters \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "id": "us-east-1",
    "name": "US East",
    "location": "New York",
    "country": "US"
  },
  {
    "id": "eu-west-1",
    "name": "EU West",
    "location": "Amsterdam",
    "country": "NL"
  }
]
```

---

## SSH Key Management

### List SSH Keys

```
GET /api/hosting/providers/{provider}/ssh-keys
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/ssh-keys \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "id": "key-abc123",
    "name": "deploy-key",
    "fingerprint": "SHA256:abcd1234...",
    "public_key": "ssh-ed25519 AAAAC3Nz..."
  }
]
```

### Add SSH Key

```
POST /api/hosting/providers/{provider}/ssh-keys
```

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/ssh-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "new-deploy-key",
    "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host"
  }'
```

**Response:**
```json
{
  "id": "key-def456",
  "name": "new-deploy-key",
  "fingerprint": "SHA256:efgh5678...",
  "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI..."
}
```

### Delete SSH Key

```
DELETE /api/hosting/providers/{provider}/ssh-keys/{id}
```

```bash
curl -X DELETE https://your-server:9090/api/hosting/providers/hostinger/ssh-keys/key-def456 \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "status": "deleted"
}
```

---

## Billing

### List Subscriptions

```
GET /api/hosting/providers/{provider}/subscriptions
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/subscriptions \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "id": "sub-abc123",
    "name": "Premium Web Hosting",
    "status": "active",
    "plan": "premium-hosting-48m",
    "price": 2.99,
    "currency": "USD",
    "renews_at": "2027-03-15T00:00:00Z",
    "created_at": "2023-03-15T00:00:00Z"
  }
]
```

### Get Product Catalog

```
GET /api/hosting/providers/{provider}/catalog
```

```bash
curl -s https://your-server:9090/api/hosting/providers/hostinger/catalog \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
[
  {
    "id": "kvm-2",
    "name": "KVM 2",
    "category": "vps",
    "price_cents": 1199,
    "currency": "USD",
    "period": "monthly",
    "description": "2 vCPU, 4 GB RAM, 80 GB SSD"
  },
  {
    "id": "premium-hosting-12m",
    "name": "Premium Web Hosting",
    "category": "hosting",
    "price_cents": 299,
    "currency": "USD",
    "period": "monthly",
    "description": "100 websites, 100 GB SSD, free SSL"
  }
]
```

---

## Error Responses

All endpoints return errors in a consistent format:

```json
{
  "error": "description of what went wrong"
}
```

| HTTP Status | Meaning |
|---|---|
| 400 | Bad request (invalid parameters) |
| 401 | Authentication required or token invalid |
| 404 | Provider or resource not found |
| 409 | Conflict (e.g., duplicate resource) |
| 429 | Rate limited by the upstream provider |
| 500 | Internal server error |
| 501 | Provider does not support this operation (`ErrNotSupported`) |

When a provider does not implement a particular capability, the endpoint returns HTTP 501 with an `ErrNotSupported` error message. This allows partial implementations where a provider only supports DNS management, for example.
