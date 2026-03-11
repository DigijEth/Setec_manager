# Setec Manager API Reference

Complete REST API reference for the Setec Manager server management panel.

**Base URL:** `https://your-server:9090`

**Authentication:** Most endpoints require a valid JWT token, provided either as a cookie (`setec_token`) or an `Authorization: Bearer <token>` header. Tokens are issued by the login endpoint and expire after 24 hours.

**Content Types:** All JSON request bodies should use `Content-Type: application/json`. Responses are `application/json` unless otherwise noted. Some endpoints also serve HTML pages when the `Accept` header includes `text/html`.

---

## Table of Contents

1. [Authentication](#authentication)
2. [Dashboard](#dashboard)
3. [Sites](#sites)
4. [AUTARCH](#autarch)
5. [SSL/TLS](#ssltls)
6. [Nginx](#nginx)
7. [Firewall](#firewall)
8. [Users](#users)
9. [Backups](#backups)
10. [Monitoring](#monitoring)
11. [Logs](#logs)
12. [Float Mode](#float-mode)
13. [Hosting Providers](#hosting-providers)

---

## Authentication

### Login

```
POST /login
```

**Auth required:** No

**Rate limited:** Yes (5 attempts per IP per minute)

**Request body:**
```json
{
  "username": "admin",
  "password": "your-password"
}
```

**Response (200):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "username": "admin",
  "role": "admin"
}
```

A `setec_token` cookie is also set with `HttpOnly`, `Secure`, and `SameSite=Strict` flags. The cookie expires in 24 hours.

```bash
curl -X POST https://your-server:9090/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "your-password"}'
```

### Logout

```
POST /logout
```

**Auth required:** No (clears the cookie regardless)

**Response (200):**
```json
{
  "status": "logged out"
}
```

```bash
curl -X POST https://your-server:9090/logout \
  -H "Authorization: Bearer $TOKEN"
```

### Auth Status

```
GET /api/auth/status
```

**Auth required:** Yes

**Response (200):**
```json
{
  "user_id": 1,
  "username": "admin",
  "role": "admin"
}
```

```bash
curl -s https://your-server:9090/api/auth/status \
  -H "Authorization: Bearer $TOKEN"
```

---

## Dashboard

### Dashboard Page

```
GET /
```

**Auth required:** Yes

Returns the dashboard HTML page (or JSON if `Accept: application/json`).

### System Info

```
GET /api/system/info
```

**Auth required:** Yes

**Response (200):**
```json
{
  "hostname": "vps-12345",
  "os": "linux",
  "arch": "amd64",
  "cpus": 4,
  "uptime": "up 15d 3h 42m",
  "load_avg": "0.45 0.38 0.32",
  "mem_total": "7.8 GB",
  "mem_used": "3.2 GB",
  "mem_percent": 41.0,
  "disk_total": "78 GB",
  "disk_used": "23 GB",
  "disk_percent": 29.5,
  "site_count": 5,
  "services": [
    {"name": "Nginx", "status": "active", "running": true},
    {"name": "AUTARCH Web", "status": "active", "running": true},
    {"name": "AUTARCH DNS", "status": "inactive", "running": false},
    {"name": "Setec Manager", "status": "active", "running": true}
  ]
}
```

```bash
curl -s https://your-server:9090/api/system/info \
  -H "Authorization: Bearer $TOKEN"
```

---

## Sites

### List Sites

```
GET /sites
```

**Auth required:** Yes

**Response (200):**
```json
[
  {
    "id": 1,
    "domain": "example.com",
    "aliases": "www.example.com",
    "app_type": "python",
    "app_root": "/var/www/example.com",
    "app_port": 8000,
    "app_entry": "app.py",
    "git_repo": "https://github.com/user/repo.git",
    "git_branch": "main",
    "ssl_enabled": true,
    "enabled": true,
    "running": true,
    "status": "active"
  }
]
```

```bash
curl -s https://your-server:9090/sites \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

### Create Site

```
POST /sites
```

**Auth required:** Yes

**Request body:**
```json
{
  "domain": "newsite.com",
  "aliases": "www.newsite.com",
  "app_type": "python",
  "app_root": "/var/www/newsite.com",
  "app_entry": "app.py",
  "git_repo": "https://github.com/user/newsite.git",
  "git_branch": "main"
}
```

| Field | Required | Default | Description |
|---|---|---|---|
| `domain` | Yes | - | Primary domain name |
| `aliases` | No | `""` | Space-separated domain aliases |
| `app_type` | No | `"static"` | Application type: `static`, `python`, `node`, `autarch` |
| `app_root` | No | `/var/www/{domain}` | Application root directory |
| `app_entry` | No | `""` | Entry point file (e.g., `app.py`, `server.js`) |
| `app_port` | No | `0` | Application listening port |
| `git_repo` | No | `""` | Git repository URL to clone |
| `git_branch` | No | `"main"` | Git branch to checkout |

**Response (201):**
```json
{
  "id": 2,
  "domain": "newsite.com",
  "app_type": "python",
  "enabled": true
}
```

```bash
curl -X POST https://your-server:9090/sites \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "newsite.com",
    "app_type": "python",
    "git_repo": "https://github.com/user/newsite.git"
  }'
```

### Get Site Detail

```
GET /sites/{id}
```

**Auth required:** Yes

**Response (200):**
```json
{
  "Site": {
    "id": 1,
    "domain": "example.com",
    "app_type": "python"
  },
  "Deployments": [
    {
      "id": 5,
      "type": "deploy",
      "status": "success",
      "created_at": "2026-03-10T14:30:00Z"
    }
  ]
}
```

```bash
curl -s https://your-server:9090/sites/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

### Update Site

```
PUT /sites/{id}
```

**Auth required:** Yes

**Request body:** Same fields as create (all optional). Only provided fields are updated.

**Response (200):** Updated site object.

```bash
curl -X PUT https://your-server:9090/sites/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ssl_enabled": true}'
```

### Delete Site

```
DELETE /sites/{id}
```

**Auth required:** Yes

Disables the nginx config, stops and removes the systemd unit, and deletes the database record. Does **not** delete the application files.

**Response (200):**
```json
{
  "status": "deleted"
}
```

```bash
curl -X DELETE https://your-server:9090/sites/1 \
  -H "Authorization: Bearer $TOKEN"
```

### Deploy Site

```
POST /sites/{id}/deploy
```

**Auth required:** Yes

Performs a `git pull`, reinstalls dependencies (pip or npm based on app type), and restarts the application service.

**Response (200):**
```json
{
  "status": "deployed"
}
```

```bash
curl -X POST https://your-server:9090/sites/1/deploy \
  -H "Authorization: Bearer $TOKEN"
```

### Start Site

```
POST /sites/{id}/start
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "started"
}
```

### Stop Site

```
POST /sites/{id}/stop
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "stopped"
}
```

### Restart Site

```
POST /sites/{id}/restart
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "restarted"
}
```

### Site Logs

```
GET /sites/{id}/logs
```

**Auth required:** Yes

Returns the last 100 lines of the site's systemd journal.

**Response (200):**
```json
{
  "logs": "Mar 10 14:30:00 vps app-example.com[1234]: Starting server on :8000\n..."
}
```

### Site Log Stream (SSE)

```
GET /sites/{id}/logs/stream
```

**Auth required:** Yes

**Content-Type:** `text/event-stream`

Streams live log output via Server-Sent Events. Connect with an EventSource client.

```bash
curl -N https://your-server:9090/sites/1/logs/stream \
  -H "Authorization: Bearer $TOKEN"
```

---

## AUTARCH

### AUTARCH Status Page

```
GET /autarch
```

**Auth required:** Yes

Returns the AUTARCH management page (HTML) or status JSON.

### AUTARCH Status API

```
GET /autarch/status
```

**Auth required:** Yes

**Response (200):**
```json
{
  "installed": true,
  "install_dir": "/var/www/autarch",
  "git_commit": "abc1234 Latest commit message",
  "venv_ready": true,
  "pip_packages": 47,
  "web_running": true,
  "web_status": "active",
  "dns_running": false,
  "dns_status": "inactive"
}
```

```bash
curl -s https://your-server:9090/autarch/status \
  -H "Authorization: Bearer $TOKEN"
```

### Install AUTARCH

```
POST /autarch/install
```

**Auth required:** Yes

Clones the AUTARCH repository, creates a Python venv, installs pip and npm packages, sets permissions, and installs systemd units. Returns an error if AUTARCH is already installed.

**Response (200):**
```json
{
  "status": "installed"
}
```

**Response (409):**
```json
{
  "error": "AUTARCH already installed at /var/www/autarch"
}
```

```bash
curl -X POST https://your-server:9090/autarch/install \
  -H "Authorization: Bearer $TOKEN"
```

### Update AUTARCH

```
POST /autarch/update
```

**Auth required:** Yes

Performs `git pull`, reinstalls pip packages, and restarts both web and DNS services.

**Response (200):**
```json
{
  "status": "updated"
}
```

### Start AUTARCH

```
POST /autarch/start
```

**Auth required:** Yes

Starts both `autarch-web` and `autarch-dns` systemd services.

**Response (200):**
```json
{
  "status": "started"
}
```

### Stop AUTARCH

```
POST /autarch/stop
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "stopped"
}
```

### Restart AUTARCH

```
POST /autarch/restart
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "restarted"
}
```

### Get AUTARCH Config

```
GET /autarch/config
```

**Auth required:** Yes

Returns the contents of `autarch_settings.conf`.

**Response (200):**
```json
{
  "config": "[settings]\nport = 8181\n..."
}
```

```bash
curl -s https://your-server:9090/autarch/config \
  -H "Authorization: Bearer $TOKEN"
```

### Update AUTARCH Config

```
PUT /autarch/config
```

**Auth required:** Yes

**Request body:**
```json
{
  "config": "[settings]\nport = 8181\n..."
}
```

Writes the config string to `autarch_settings.conf` with `0600` permissions.

**Response (200):**
```json
{
  "status": "saved"
}
```

```bash
curl -X PUT https://your-server:9090/autarch/config \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"config": "[settings]\nport = 8181"}'
```

### Build AUTARCH DNS

```
POST /autarch/dns/build
```

**Auth required:** Yes

Runs `go build` in the AUTARCH DNS server directory.

**Response (200):**
```json
{
  "status": "built"
}
```

---

## SSL/TLS

### SSL Overview

```
GET /ssl
```

**Auth required:** Yes

Returns the SSL management page (HTML) or certificate list.

### SSL Status

```
GET /api/ssl/status
```

**Auth required:** Yes

**Response (200):**
```json
[
  {
    "domain": "example.com",
    "issuer": "Let's Encrypt",
    "not_before": "2025-12-15T00:00:00Z",
    "not_after": "2026-03-15T00:00:00Z",
    "days_left": 4,
    "auto_renew": true
  }
]
```

```bash
curl -s https://your-server:9090/api/ssl/status \
  -H "Authorization: Bearer $TOKEN"
```

### Issue Certificate

```
POST /ssl/{domain}/issue
```

**Auth required:** Yes

Issues a new Let's Encrypt SSL certificate for the domain using the ACME protocol and HTTP-01 challenge.

**Response (200):**
```json
{
  "status": "issued",
  "cert": "/etc/letsencrypt/live/example.com/fullchain.pem"
}
```

```bash
curl -X POST https://your-server:9090/ssl/example.com/issue \
  -H "Authorization: Bearer $TOKEN"
```

### Renew Certificate

```
POST /ssl/{domain}/renew
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "renewed"
}
```

```bash
curl -X POST https://your-server:9090/ssl/example.com/renew \
  -H "Authorization: Bearer $TOKEN"
```

---

## Nginx

### Nginx Status

```
GET /nginx
```

**Auth required:** Yes

**Response (200):**
```json
{
  "running": true,
  "status": "active",
  "config_test": "nginx: configuration file /etc/nginx/nginx.conf syntax is ok",
  "config_ok": true
}
```

```bash
curl -s https://your-server:9090/nginx \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

### Reload Nginx

```
POST /nginx/reload
```

**Auth required:** Yes

Tests the configuration first. If the test fails, the reload is aborted.

**Response (200):**
```json
{
  "status": "reloaded"
}
```

**Response (400):**
```json
{
  "error": "nginx config test failed -- not reloading"
}
```

```bash
curl -X POST https://your-server:9090/nginx/reload \
  -H "Authorization: Bearer $TOKEN"
```

### Restart Nginx

```
POST /nginx/restart
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "restarted"
}
```

### View Nginx Config

```
GET /nginx/config/{domain}
```

**Auth required:** Yes

Returns the nginx site configuration for a specific domain.

**Response (200):**
```json
{
  "domain": "example.com",
  "config": "server {\n    listen 80;\n    server_name example.com;\n    ...\n}"
}
```

```bash
curl -s https://your-server:9090/nginx/config/example.com \
  -H "Authorization: Bearer $TOKEN"
```

### Test Nginx Config

```
POST /nginx/test
```

**Auth required:** Yes

**Response (200):**
```json
{
  "output": "nginx: configuration file /etc/nginx/nginx.conf syntax is ok\nnginx: configuration file /etc/nginx/nginx.conf test is successful",
  "valid": true
}
```

```bash
curl -X POST https://your-server:9090/nginx/test \
  -H "Authorization: Bearer $TOKEN"
```

---

## Firewall

### List Firewall Rules

```
GET /firewall
```

**Auth required:** Yes

**Response (200):**
```json
{
  "enabled": true,
  "rules": [
    {
      "id": 1,
      "direction": "in",
      "protocol": "tcp",
      "port": "22",
      "source": "any",
      "action": "allow",
      "comment": "SSH"
    },
    {
      "id": 2,
      "direction": "in",
      "protocol": "tcp",
      "port": "80",
      "source": "any",
      "action": "allow",
      "comment": "HTTP"
    }
  ],
  "ufw_output": "Status: active\n..."
}
```

```bash
curl -s https://your-server:9090/firewall \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

### Firewall Status

```
GET /api/firewall/status
```

**Auth required:** Yes

Same response as `GET /firewall` but always returns JSON.

### Add Firewall Rule

```
POST /firewall/rules
```

**Auth required:** Yes

**Request body:**
```json
{
  "port": "8080",
  "protocol": "tcp",
  "source": "any",
  "action": "allow",
  "comment": "Custom app"
}
```

| Field | Required | Default | Description |
|---|---|---|---|
| `port` | Yes | - | Port number or range (e.g., `"8080"`, `"3000:3100"`) |
| `protocol` | No | `"tcp"` | Protocol: `tcp`, `udp`, or empty for both |
| `source` | No | `"any"` | Source IP or CIDR (e.g., `"192.168.1.0/24"`) |
| `action` | No | `"allow"` | Action: `allow` or `deny` |
| `comment` | No | `""` | Human-readable description |

**Response (201):**
```json
{
  "status": "rule added"
}
```

```bash
curl -X POST https://your-server:9090/firewall/rules \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"port": "8080", "protocol": "tcp", "action": "allow", "comment": "Custom app"}'
```

### Delete Firewall Rule

```
DELETE /firewall/rules/{id}
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "rule deleted"
}
```

```bash
curl -X DELETE https://your-server:9090/firewall/rules/3 \
  -H "Authorization: Bearer $TOKEN"
```

### Enable Firewall

```
POST /firewall/enable
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "enabled"
}
```

### Disable Firewall

```
POST /firewall/disable
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "disabled"
}
```

---

## Users

### System Users

#### List System Users

```
GET /users
```

**Auth required:** Yes

**Response (200):**
```json
[
  {
    "username": "root",
    "uid": "0",
    "home_dir": "/root",
    "shell": "/bin/bash"
  },
  {
    "username": "autarch",
    "uid": "1000",
    "home_dir": "/home/autarch",
    "shell": "/bin/bash"
  }
]
```

```bash
curl -s https://your-server:9090/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

#### Create System User

```
POST /users
```

**Auth required:** Yes

**Request body:**
```json
{
  "username": "deploy",
  "password": "secure-password",
  "shell": "/bin/bash"
}
```

| Field | Required | Default | Description |
|---|---|---|---|
| `username` | Yes | - | System username |
| `password` | Yes | - | User password |
| `shell` | No | `"/bin/bash"` | Login shell |

**Response (201):**
```json
{
  "status": "created",
  "username": "deploy"
}
```

```bash
curl -X POST https://your-server:9090/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username": "deploy", "password": "secure-password"}'
```

#### Delete System User

```
DELETE /users/{id}
```

**Auth required:** Yes

The `{id}` parameter is the username (not a numeric ID). The `root` and `autarch` accounts are protected and cannot be deleted.

**Response (200):**
```json
{
  "status": "deleted"
}
```

```bash
curl -X DELETE https://your-server:9090/users/deploy \
  -H "Authorization: Bearer $TOKEN"
```

### Panel Users

Panel users are Setec Manager web interface accounts (separate from system users).

#### List Panel Users

```
GET /panel/users
```

**Auth required:** Yes

**Response (200):** Array of panel user objects.

#### Create Panel User

```
POST /panel/users
```

**Auth required:** Yes

**Request body:**
```json
{
  "username": "operator",
  "password": "secure-password",
  "role": "admin"
}
```

| Field | Required | Default | Description |
|---|---|---|---|
| `username` | Yes | - | Panel username |
| `password` | Yes | - | Panel password |
| `role` | No | `"admin"` | User role |

**Response (201):**
```json
{
  "id": 2,
  "username": "operator"
}
```

#### Update Panel User

```
PUT /panel/users/{id}
```

**Auth required:** Yes

**Request body:**
```json
{
  "password": "new-password",
  "role": "admin"
}
```

Both fields are optional. Only provided fields are updated.

**Response (200):**
```json
{
  "status": "updated"
}
```

#### Delete Panel User

```
DELETE /panel/users/{id}
```

**Auth required:** Yes

**Response (200):**
```json
{
  "status": "deleted"
}
```

---

## Backups

### List Backups

```
GET /backups
```

**Auth required:** Yes

**Response (200):** Array of backup records with ID, type, file path, size, and creation timestamp.

```bash
curl -s https://your-server:9090/backups \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

### Create Site Backup

```
POST /backups/site/{id}
```

**Auth required:** Yes

Creates a `tar.gz` archive of the site's application directory.

**Response (201):**
```json
{
  "id": 3,
  "path": "/opt/setec-manager/data/backups/site-example.com-20260311-143000.tar.gz",
  "size": 15728640
}
```

```bash
curl -X POST https://your-server:9090/backups/site/1 \
  -H "Authorization: Bearer $TOKEN"
```

### Create Full System Backup

```
POST /backups/full
```

**Auth required:** Yes

Creates a `tar.gz` archive of the webroot (`/var/www`), nginx configuration (`/etc/nginx`), and Setec Manager data (`/opt/setec-manager/data`).

**Response (201):**
```json
{
  "id": 4,
  "path": "/opt/setec-manager/data/backups/full-system-20260311-143000.tar.gz",
  "size": 52428800
}
```

```bash
curl -X POST https://your-server:9090/backups/full \
  -H "Authorization: Bearer $TOKEN"
```

### Delete Backup

```
DELETE /backups/{id}
```

**Auth required:** Yes

Deletes both the database record and the backup file from disk.

**Response (200):**
```json
{
  "status": "deleted"
}
```

```bash
curl -X DELETE https://your-server:9090/backups/3 \
  -H "Authorization: Bearer $TOKEN"
```

### Download Backup

```
GET /backups/{id}/download
```

**Auth required:** Yes

Returns the backup file as a download with `Content-Disposition: attachment` header.

```bash
curl -O -J https://your-server:9090/backups/3/download \
  -H "Authorization: Bearer $TOKEN"
```

---

## Monitoring

### Monitor Page

```
GET /monitor
```

**Auth required:** Yes

Returns the monitoring dashboard HTML page.

### CPU Usage

```
GET /api/monitor/cpu
```

**Auth required:** Yes

**Response (200):**
```json
{
  "cpu": "%Cpu(s): 12.3 us, 2.1 sy, 85.6 id",
  "overall": 14.4,
  "idle": 85.6,
  "cores": [
    {"core": 0, "user": 15.2, "system": 3.1},
    {"core": 1, "user": 10.5, "system": 1.2}
  ]
}
```

```bash
curl -s https://your-server:9090/api/monitor/cpu \
  -H "Authorization: Bearer $TOKEN"
```

### Memory Usage

```
GET /api/monitor/memory
```

**Auth required:** Yes

**Response (200):**
```json
{
  "total": "7.8 GB",
  "used": "3.2 GB",
  "free": "1.1 GB",
  "available": "4.6 GB",
  "swap_total": "2.0 GB",
  "swap_used": "256 MB",
  "swap_free": "1.7 GB"
}
```

```bash
curl -s https://your-server:9090/api/monitor/memory \
  -H "Authorization: Bearer $TOKEN"
```

### Disk Usage

```
GET /api/monitor/disk
```

**Auth required:** Yes

**Response (200):**
```json
[
  {
    "filesystem": "/dev/vda1",
    "size": "78G",
    "used": "23G",
    "available": "52G",
    "use_percent": "30%",
    "mount_point": "/"
  }
]
```

```bash
curl -s https://your-server:9090/api/monitor/disk \
  -H "Authorization: Bearer $TOKEN"
```

### Service Status

```
GET /api/monitor/services
```

**Auth required:** Yes

Checks the status of key services: `nginx`, `autarch-web`, `autarch-dns`, `setec-manager`, `ufw`.

**Response (200):**
```json
[
  {"name": "nginx", "active": "active", "running": true, "memory": "12.5 MB"},
  {"name": "autarch-web", "active": "active", "running": true, "memory": "85.3 MB"},
  {"name": "autarch-dns", "active": "inactive", "running": false, "memory": ""},
  {"name": "setec-manager", "active": "active", "running": true, "memory": "18.2 MB"},
  {"name": "ufw", "active": "active", "running": true, "memory": ""}
]
```

```bash
curl -s https://your-server:9090/api/monitor/services \
  -H "Authorization: Bearer $TOKEN"
```

---

## Logs

### Logs Page

```
GET /logs
```

**Auth required:** Yes

Returns the logs viewer HTML page.

### System Logs

```
GET /api/logs/system
```

**Auth required:** Yes

**Query parameters:**

| Parameter | Default | Description |
|---|---|---|
| `lines` | `100` | Number of log lines to return |

**Response (200):**
```json
{
  "logs": "Mar 10 14:30:00 vps systemd[1]: Started Setec Manager.\n..."
}
```

```bash
curl -s "https://your-server:9090/api/logs/system?lines=50" \
  -H "Authorization: Bearer $TOKEN"
```

### Nginx Logs

```
GET /api/logs/nginx
```

**Auth required:** Yes

**Query parameters:**

| Parameter | Default | Description |
|---|---|---|
| `type` | `"access"` | Log type: `access` or `error` |

Returns the last 200 lines of the specified nginx log file.

**Response (200):**
```json
{
  "logs": "93.184.216.34 - - [10/Mar/2026:14:30:00 +0000] \"GET / HTTP/1.1\" 200 ...",
  "type": "access"
}
```

```bash
curl -s "https://your-server:9090/api/logs/nginx?type=error" \
  -H "Authorization: Bearer $TOKEN"
```

### Log Stream (SSE)

```
GET /api/logs/stream
```

**Auth required:** Yes

**Content-Type:** `text/event-stream`

**Query parameters:**

| Parameter | Default | Description |
|---|---|---|
| `unit` | `"autarch-web"` | Systemd unit to stream logs from |

Streams live log output via Server-Sent Events using `journalctl -f`.

```bash
curl -N "https://your-server:9090/api/logs/stream?unit=nginx" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Float Mode

Float Mode enables remote sessions for managing the server through a WebSocket bridge.

### Register Float Session

```
POST /float/register
```

**Auth required:** Yes

Returns HTTP 503 if Float Mode is disabled in the configuration.

**Request body (optional):**
```json
{
  "user_agent": "FloatClient/1.0"
}
```

**Response (201):**
```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "expires_in": "24h"
}
```

```bash
curl -X POST https://your-server:9090/float/register \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_agent": "FloatClient/1.0"}'
```

### List Float Sessions

```
GET /float/sessions
```

**Auth required:** Yes

Cleans expired sessions before returning the list.

**Response (200):** Array of active float session objects.

```bash
curl -s https://your-server:9090/float/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
```

### Disconnect Float Session

```
DELETE /float/sessions/{id}
```

**Auth required:** Yes

The `{id}` is the UUID session ID from the register response.

**Response (200):**
```json
{
  "status": "disconnected"
}
```

```bash
curl -X DELETE https://your-server:9090/float/sessions/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer $TOKEN"
```

### Float WebSocket

```
GET /float/ws
```

**Auth required:** Yes

Upgrades to a WebSocket connection for the Float Mode bridge. Use a WebSocket client library, not curl.

---

## Hosting Providers

The hosting provider API provides a unified interface for managing DNS records, domains, VPS instances, SSH keys, and billing across different hosting providers. See the [Hosting Providers Guide](hosting-providers.md) for detailed documentation.

### Provider Management

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosting/providers` | List all registered providers |
| `POST` | `/api/hosting/providers/{provider}/configure` | Set API credentials |
| `POST` | `/api/hosting/providers/{provider}/test` | Test connection |
| `DELETE` | `/api/hosting/providers/{provider}` | Remove saved credentials |

### DNS

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosting/providers/{provider}/dns/{domain}` | List DNS records |
| `POST` | `/api/hosting/providers/{provider}/dns/{domain}` | Create DNS record |
| `PUT` | `/api/hosting/providers/{provider}/dns/{domain}` | Update DNS records (batch) |
| `DELETE` | `/api/hosting/providers/{provider}/dns/{domain}` | Delete DNS record |
| `POST` | `/api/hosting/providers/{provider}/dns/{domain}/reset` | Reset DNS zone |

### Domains

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosting/providers/{provider}/domains` | List domains |
| `GET` | `/api/hosting/providers/{provider}/domains/{domain}` | Get domain details |
| `POST` | `/api/hosting/providers/{provider}/domains/check` | Check availability |
| `POST` | `/api/hosting/providers/{provider}/domains/purchase` | Purchase domain |
| `PUT` | `/api/hosting/providers/{provider}/domains/{domain}/nameservers` | Set nameservers |
| `POST` | `/api/hosting/providers/{provider}/domains/{domain}/lock` | Enable domain lock |
| `DELETE` | `/api/hosting/providers/{provider}/domains/{domain}/lock` | Disable domain lock |
| `POST` | `/api/hosting/providers/{provider}/domains/{domain}/privacy` | Enable WHOIS privacy |
| `DELETE` | `/api/hosting/providers/{provider}/domains/{domain}/privacy` | Disable WHOIS privacy |

### VPS

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosting/providers/{provider}/vms` | List VMs |
| `GET` | `/api/hosting/providers/{provider}/vms/{id}` | Get VM details |
| `POST` | `/api/hosting/providers/{provider}/vms` | Create VM |
| `GET` | `/api/hosting/providers/{provider}/datacenters` | List data centers |

### SSH Keys

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosting/providers/{provider}/ssh-keys` | List SSH keys |
| `POST` | `/api/hosting/providers/{provider}/ssh-keys` | Add SSH key |
| `DELETE` | `/api/hosting/providers/{provider}/ssh-keys/{id}` | Delete SSH key |

### Billing

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosting/providers/{provider}/subscriptions` | List subscriptions |
| `GET` | `/api/hosting/providers/{provider}/catalog` | Get product catalog |

For complete request/response examples for each hosting endpoint, see [hosting-providers.md](hosting-providers.md).

---

## Error Format

All API endpoints return errors in a consistent JSON format:

```json
{
  "error": "description of the error"
}
```

### Common HTTP Status Codes

| Code | Meaning |
|---|---|
| 200 | Success |
| 201 | Created |
| 303 | Redirect (HTML form submissions) |
| 400 | Bad request (invalid parameters) |
| 401 | Authentication required |
| 403 | Forbidden (insufficient permissions) |
| 404 | Resource not found |
| 409 | Conflict (duplicate resource) |
| 429 | Rate limited |
| 500 | Internal server error |
| 501 | Not implemented (provider does not support this operation) |
| 503 | Service unavailable (e.g., Float Mode disabled) |
