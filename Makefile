# Code Reviewer Makefile
# =====================

# Variables
APP_NAME := code-reviewer
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
DOCKER_REGISTRY := yourorg
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(APP_NAME)

# Go variables
GO := go
GOFLAGS := -v
LDFLAGS := -ldflags "-w -s -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Directories
BIN_DIR := bin
CMD_DIR := cmd
COVERAGE_DIR := coverage

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

.PHONY: all build clean test lint fmt vet run help
.PHONY: docker-build docker-push docker-run
.PHONY: compose-up compose-down compose-logs compose-ps
.PHONY: deps deps-update deps-tidy
.PHONY: install-tools coverage
.PHONY: helm-lint helm-template helm-install helm-upgrade helm-uninstall

# Default target
all: lint test build

## =============================================================================
## Build Commands
## =============================================================================

build: ## Build all binaries
	@echo "$(GREEN)Building binaries...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/ ./$(CMD_DIR)/...

build-linux: ## Build for Linux (amd64)
	@echo "$(GREEN)Building for Linux...$(NC)"
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/ ./$(CMD_DIR)/...

build-darwin: ## Build for macOS (arm64)
	@echo "$(GREEN)Building for macOS...$(NC)"
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/ ./$(CMD_DIR)/...

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning...$(NC)"
	@rm -rf $(BIN_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f coverage.out coverage.html

## =============================================================================
## Development Commands
## =============================================================================

run: build ## Run the worker locally
	@echo "$(GREEN)Running worker...$(NC)"
	./$(BIN_DIR)/postgres-worker

run-dev: ## Run with development settings
	@echo "$(GREEN)Running in development mode...$(NC)"
	ENVIRONMENT=development LOG_LEVEL=debug LOG_FORMAT=text $(GO) run ./$(CMD_DIR)/postgres-worker

fmt: ## Format Go code
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GO) fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GO) vet ./...

lint: ## Run linters
	@echo "$(GREEN)Running linters...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not installed, running go vet only$(NC)"; \
		$(GO) vet ./...; \
	fi

## =============================================================================
## Test Commands
## =============================================================================

test: ## Run tests
	@echo "$(GREEN)Running tests...$(NC)"
	$(GO) test -race ./...

test-short: ## Run short tests only
	@echo "$(GREEN)Running short tests...$(NC)"
	$(GO) test -short ./...

test-verbose: ## Run tests with verbose output
	@echo "$(GREEN)Running tests (verbose)...$(NC)"
	$(GO) test -race -v ./...

coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(GREEN)Coverage report: $(COVERAGE_DIR)/coverage.html$(NC)"

coverage-report: coverage ## Show coverage in browser
	@if command -v open >/dev/null 2>&1; then \
		open $(COVERAGE_DIR)/coverage.html; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open $(COVERAGE_DIR)/coverage.html; \
	fi

## =============================================================================
## Dependency Commands
## =============================================================================

deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GO) mod download

deps-update: ## Update dependencies
	@echo "$(GREEN)Updating dependencies...$(NC)"
	$(GO) get -u ./...
	$(GO) mod tidy

deps-tidy: ## Tidy go.mod
	@echo "$(GREEN)Tidying go.mod...$(NC)"
	$(GO) mod tidy

deps-verify: ## Verify dependencies
	@echo "$(GREEN)Verifying dependencies...$(NC)"
	$(GO) mod verify

## =============================================================================
## Docker Commands
## =============================================================================

docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(VERSION) \
		-t $(DOCKER_IMAGE):latest \
		.

docker-push: ## Push Docker image to registry
	@echo "$(GREEN)Pushing Docker image...$(NC)"
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

docker-run: ## Run Docker container locally
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run --rm -it \
		-p 8080:8080 \
		-p 9090:9090 \
		-e ENVIRONMENT=development \
		-e LOG_LEVEL=debug \
		$(DOCKER_IMAGE):latest

docker-shell: ## Shell into Docker container
	@echo "$(GREEN)Opening shell in container...$(NC)"
	docker run --rm -it --entrypoint /bin/sh $(DOCKER_IMAGE):latest

## =============================================================================
## Docker Compose Commands
## =============================================================================

compose-up: ## Start all services
	@echo "$(GREEN)Starting services...$(NC)"
	docker-compose up -d

compose-up-build: ## Build and start all services
	@echo "$(GREEN)Building and starting services...$(NC)"
	docker-compose up -d --build

compose-up-full: ## Start with observability stack
	@echo "$(GREEN)Starting with observability...$(NC)"
	docker-compose --profile observability up -d

compose-up-milvus: ## Start with Milvus vector database
	@echo "$(GREEN)Starting with Milvus...$(NC)"
	docker-compose --profile milvus up -d

compose-down: ## Stop all services
	@echo "$(YELLOW)Stopping services...$(NC)"
	docker-compose down

compose-down-volumes: ## Stop services and remove volumes
	@echo "$(RED)Stopping services and removing volumes...$(NC)"
	docker-compose down -v

compose-logs: ## Show logs
	docker-compose logs -f

compose-logs-app: ## Show application logs only
	docker-compose logs -f code-reviewer

compose-ps: ## Show running services
	docker-compose ps

compose-restart: ## Restart services
	@echo "$(YELLOW)Restarting services...$(NC)"
	docker-compose restart

compose-pull: ## Pull latest images
	@echo "$(GREEN)Pulling latest images...$(NC)"
	docker-compose pull

## =============================================================================
## Ollama Commands
## =============================================================================

ollama-pull: ## Pull required Ollama models
	@echo "$(GREEN)Pulling Ollama models...$(NC)"
	docker-compose exec ollama ollama pull qwen2.5-coder:32b
	docker-compose exec ollama ollama pull nomic-embed-text

ollama-list: ## List Ollama models
	docker-compose exec ollama ollama list

## =============================================================================
## Helm Commands
## =============================================================================

HELM_RELEASE := code-reviewer
HELM_NAMESPACE := code-reviewer
HELM_CHART := ./charts/code-reviewer

helm-lint: ## Lint Helm chart
	@echo "$(GREEN)Linting Helm chart...$(NC)"
	helm lint $(HELM_CHART)

helm-template: ## Render Helm templates
	@echo "$(GREEN)Rendering Helm templates...$(NC)"
	helm template $(HELM_RELEASE) $(HELM_CHART)

helm-template-debug: ## Render Helm templates with debug
	helm template $(HELM_RELEASE) $(HELM_CHART) --debug

helm-install: ## Install Helm chart
	@echo "$(GREEN)Installing Helm chart...$(NC)"
	helm install $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace

helm-install-dry-run: ## Dry run Helm install
	helm install $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) \
		--dry-run

helm-upgrade: ## Upgrade Helm release
	@echo "$(GREEN)Upgrading Helm release...$(NC)"
	helm upgrade $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE)

helm-uninstall: ## Uninstall Helm release
	@echo "$(YELLOW)Uninstalling Helm release...$(NC)"
	helm uninstall $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

helm-status: ## Show Helm release status
	helm status $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

helm-package: ## Package Helm chart
	@echo "$(GREEN)Packaging Helm chart...$(NC)"
	helm package $(HELM_CHART)

## =============================================================================
## Tool Installation
## =============================================================================

install-tools: ## Install development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest

## =============================================================================
## CI/CD Commands
## =============================================================================

ci: deps lint test build ## Run CI pipeline locally
	@echo "$(GREEN)CI pipeline completed successfully!$(NC)"

ci-full: deps lint test coverage build docker-build helm-lint ## Full CI with Docker and Helm
	@echo "$(GREEN)Full CI pipeline completed successfully!$(NC)"

## =============================================================================
## Utility Commands
## =============================================================================

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

env: ## Show environment configuration
	@echo "APP_NAME: $(APP_NAME)"
	@echo "DOCKER_IMAGE: $(DOCKER_IMAGE)"
	@echo "VERSION: $(VERSION)"

## =============================================================================
## Help
## =============================================================================

help: ## Show this help
	@echo "$(APP_NAME) - Makefile Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'
