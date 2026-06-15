#!/usr/bin/env bash
# trust-dev-ca.sh - one-time script to trust the Caddy local CA on your machine.
# Run this after `docker compose up` so Caddy has had time to generate its CA.

set -euo pipefail

COMPOSE_FILE="$(dirname "$0")/docker-compose-dev.yaml"
CA_CERT="/tmp/hades-dev-ca.crt"

echo "Waiting for Caddy to generate its local CA..."
for i in $(seq 1 30); do
  if docker compose -f "$COMPOSE_FILE" exec -T caddy test -f /data/caddy/pki/authorities/local/root.crt 2>/dev/null; then
    break
  fi
  sleep 1
done

docker compose -f "$COMPOSE_FILE" exec -T caddy \
  cat /data/caddy/pki/authorities/local/root.crt > "$CA_CERT"

echo "Trusting Hades dev CA..."

# Not tested
if [[ "$OSTYPE" == "darwin"* ]]; then
  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$CA_CERT"

# Tested
elif command -v update-ca-trust &>/dev/null; then
  # Arch
  sudo cp "$CA_CERT" /etc/ca-certificates/trust-source/anchors/hades-dev-ca.crt;
  sudo update-ca-trust;
  sudo trust extract-compat;

# Not tested
elif command -v update-ca-certificates &>/dev/null; then
  # Debian / Ubuntu
  sudo cp "$CA_CERT" /usr/local/share/ca-certificates/hades-dev-ca.crt
  sudo update-ca-certificates

else
  echo "Unsupported OS - manually trust: $CA_CERT"
  exit 1
fi

echo "Done. Restart your browser and buf CLI if needed."
