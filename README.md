# Setec App Manager

VPS management panel for site deployment, DNS, SSL, Docker, Git hosting, firewall, monitoring, and more.

Built by darkHal Security Group & Setec Security Labs.

## Quick Start

### Prerequisites

- Debian 13 (or Ubuntu 22.04+) VPS with root access
- x86_64 architecture

### Option 1: Build from source on the VPS

```bash
# Install Go 1.22+ if not already installed
wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone and build
git clone https://github.com/DigijEth/Setec_manager.git
cd Setec_manager
go build -ldflags="-s -w" -o setec-manager ./cmd/

# Run setup
sudo mkdir -p /opt/setec-manager/data
sudo cp setec-manager /opt/setec-manager/
sudo cp config.yaml /opt/setec-manager/
sudo /opt/setec-manager/setec-manager --setup
```

### Option 2: Cross-compile on your machine, deploy to VPS

```bash
# Build (on your machine)
./build.sh

# Copy to VPS
scp setec-manager config.yaml root@YOUR_VPS_IP:/opt/setec-manager/

# SSH in and run setup
ssh root@YOUR_VPS_IP
chmod +x /opt/setec-manager/setec-manager
/opt/setec-manager/setec-manager --setup
```

### Option 3: Clone on VPS and build there

```bash
ssh root@YOUR_VPS_IP

# Install Go
wget -q https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone, build, install
git clone https://github.com/DigijEth/Setec_manager.git /opt/setec-manager/src
cd /opt/setec-manager/src
go build -ldflags="-s -w" -o /opt/setec-manager/setec-manager ./cmd/
cp config.yaml /opt/setec-manager/

# Run first-time setup
/opt/setec-manager/setec-manager --setup
```

## What --setup does

The setup command:
1. Creates required directories (`/opt/setec-manager/data/`, backups, ACME)
2. Installs nginx, certbot, and ufw via apt
3. Configures nginx snippets for reverse proxy and SSL
4. Creates the default admin user
5. Generates a self-signed TLS certificate for the manager dashboard
6. Installs a systemd service unit (`setec-manager.service`)

## Starting the service

After setup:

```bash
# Start via systemd
systemctl start setec-manager

# Or run directly
/opt/setec-manager/setec-manager --config /opt/setec-manager/config.yaml
```

## Accessing the dashboard

```
https://YOUR_VPS_IP:9090
```

Default credentials:
- **Username:** admin
- **Password:** autarch
- **Change this immediately after first login.**

Your browser will warn about the self-signed certificate — this is expected.
Accept the warning to proceed.

## Configuration

Edit `/opt/setec-manager/config.yaml` to change:

- **Server port** (default: 9090)
- **TLS cert/key paths**
- **Nginx paths**
- **ACME/Let's Encrypt email**
- **Backup settings**
- **Logging settings**

## Troubleshooting

### "cannot execute binary file"

This means the binary doesn't have execute permission:

```bash
chmod +x /opt/setec-manager/setec-manager
```

If you still get the error, check your VPS architecture:

```bash
uname -m
```

If it says `aarch64` (ARM), you need to rebuild for ARM:

```bash
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o setec-manager ./cmd/
```

### Port 9090 not reachable

Open the port in the firewall:

```bash
ufw allow 9090/tcp
```

### "Failed to load config"

Make sure config.yaml is at the path specified by --config:

```bash
ls -la /opt/setec-manager/config.yaml
```

### "Failed to open database"

Ensure the data directory exists and is writable:

```bash
mkdir -p /opt/setec-manager/data
chmod 755 /opt/setec-manager/data
```

## Features

- **Site Management** — Deploy and manage web applications with Nginx reverse proxy
- **Docker Management** — Install Docker, manage containers/images/volumes, one-click deploy with domain routing
- **Git Hosting** — Integrated Gitea setup wizard, repo/user/org management
- **Hosting Provider API** — Hostinger integration + pluggable interface for other providers
- **SSL/TLS** — Let's Encrypt certificates via certbot
- **Firewall** — UFW rule management
- **User Management** — RBAC with groups (Admin, Support, Power User, Subscriber)
- **Backups** — Site and full system backups
- **Monitoring** — CPU, memory, disk, service status
- **Log Streaming** — Live log viewer via SSE
- **Cron Scheduler** — SSL renewal, backups, git pull, service restart
- **Float Mode** — WebSocket USB bridge (coming soon)

## License

See [LICENSE](LICENSE) for terms. Free for defensive use. See the license for all other terms.
