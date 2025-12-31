.PHONY: build test clean run-server run-worker lint docker

# Build variables
BINARY_DIR := bin
SERVER_BINARY := $(BINARY_DIR)/server
WORKER_BINARY := $(BINARY_DIR)/worker
GO := go

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)

# Build all binaries
build: $(SERVER_BINARY) $(WORKER_BINARY)

$(BINARY_DIR):
	mkdir -p $(BINARY_DIR)

$(SERVER_BINARY): $(BINARY_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(SERVER_BINARY) ./cmd/server

$(WORKER_BINARY): $(BINARY_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(WORKER_BINARY) ./cmd/worker

# Run tests
test:
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html

# Run server locally
run-server: $(SERVER_BINARY)
	$(SERVER_BINARY) -addr :8080

# Run worker locally
run-worker: $(WORKER_BINARY)
	$(WORKER_BINARY) -temporal-addr localhost:7233

# Lint code
lint:
	golangci-lint run ./...

# Format code
fmt:
	$(GO) fmt ./...

# Check for vulnerabilities
vuln:
	govulncheck ./...

# Docker build
docker:
	docker build -t code-reviewer:latest .

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build server and worker binaries"
	@echo "  test          - Run all tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean         - Remove build artifacts"
	@echo "  run-server    - Run the HTTP server"
	@echo "  run-worker    - Run the Temporal worker"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  vuln          - Check for vulnerabilities"
	@echo "  docker        - Build Docker image"
