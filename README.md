# Hades

An open-source [Buf](https://github.com/bufbuild/buf)-compatible schema registry for managing and versioning Protocol Buffer definitions.

## Features

- **Buf CLI integration**: `buf push`, `buf dep update`, `buf generate` work out of the box
- **Immutable commit history**: every push creates a commit; diffs and file tree browsing at any ref
- **Modules and organizations**: fine-grained role-based access control (OPA-backed)
- **CI checks**: `buf lint` and `buf breaking` run automatically on push
- **SDK generation**: async codegen (Go, Python, gRPC stubs) stored in S3-compatible storage
- **Go module proxy**: generated Go SDKs are `go get`-able directly from Hades
- **Audit log**: append-only security event log with cursor pagination
- **Auth**: sessions, email verification, password reset, TOTP, OAuth (GitHub, Google), API tokens

## Quickstart

**Run with no external dependencies (SQLite + local filesystem):**

```bash
go run ./cmd/hades serve --config config/dev.yaml
```

The default `config/dev.yaml` uses SQLite for metadata and local disk for git and artifacts. No Docker required.

**Run with Postgres:**

```bash
docker compose -f development/docker-compose-minimal.yaml up -d
make migrate-up
go run ./cmd/hades serve --config config/dev.yaml
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for the full development guide.

## Architecture

```
cmd/hades/           CLI entry point (Cobra)
internal/hades/
  server/            Connect-RPC handlers (one package per domain)
  storage/
    db/              Metadata: PostgreSQL or SQLite
      postgres/      Postgres implementations
      sqlite/        SQLite implementations
    git/             Git storage
      gogit/         go-git backend (local bare repos, default)
      gitaly/        Gitaly gRPC backend
  authorization/     OPA policy engine (RBAC)
config/              YAML config structs
migration/           SQL migration files
frontend/            Next.js web UI
```

Storage backends are configurable and swappable:

| Layer | Default | Alternatives |
|---|---|---|
| Metadata | SQLite | PostgreSQL |
| Git | go-git (local) | Gitaly |
| Artifacts | Local disk | S3 / MinIO |
| Cache | In-memory | Redis |

## Using the Buf CLI

```bash
# Authenticate (writes to ~/.netrc)
buf registry login your.domain.com

# Push a module
buf push

# Resolve dependencies from your registry
buf dep update

# Generate code
buf generate
```

See `development/protos/simpleproject` for a working example.

## Documentation

- [DEVELOPMENT.md](DEVELOPMENT.md): local setup, SQLite vs Postgres, testing, config reference
- [DEPLOYMENT.md](DEPLOYMENT.md): production deployment, TLS, Gitaly, S3, observability

## Contributing

Hades is in active development. Architecture is intentional and improving. If you spot a better approach, open an issue or PR. Contributions are welcome.

## License

Apache 2.0. See [LICENSE](LICENSE).
