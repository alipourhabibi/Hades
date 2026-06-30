# Development Guide

Everything you need to run Hades locally. No Gitaly, no S3, no observability stack required.

## Prerequisites

- Go 1.22+
- Docker (for Postgres and Caddy)

Install dev tools (buf, grpcurl, migrate, yq, opa):

```bash
make install-tools
```

## Why TLS is required

The `buf` CLI only communicates over HTTPS with a trusted certificate. It does not support plaintext or self-signed certs. Hades runs in h2c mode (plaintext HTTP/2 on port 50051); Caddy sits in front and terminates TLS with an auto-generated local CA. **Caddy must be running for `buf push`, `buf dep update`, and `buf generate` to work.**

## One-time setup (per machine)

**1. Add the dev domain to hosts:**

```bash
sudo sh -c 'echo "127.0.0.1 example.com" >> /etc/hosts'
```

**2. Start Postgres + Caddy:**

```bash
docker compose -f development/docker-compose-minimal.yaml up -d
```

**3. Trust the auto-generated CA (requires Caddy to be running):**

```bash
./development/trust-dev-ca.sh
```

This exports Caddy's root certificate and installs it into the system trust store. Supports Arch/Fedora (`update-ca-trust`), Debian/Ubuntu (`update-ca-certificates`), and macOS (Keychain). After this, `buf`, `grpcurl`, and browsers trust `https://example.com` without warnings.

**4. Run migrations (Postgres only):**

```bash
make migrate-up
```

**5. Start the server:**

```bash
go run ./cmd/hades serve --config config/dev.yaml
```

## How TLS works in development

```
buf / grpcurl / browser
        │  HTTPS :443
        ▼
  ┌─────────────┐
  │    Caddy    │  (Docker, tls internal, auto-cert for example.com)
  └──────┬──────┘
         │  h2c (plaintext HTTP/2) :50051
         ▼
  ┌─────────────┐
  │ Hades server│  (go run on host)
  └─────────────┘
```

Caddy also proxies the Next.js frontend (`:3000`) for everything that is not an RPC path.

## Daily use

```bash
docker compose -f development/docker-compose-minimal.yaml up -d
go run ./cmd/hades serve --config config/dev.yaml
```

## Storage backends

The default `config/dev.yaml` uses Postgres for metadata. To run with zero external deps (SQLite), change:

```yaml
backends:
  database: sqlite

sqlite:
  path: ./hades.db
```

SQLite needs no migrations; the schema is applied automatically on first start. Caddy is still required for `buf` CLI access.

## Seed data (optional)

Create a sample user, module, and push protos:

```bash
cd development
./init.sh
```

This creates user `googleapis`, pushes the bundled googleapis protos, and writes credentials to `~/.netrc` so `buf` can authenticate.

Try it:

```bash
cd development/protos/simpleproject
buf dep update
buf push
```

## Database Migrations

```bash
make migrate-up    # apply
make migrate-down  # roll back one step
```

Requires `golang-migrate` CLI (`make install-tools` installs it).

## Testing

```bash
make test              # unit + OPA policy tests
make test-unit         # unit only
make test-opa          # OPA rego tests (requires opa CLI)
make test-integration  # requires Docker (testcontainers)
make test-e2e          # requires full stack running
```

## Configuration reference

Config is a YAML file passed via `--config`. Key fields:

| Field | Default | Options |
|---|---|---|
| `backends.database` | `sqlite` | `sqlite`, `postgres` |
| `backends.git` | `gogit` | `gogit`, `gitaly` |
| `backends.artifactStorage` | `disk` | `disk`, `s3`, `gitaly` |
| `backends.cache` | `memory` | `memory`, `redis` |
| `server.listenPort` | `50051` | any port |
| `server.registryHost` | (required) | domain used by buf CLI, must match Caddy domain |

See `config/dev.yaml` for the full annotated config.

## Frontend

```bash
cd frontend
cp .env.example .env.local
npm install
npm run dev   # http://localhost:3000
```

Requests to `/api/rpc/*` proxy to the backend at `BACKEND_URL` (default `http://localhost:50051`). The Caddy proxy also forwards non-RPC traffic to the frontend at `https://example.com`.
