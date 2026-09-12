# Define the path to the config file
CONFIG_FILE=config/dev.yaml

# Use yq to extract values from YAML
DATABASE_HOST := $(shell yq '.db.connectionString' $(CONFIG_FILE))

# Migration command (using DATABASE_HOST)
MIGRATE := migrate -path=migration -database "$(DATABASE_HOST)" -verbose

# Default target
.PHONY: all
all: migrate

# Run migrations
.PHONY: migrate
migrate-up:
	@echo "Running migrations using connection string: $(DATABASE_HOST)..."
	@$(MIGRATE) up

# Down migrations
.PHONY: migrate
migrate-down:
	@echo "Down migrations using connection string: $(DATABASE_HOST)..."
	@$(MIGRATE) down

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/*

# Testing

# GO_PACKAGES is every Go package in the module, excluding the frontend.
#
# `go list ./...` picks up frontend/node_modules/flatted/golang/pkg/flatted as a
# package, so `go test ./...` and `go mod tidy` traverse node_modules on any
# machine where it exists.
GO_PACKAGES := $(shell go list ./... | grep -v '/frontend/')

# Run all unit tests (no external deps required).
#
# The previous list named packages that no longer exist
# (server/commits, server/bufmodules and five more were renamed to
# server/buf/* and server/commit), so `go list` errored and the target failed.
# Listing them by hand went stale the moment anything moved; the whole module
# does not.
.PHONY: test-unit
test-unit:
	go test $(GO_PACKAGES) -count=1

# Run OPA Rego policy tests (requires `opa` CLI on PATH).
.PHONY: test-opa
test-opa:
	opa test internal/hades/authorization/hades/authz/ -v

# Run integration tests (requires Docker / testcontainers).
.PHONY: test-integration
test-integration:
	go test -tags=integration $(GO_PACKAGES) -count=1

# Run the storage suite against PostgreSQL as well as SQLite.
#
# Every backend-specific defect this project has had existed because the two
# implementations were never tested against each other; the suite skips
# PostgreSQL silently when the DSN is unset, so this target is how it is not
# skipped.
.PHONY: test-parity
test-parity:
	@test -n "$(HADES_TEST_POSTGRES_DSN)" || \
		(echo "set HADES_TEST_POSTGRES_DSN, e.g. postgres://hades:hades@localhost:5432/hades_test?sslmode=disable" && false)
	go test ./internal/hades/storage/db/... -count=1

# Lint Go sources (requires golangci-lint on PATH).
.PHONY: lint
lint:
	# The config is verified first. issues.exclude-dirs is a v1 key and this
	# file declares version 2, so the frontend exclusion silently did nothing
	# and the vendored Go package under frontend/node_modules was linted. A
	# schema error has no other symptom, so it is checked rather than hoped for.
	golangci-lint config verify
	golangci-lint run ./...

# Lint and breaking-check the protobuf schemas (requires buf on PATH).
.PHONY: lint-proto
lint-proto:
	cd api && buf lint

# Run end-to-end tests (requires the full infrastructure running).
.PHONY: test-e2e
test-e2e:
	go test -tags=e2e $(GO_PACKAGES) -count=1

# Run all tests: unit + OPA + proto lint. Integration/E2E/parity are opt-in.
.PHONY: test
test: test-unit test-opa lint-proto

# Tools

# Install all required development binaries.
.PHONY: install-tools
install-tools:
	@echo "Installing yq..."
	go install github.com/mikefarah/yq/v4@latest
	@echo "Installing migrate..."
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "Installing buf..."
	go install github.com/bufbuild/buf/cmd/buf@latest
	@echo "Installing grpcurl..."
	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
	@echo "Installing opa..."
	go install github.com/open-policy-agent/opa@latest
	@echo "Installing regal..."
	go install github.com/styrainc/regal@latest
	# The path ends in /v2 because Go wants that for major version 2 and up.
	# Drop it and you get golangci-lint 1.x, which cannot read our config.
	@echo "Installing golangci-lint..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo "All tools installed."

# Generate an HTML coverage report for unit tests.
.PHONY: test-coverage
test-coverage:
	go test $(GO_PACKAGES) -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
