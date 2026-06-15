# Hades Schema Registry

Hades is an open source [Buf](https://github.com/bufbuild/buf) compatible schema registry.

## Getting started
```bash
cp config/config.sample.yaml config/dev.yaml
docker compose -f development/docker-compose-dev.yaml up -d
go run ./cmd/hades serve --config config/dev.yaml
```

## Development env

### One-time setup (per machine)

**1. Add `example.com` to hosts**
```bash
sudo sh -c 'echo "127.0.0.1 example.com" >> /etc/hosts'
```

**2. Start infrastructure** (PostgreSQL, PgAdmin, Gitaly, MinIO, Caddy, observability stack)
```bash
docker compose -f development/docker-compose-dev.yaml up -d
```
This starts: PostgreSQL, PgAdmin, Gitaly, MinIO, Caddy (TLS proxy), and the observability stack (OTel Collector, Jaeger, Prometheus, Grafana).

**3. Trust the auto-generated dev CA**
```bash
./development/trust-dev-ca.sh
```
This exports Caddy's root certificate and installs it into the system trust store. Supports Arch/Fedora (`update-ca-trust`), Debian/Ubuntu (`update-ca-certificates`), and macOS (Keychain). After this step, `buf`, `grpcurl`, and browsers will trust `https://example.com` without warnings.

**4. Copy the sample config and fill in your settings**
```bash
cp config/config.sample.yaml config/dev.yaml
```

**5. Run migrations**
```bash
make migrate-up
```

### Daily use (no sudo)
```bash
docker compose -f development/docker-compose-dev.yaml up -d
go run ./cmd/hades serve --config config/dev.yaml
```

### How TLS works in development

```
buf / grpcurl / browser
        │  HTTPS :443
        ▼
  ┌─────────────┐
  │    Caddy    │  (Docker container)
  │  tls internal  ← auto-generated cert for example.com
  └──────┬──────┘
         │  h2c (plaintext HTTP/2) :50051
         ▼
  ┌─────────────┐
  │ Hades server│  (go run on host)
  │  no TLS     │
  └─────────────┘
```

- **Caddy** listens on port 443 inside Docker (no `ip_unprivileged_port_start` sysctl needed on the host).
- **Caddy's local CA** issues a certificate for `example.com`. After running `trust-dev-ca.sh` once, all tools trust it.
- **The Go server** runs in h2c mode (plain HTTP/2, no TLS) on port 50051. Set `certFile`/`certKey` in the config to switch to direct TLS (production).

### Observability URLs (local)

| Service | URL |
|---|---|
| Jaeger (traces) | http://localhost:16686 |
| Prometheus | http://localhost:9091 |
| Grafana | http://localhost:3000 (admin/admin) |
| PgAdmin | http://localhost:8080 (admin@example.com/admin) |
| MinIO | http://localhost:9001 (minioadmin/minioadmin) |

### Initialize development environment

`cd` into the development folder, then run:
```bash
./install_tools.sh
./init.sh
```

- Creates a user: `googleapis`
- Creates a module: `googleapis` with sample protos
- Sets up a project in `protos/simpleproject` that depends on the googleapis module

---

I coded this fast, focusing on getting things working first rather than following the best practices from day one. So there's room for improvement, refactoring, and optimizations.

If you spot something that could be improved - better architecture, cleaner code, or best practices - feel free to open an issue or a PR. Your contributions are highly appreciated!

Enjoy hacking on Hades!

### Features ready to test

#### Buf CLI integration
```bash
buf dep update   # resolve dependencies from Hades registry
buf push         # push .proto modules to Hades
buf generate     # generate code from registry modules
```
Navigate to `development/protos/simpleproject` to try these.  
You can modify protos in `development/protos/googleapis` and push with `buf push`.

#### Authentication & sessions
- Register, login, logout with session tokens
- Email verification + resend (need more tests)
- Password reset and change
- Session listing and revocation (`ListSessions`, `RevokeSession`, `RevokeAllOtherSessions`)

#### Modules & commits
- Create and list modules (`CreateModuleByName`, `ListModules`, `GetModule`)
- Immutable commit history (`ListCommits`, `GetCommit`)
- File tree browsing at HEAD (`ListModuleFiles`, `GetFileContent`)
- Commit diffs (`GetCommitDiff`)

#### Organizations
- Create, update, list orgs (`CreateOrg`, `GetOrg`, `UpdateOrg`, `ListOrganizations`)
- Member management (`AddOrgMember`, `RemoveOrgMember`, `ListOrgMembers`, `GetUserOrgs`)

#### CI checks (runs on every push) (need integration with the project's yaml file)
- `buf lint` - proto style and naming rules
- `buf breaking` - backward compatibility against previous commit
- Results queryable via `CIService/GetCIRun`

#### SDK generation (async, runs after push)
- See [SDK Generation](#sdk-generation) section below

#### Audit log
- Append-only security event log with cursor pagination (`ListAuditLog`)

#### Notifications
- In-app alerts for commits, SDK jobs, CI failures (`ListNotifications`, `MarkNotificationRead`)

#### Buf-protocol adapters (used by the `buf` CLI transparently)
- `buf.registry.module.v1` - `GetModules`, `ListModules`, `GetCommits`, `ListCommits`
- `buf.registry.module.v1` - `Upload`, `Download`, `GetGraph`

#### Go module proxy
- `/go/{module}/@v/...` - standard Go proxy protocol so SDK-generated Go packages are `go get`-able directly from Hades

## Frontend

Next.js web UI lives in [`frontend/`](./frontend/README.md).

### Dev

```bash
cd frontend
cp .env.example .env.local   # set BACKEND_URL, NEXT_PUBLIC_DOMAIN
npm install
npm run dev                  # http://localhost:3000
```

Requests to `/api/rpc/*` are proxied to `BACKEND_URL` (default `http://localhost:50051`), so the backend must be running.

See [`frontend/README.md`](./frontend/README.md) for full env var reference and route map.

---

## SDK Generation

When a module is pushed, Hades asynchronously generates SDK code (Go, Python, gRPC stubs, etc.) using `protoc` and stores the artifacts in S3-compatible object storage (MinIO in dev).

### How it works

1. `buf push` → upload lands in Hades
2. One `sdk_jobs` row per configured generator is inserted in the same DB transaction as the commit
3. A background worker polls `sdk_jobs`, fetches `.proto` files from Gitaly, runs `protoc`, and uploads the output to S3
4. Job status is queryable via the `SDKService/ListSDKs` RPC

### Config

Add an `sdk` block to `config/dev.yaml`:

```yaml
sdk:
  enabled: true
  bufBin: "buf"
  protocBin: "protoc"
  lintEnabled: true
  breakingEnabled: true
  generators:
    - language: go
      plugin: go
      options: "paths=source_relative"
    - language: go-grpc
      plugin: go-grpc
      options: "paths=source_relative"
    - language: python
      plugin: python
      options: ""
  storage:
    type: s3
    s3:
      endpoint: "http://localhost:9000"
      bucket: "hades-sdks"
      accessKeyId: "minioadmin"
      secretAccessKey: "minioadmin"
      useSSL: false
      region: "us-east-1"
```

### Required binaries

Install the protoc plugins needed for the generators you enable:

```bash
# Go
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Python
pip install grpcio-tools
```

All binaries must be on `$PATH`. `buf` and `protoc` paths are configurable via `sdk.bufBin` / `sdk.protocBin`.

### Query job status

```bash
grpcurl -plaintext \
  -d '{"owner": "alice", "module": "mymodule"}' \
  localhost:50051 \
  hades.api.registry.v1.SDKService/ListSDKs
```

#### Licensing
This project includes files from Google APIs for development purposes, which are licensed under the Apache License 2.0. See the `LICENSE` file in `development/protos/googleapis` for details.
