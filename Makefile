DOCKER_ORG ?= ddtcorex/govard-
BINARY_NAME=govard
BUILD_DIR=bin
TEST_BINARY=$(BUILD_DIR)/govard-test
UNIT_PACKAGES=$(shell go list ./... | grep -v '^govard/tests/integration$$')
COVER_PACKAGES=$(shell go list ./internal/... | tr '\n' ',' | sed 's/,$$//')
VERSION_RAW ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo 1.0.0)
VERSION ?= $(patsubst v%,%,$(VERSION_RAW))
LDFLAGS ?= -s -w -X govard/internal/cmd.Version=$(VERSION) -X govard/internal/desktop.Version=$(VERSION)

.PHONY: help install build-test-binary build clean test test-fast test-unit test-coverage test-integration test-integration-ci test-frontend lint fmt fmt-check vet push test-realenv-setup test-realenv test-realenv-clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

install: ## Build and install Govard binary to system (via install.sh)
	./install.sh --source -y

build-frontend:
	@echo "Building frontend assets..."
	@cd desktop/frontend && npm install && npm run build:css

build: build-frontend ## Build Govard binary for the current platform
	@echo "Building Govard..."
	mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) cmd/govard/main.go

build-test-binary: build-frontend
	@echo "Building test binary..."
	mkdir -p $(BUILD_DIR)
	go build -mod=mod -ldflags "$(LDFLAGS)" -tags integration -o $(TEST_BINARY) cmd/govard/main.go

test: lint fmt-check vet test-frontend test-unit test-integration ## Run all tests

test-fast: lint fmt-check vet test-frontend test-unit

fmt-check:
	@echo "Checking code formatting..."
	@unformatted="$$(find . -type f -name '*.go' -not -path './vendor/*' -print0 | xargs -0 gofmt -s -l)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

test-unit:
	@echo "Running unit tests..."
	go test $(UNIT_PACKAGES) -v -short

test-coverage:
	@echo "Running unit tests with coverage..."
	go test ./tests -coverprofile=coverage.out -covermode=atomic -coverpkg=$(COVER_PACKAGES)
	go tool cover -func=coverage.out
	@echo "Coverage profile written to coverage.out"

test-integration: build-test-binary
	@echo "Running integration tests..."
	go test -tags integration ./tests/integration/... -v -timeout 30m

test-integration-ci: build-test-binary
	@echo "Running integration tests (CI mode)..."
	go test -tags integration ./tests/integration/... -v -timeout 30m -parallel 4

test-frontend:
	@echo "Running frontend unit tests..."
	node --test tests/frontend/*.test.mjs

lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		$$(go env GOPATH)/bin/golangci-lint run ./...; \
	fi

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

# Real environment tests (3 Magento 2 instances)
REALENV_DIR := tests/integration/realenv

.PHONY: test-realenv-setup test-realenv test-realenv-clean test-realenv-full

test-realenv-setup: build-test-binary
	@echo "Setting up three-environment test infrastructure..."
	@cd $(REALENV_DIR) && chmod +x setup-three-env.sh && ./setup-three-env.sh

test-realenv:
	@echo "Running real environment tests..."
	@go test -mod=mod -tags realenv ./tests/integration/realenv/... -v -timeout 30m

test-realenv-clean: ## Cleanup real environment test infrastructure
	@echo "Cleaning up real environment..."
	@cd $(REALENV_DIR) && ./setup-three-env.sh cleanup 2>/dev/null || true
	@docker compose -f $(REALENV_DIR)/docker-compose.three-env.yml down -v 2>/dev/null || true

# Full realenv test cycle
test-realenv-full: test-realenv-clean test-realenv-setup test-realenv test-realenv-clean

clean: ## Remove build artifacts and clean test cache
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean -testcache

images:
	@echo "Building Govard Docker Images..."
	DOCKER_ORG=$(DOCKER_ORG) docker buildx bake -f docker/docker-bake.hcl

push:
	@echo "Pushing Govard Docker Images..."
	DOCKER_ORG=$(DOCKER_ORG) docker buildx bake -f docker/docker-bake.hcl --push
