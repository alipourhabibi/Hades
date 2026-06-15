#!/usr/bin/env bash
# trust-ca.sh - one-time script to trust the Caddy local CA on this machine.
# Run once after `docker compose up -d` so curl, buf, and browsers trust the cert.

set -euo pipefail

COMPOSE_FILE="$(dirname "$0")/docker-compose.yml"
CA_CERT="/tmp/hades-ca.crt"

echo "Waiting for Caddy to generate its local CA..."
for i in $(seq 1 30); do
  if docker compose -f "$COMPOSE_FILE" exec -T caddy test -f /data/caddy/pki/authorities/local/root.crt 2>/dev/null; then
    break
  fi
  sleep 1
done

docker compose -f "$COMPOSE_FILE" exec -T caddy \
  cat /data/caddy/pki/authorities/local/root.crt > "$CA_CERT"

echo "Trusting Hades CA..."

if [[ "$OSTYPE" == "darwin"* ]]; then
  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$CA_CERT"
elif command -v update-ca-trust &>/dev/null; then
  sudo cp "$CA_CERT" /etc/ca-certificates/trust-source/anchors/hades-ca.crt
  sudo update-ca-trust
  sudo trust extract-compat
elif command -v update-ca-certificates &>/dev/null; then
  sudo cp "$CA_CERT" /usr/local/share/ca-certificates/hades-ca.crt
  sudo update-ca-certificates
else
  echo "Unsupported OS - manually trust: $CA_CERT"
  exit 1
fi

echo "Done. Restart browser and buf CLI if needed."
