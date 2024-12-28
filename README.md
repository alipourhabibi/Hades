# Hades Schema Registry

Hades is an open source [Buf](https://github.com/bufbuild/buf) compatible schema registry.

## Getting started
bring up the required tools.
```sh
docker compose -f development/docker-compose-dev.yaml up -d
go run ./cmd/hades/. serve --config config/dev.yaml
```
