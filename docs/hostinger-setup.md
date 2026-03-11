# Hostinger Setup Guide

This guide covers configuring the Hostinger hosting provider integration in Setec Manager.

---

## Getting Your API Token

Hostinger provides API access through bearer tokens generated in the hPanel control panel.

### Step-by-Step

1. **Log in to hPanel.** Go to [https://hpanel.hostinger.com](https://hpanel.hostinger.com) and sign in with your Hostinger account.

2. **Navigate to your profile.** Click your profile icon or name in the top-right corner of the dashboard.

3. **Open Account Settings.** Select "Account Settings" or "Profile" from the dropdown menu.

4. **Go to the API section.** Look for the "API" or "API Tokens" tab. This may be under "Account" > "API" depending on your hPanel version.

5. **Generate a new token.** Click "Create API Token" or "Generate Token."
   - Give the token a descriptive name (e.g., `setec-manager`).
   - Select the permissions/scopes you need. For full Setec Manager integration, grant:
     - DNS management (read/write)
     - Domain management (read/write)
     - VPS management (read/write)
     - Billing (read)
   - Set an expiration if desired (recommended: no expiration for server-to-server use, but rotate periodically).

6. **Copy the token.** The token is shown only once. Copy it immediately and store it securely. It will look like a long alphanumeric string.

**Important:** Treat this token like a password. Anyone with the token has API access to your Hostinger account.

---

## Configuring in Setec Manager

### Via the Web UI

1. Log in to your Setec Manager dashboard at `https://your-server:9090`.
2. Navigate to the Hosting Providers section.
3. Click "Hostinger" from the provider list.
4. Paste your API token into the "API Key" field.
5. Click "Test Connection" -- you should see a success message confirming the token is valid.
6. Click "Save Configuration" to persist the credentials.

### Via the API

```bash
# Set your Setec Manager JWT token
export TOKEN="your-setec-manager-jwt"

# Configure the Hostinger provider
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/configure \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "YOUR_HOSTINGER_BEARER_TOKEN"
  }'

# Verify the connection
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/test \
  -H "Authorization: Bearer $TOKEN"
```

### Via Config File

Create the config file directly on the server:

```bash
sudo mkdir -p /opt/setec-manager/data/hosting
sudo tee /opt/setec-manager/data/hosting/hostinger.json > /dev/null << 'EOF'
{
  "provider": "hostinger",
  "api_key": "YOUR_HOSTINGER_BEARER_TOKEN",
  "api_secret": "",
  "extra": {},
  "connected": true
}
EOF
sudo chmod 600 /opt/setec-manager/data/hosting/hostinger.json
```

Restart Setec Manager for the config to be loaded:

```bash
sudo systemctl restart setec-manager
```

---

## Available Features

The Hostinger provider supports all major integration capabilities:

| Feature | Status | Notes |
|---|---|---|
| DNS Record Listing | Supported | Lists all records in a zone |
| DNS Record Creation | Supported | Adds records without overwriting |
| DNS Record Update (Batch) | Supported | Validates before applying; supports overwrite mode |
| DNS Record Deletion | Supported | Filter by name and/or type |
| DNS Zone Reset | Supported | Resets to Hostinger default records |
| Domain Listing | Supported | All domains on the account |
| Domain Details | Supported | Full WHOIS and registration info |
| Domain Availability Check | Supported | Batch check with pricing |
| Domain Purchase | Supported | Requires valid payment method |
| Nameserver Management | Supported | Update authoritative nameservers |
| Domain Lock | Supported | Enable/disable transfer lock |
| Privacy Protection | Supported | Enable/disable WHOIS privacy |
| VPS Listing | Supported | All VPS instances |
| VPS Details | Supported | Full specs, IP, status |
| VPS Creation | Supported | Requires plan, template, data center |
| Data Center Listing | Supported | Available regions for VM creation |
| SSH Key Management | Supported | Add, list, delete public keys |
| Subscription Listing | Supported | Active billing subscriptions |
| Product Catalog | Supported | Available plans and pricing |

---

## Rate Limits

The Hostinger API enforces rate limiting on all endpoints. The Setec Manager integration handles rate limits automatically:

- **Detection:** HTTP 429 (Too Many Requests) responses are detected.
- **Retry-After header:** The client reads the `Retry-After` header to determine how long to wait.
- **Automatic retry:** Up to 3 retries are attempted with the specified back-off.
- **Back-off cap:** Individual retry delays are capped at 60 seconds.
- **Failure:** If all retries are exhausted, the error is returned to the caller.

### Best Practices

- Avoid rapid-fire bulk operations. Space out batch DNS updates.
- Use the batch `UpdateDNSRecords` endpoint with multiple records in one call instead of creating records one at a time.
- Cache domain and VM listings on the client side when possible.
- If you see frequent 429 errors in logs, reduce the frequency of polling operations.

---

## DNS Record Management

### Hostinger API Endpoints Used

| Operation | Hostinger API Path |
|---|---|
| List records | `GET /api/dns/v1/zones/{domain}` |
| Update records | `PUT /api/dns/v1/zones/{domain}` |
| Validate records | `POST /api/dns/v1/zones/{domain}/validate` |
| Delete records | `DELETE /api/dns/v1/zones/{domain}` |
| Reset zone | `POST /api/dns/v1/zones/{domain}/reset` |

### Supported Record Types

| Type | Example Content | Priority | Notes |
|---|---|---|---|
| A | `93.184.216.34` | No | IPv4 address |
| AAAA | `2606:2800:220:1::` | No | IPv6 address |
| CNAME | `example.com` | No | Must be a hostname, not an IP |
| MX | `mail.example.com` | Yes | Priority determines delivery order (lower = higher priority) |
| TXT | `v=spf1 include:...` | No | Used for SPF, DKIM, domain verification |
| NS | `ns1.example.com` | No | Nameserver delegation |
| SRV | `sip.example.com` | Yes | Service location records |
| CAA | `letsencrypt.org` | No | Certificate authority authorization |

### Record ID Synthesis

Hostinger does not return unique record IDs in zone listings. Setec Manager synthesizes an ID from `name/type/priority` for each record. For example, an MX record for the root domain with priority 10 gets the ID `@/MX/10`. This ID is used internally for tracking but should not be passed back to the Hostinger API.

### Validation Before Write

The Hostinger provider validates DNS records before applying changes. When you call `UpdateDNSRecords`, the system:

1. Converts generic `DNSRecord` structs to Hostinger-specific format.
2. Sends the records to the `/validate` endpoint.
3. If validation passes, sends the actual update to the zone endpoint.
4. If validation fails, returns the validation error without modifying the zone.

This prevents malformed records from corrupting your DNS zone.

---

## Domain Management

### Purchasing Domains

Before purchasing a domain:

1. Check availability using the availability check endpoint.
2. Note the price and currency in the response.
3. Ensure you have a valid payment method configured in your Hostinger account.
4. Submit the purchase request with the `payment_method_id` from your Hostinger account.

```bash
# Check availability
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/check \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"domains": ["my-new-site.com"]}'

# Purchase (if available)
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/purchase \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "my-new-site.com",
    "period": 1,
    "auto_renew": true,
    "privacy_protection": true
  }'
```

### Domain Transfers

Domain transfers are initiated outside of Setec Manager through the Hostinger hPanel. Once a domain is transferred to your Hostinger account, it will appear in `ListDomains` and can be managed through Setec Manager.

### WHOIS Privacy

Hostinger offers WHOIS privacy protection (also called "Domain Privacy Protection") that replaces your personal contact information in WHOIS records with proxy information. Enable it to keep your registrant details private:

```bash
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/domains/example.com/privacy \
  -H "Authorization: Bearer $TOKEN"
```

---

## VPS Management

### Creating a VM

To create a VPS instance, you need three pieces of information:

1. **Plan ID** -- Get from the catalog endpoint (`GET /api/hosting/providers/hostinger/catalog`).
2. **Data Center ID** -- Get from the data centers endpoint (`GET /api/hosting/providers/hostinger/datacenters`).
3. **Template** -- The OS template name (e.g., `"ubuntu-22.04"`, `"debian-12"`, `"centos-9"`).

```bash
# List available plans
curl -s https://your-server:9090/api/hosting/providers/hostinger/catalog \
  -H "Authorization: Bearer $TOKEN" | jq '.[] | select(.category == "vps")'

# List data centers
curl -s https://your-server:9090/api/hosting/providers/hostinger/datacenters \
  -H "Authorization: Bearer $TOKEN"

# Create the VM
curl -X POST https://your-server:9090/api/hosting/providers/hostinger/vms \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "plan": "kvm-2",
    "data_center_id": "us-east-1",
    "template": "ubuntu-22.04",
    "password": "YourSecurePassword!",
    "hostname": "app-server",
    "ssh_key_id": "key-abc123"
  }'
```

### Docker Support

Hostinger VPS instances support Docker out of the box on Linux templates. After creating a VM:

1. SSH into the new VM.
2. Install Docker using the standard installation method for your chosen OS.
3. Alternatively, select a Docker-optimized template if available in your Hostinger account.

### VM Status Values

| Status | Description |
|---|---|
| `running` | VM is powered on and operational |
| `stopped` | VM is powered off |
| `creating` | VM is being provisioned (may take a few minutes) |
| `error` | VM encountered an error during provisioning |
| `suspended` | VM is suspended (usually billing-related) |

---

## Troubleshooting

### Common Errors

#### "hostinger API error 401: Unauthorized"

**Cause:** The API token is invalid, expired, or revoked.

**Fix:**
1. Log in to hPanel and verify the token exists and is not expired.
2. Generate a new token if needed.
3. Update the configuration in Setec Manager.

#### "hostinger API error 403: Forbidden"

**Cause:** The API token does not have the required permissions/scopes.

**Fix:**
1. Check the token's permissions in hPanel.
2. Ensure the token has read/write access for the feature you are trying to use (DNS, domains, VPS, billing).
3. Generate a new token with the correct scopes if needed.

#### "hostinger API error 429: rate limited"

**Cause:** Too many API requests in a short period.

**Fix:**
- The client retries automatically up to 3 times. If you still see this error, you are making requests too frequently.
- Space out bulk operations.
- Use batch endpoints (e.g., `UpdateDNSRecords` with multiple records) instead of individual calls.

#### "hostinger API error 404: Not Found"

**Cause:** The domain, VM, or resource does not exist in your Hostinger account.

**Fix:**
- Verify the domain is registered with Hostinger (not just DNS-hosted).
- Check that the VM ID is correct.
- Ensure the domain's DNS zone is active in Hostinger.

#### "validate DNS records: hostinger API error 422"

**Cause:** One or more DNS records failed validation.

**Fix:**
- Check record types are valid (A, AAAA, CNAME, MX, TXT, NS, SRV, CAA).
- Verify content format matches the record type (e.g., A records must be valid IPv4 addresses).
- Ensure TTL is a positive integer.
- MX and SRV records require a priority value.
- CNAME records cannot coexist with other record types at the same name.

#### "connection failed" or "execute request" errors

**Cause:** Network connectivity issue between Setec Manager and `developers.hostinger.com`.

**Fix:**
- Verify the server has outbound HTTPS access.
- Check DNS resolution: `dig developers.hostinger.com`.
- Check if a firewall is blocking outbound port 443.
- Verify the server's system clock is accurate (TLS certificate validation requires correct time).

#### "hosting provider 'hostinger' not registered"

**Cause:** The Hostinger provider package was not imported in the binary.

**Fix:**
- Ensure `cmd/main.go` includes the blank import: `_ "setec-manager/internal/hosting/hostinger"`.
- Rebuild and restart Setec Manager.

### Checking Logs

Setec Manager logs hosting provider operations to the configured log file (default: `/var/log/setec-manager.log`). Look for lines containing `hostinger` or `hosting`:

```bash
grep -i hostinger /var/log/setec-manager.log | tail -20
```

### Testing Connectivity Manually

You can test the Hostinger API directly from the server to rule out Setec Manager issues:

```bash
curl -s -H "Authorization: Bearer YOUR_HOSTINGER_TOKEN" \
  https://developers.hostinger.com/api/dns/v1/zones/your-domain.com
```

If this succeeds but Setec Manager fails, the issue is in the Setec Manager configuration. If this also fails, the issue is with the token or network connectivity.
