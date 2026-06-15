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

# Run all unit tests (no external deps required).
.PHONY: test-unit
test-unit:
	go test ./utils/... ./internal/hades/authorization/... ./internal/hades/server/commits/... ./internal/hades/server/graph/... ./internal/hades/server/module/... ./internal/hades/server/bufcommits/... ./internal/hades/server/bufmodules/... ./internal/hades/server/bufgraph/... ./internal/hades/server/bufdownload/... ./internal/hades/server/bufupload/... -count=1

# Run OPA Rego policy tests (requires `opa` CLI on PATH).
.PHONY: test-opa
test-opa:
	opa test internal/hades/authorization/hades/authz/ -v

# Run integration tests (requires Docker / testcontainers).
.PHONY: test-integration
test-integration:
	go test -tags=integration ./... -count=1

# Run end-to-end tests (requires the full infrastructure running).
.PHONY: test-e2e
test-e2e:
	go test -tags=e2e ./... -count=1

# Run all tests: unit + OPA. Integration/E2E are opt-in.
.PHONY: test
test: test-unit test-opa

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
	@echo "All tools installed."

# Generate an HTML coverage report for unit tests.
.PHONY: test-coverage
test-coverage:
	go test ./utils/... ./internal/hades/authorization/... ./internal/hades/server/commits/... ./internal/hades/server/graph/... ./internal/hades/server/module/... ./internal/hades/server/bufcommits/... ./internal/hades/server/bufmodules/... ./internal/hades/server/bufgraph/... ./internal/hades/server/bufdownload/... ./internal/hades/server/bufupload/... -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
