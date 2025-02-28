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

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/*
