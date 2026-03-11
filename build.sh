#!/bin/bash
# Build Setec App Manager for Debian 13 (linux/amd64)
set -e

echo "Building Setec App Manager..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o setec-manager ./cmd/

echo "Binary: setec-manager ($(du -h setec-manager | cut -f1))"
echo ""
echo "Deploy to VPS:"
echo "  scp setec-manager root@<your-vps>:/opt/setec-manager/"
echo "  ssh root@<your-vps> '/opt/setec-manager/setec-manager --setup'"
