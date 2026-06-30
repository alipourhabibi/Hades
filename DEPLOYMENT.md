# Deployment Guide

## Requirements

- A server with Docker and Docker Compose installed
- A domain name with an **A record pointing to your server's public IP**
- Ports **80** and **443** open (Caddy uses port 80 for the Let's Encrypt HTTP challenge)

## Deploy

**1. Clone the repo on your server:**

```bash
git clone https://github.com/alipourhabibi/Hades.git
cd Hades
```

**2. Edit `config/prod.yaml`:**

Two fields are required:

```yaml
server:
  registryHost: "yourdomain.com"   # your domain

totp:
  encryptionKey: "<64-char hex>"   # generate: openssl rand -hex 32
```

Everything else in `config/prod.yaml` is pre-configured for the Docker Compose stack (Postgres, Gitaly, MinIO, Redis all use the internal Docker network).

**3. Set your domain and start:**

```bash
DOMAIN=yourdomain.com docker compose up -d
```

That's it. Caddy automatically obtains a TLS certificate from Let's Encrypt on first start.

## What runs

| Service | Role |
|---|---|
| `caddy` | TLS termination, reverse proxy (Let's Encrypt auto-cert) |
| `hades` | Schema registry API (h2c on port 50051, internal) |
| `frontend` | Next.js web UI (port 3000, internal) |
| `postgres` | Metadata storage |
| `gitaly` | Git repository storage |
| `minio` | SDK artifact storage (S3-compatible) |
| `redis` | Cache / rate limiting |
| `migrator` | Runs DB migrations on startup, then exits |

## Optional: change credentials

Default credentials are set in `docker-compose.yml` via environment variables. Override them with a `.env` file:

```bash
POSTGRES_PASSWORD=strongpassword
MINIO_ROOT_USER=admin
MINIO_ROOT_PASSWORD=strongpassword
```

Update the matching values in `config/prod.yaml` if you change Postgres or MinIO credentials.

## Updates

```bash
docker compose pull
docker compose up -d
```

The `migrator` service runs automatically on startup and applies any new migrations.
