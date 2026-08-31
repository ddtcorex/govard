DOCKER_ORG ?= ddtcorex/govard-
HOST_OS := $(shell uname -s)
DOCKER_MULTIARCH_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_BAKE ?= ./scripts/docker-buildx-bake.sh
ifeq ($(HOST_OS),Darwin)
DOCKER_PLATFORMS ?= $(DOCKER_MULTIARCH_PLATFORMS)
else
DOCKER_PLATFORMS ?=
endif
comma := ,
DOCKER_PLATFORM_FLAGS := $(foreach platform,$(subst $(comma), ,$(DOCKER_PLATFORMS)),--set "*.platform=$(platform)")
BINARY_NAME=govard
BUILD_DIR=bin
TEST_BINARY=$(BUILD_DIR)/govard-test
UNIT_PACKAGES=$(shell go list ./... | grep -v '^govard/tests/integration$$')
COVER_PACKAGES=$(shell go list ./internal/... | tr '\n' ',' | sed 's/,$$//')
VERSION_RAW ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo 1.0.0)
VERSION ?= $(patsubst v%,%,$(VERSION_RAW))
GOLANGCI_LINT_VERSION ?= v2.11.3
GOLANGCI_LINT_BIN ?= $(shell go env GOPATH)/bin/golangci-lint
LDFLAGS ?= -s -w -X govard/internal/cmd.Version=$(VERSION) -X govard/internal/desktop.Version=$(VERSION)

.PHONY: help install install-release build-test-binary build clean test test-unit test-coverage test-integration test-integration-ci test-frontend lint lint-install fmt fmt-check vet generate generate-check images push

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

install: ## Build and install Govard CLI + Desktop binaries from current source tree
	./install.sh --source --source-dir "$(CURDIR)" -y

install-release: ## Install latest release Govard CLI + Desktop binaries to system
	./install.sh -y

# build-frontend regenerates the desktop app's embedded CSS from its Tailwind
# source. Only run it deliberately after editing desktop/frontend/assets/styles-src.css
# and commit the result - cmd/govard/main.go (built below) does not import
# desktop/frontend's embed.FS at all, and cmd/govard-desktop (built by install.sh)
# embeds whatever is already committed, so neither needs this as a prerequisite.
build-frontend:
	@echo "Building frontend assets..."
	@cd desktop/frontend && yarn install && yarn run build:css

build: generate ## Build Govard binary for the current platform
	@echo "Building Govard..."
	mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) cmd/govard/main.go

build-test-binary:
	@echo "Building test binary..."
	mkdir -p $(BUILD_DIR)
	go build -mod=mod -ldflags "$(LDFLAGS)" -tags integration -o $(TEST_BINARY) cmd/govard/main.go

generate: ## Regenerate internal/frameworks/all_generated.go
	go generate ./internal/frameworks/...

generate-check: generate ## Fail if generated files are stale (CI drift check)
	@if ! git diff --quiet -- internal/frameworks/all_generated.go; then \
		echo "internal/frameworks/all_generated.go is out of date - run 'make generate' and commit the result:"; \
		git diff -- internal/frameworks/all_generated.go; \
		exit 1; \
	fi

test: generate lint fmt-check vet test-frontend test-unit test-integration ## Run all tests


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

lint-install: ## Install golangci-lint if missing
	@if ! command -v $(GOLANGCI_LINT_BIN) >/dev/null 2>&1 || ! $(GOLANGCI_LINT_BIN) version | grep -q $(GOLANGCI_LINT_VERSION); then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

lint: lint-install ## Run linter (synchronized with CI)
	@echo "Running linter..."
	$(GOLANGCI_LINT_BIN) run ./...

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

clean: ## Remove build artifacts and clean test cache
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean -testcache

images: ## Build Govard Docker images
	@echo "Building Govard Docker Images..."
	DOCKER_ORG=$(DOCKER_ORG) DOCKER_PLATFORMS="$(DOCKER_PLATFORMS)" $(DOCKER_BAKE) -f docker/docker-bake.hcl $(DOCKER_PLATFORM_FLAGS)

push: ## Push Govard Docker images as multi-arch manifests
	@echo "Pushing Govard Docker Images for $(DOCKER_PLATFORMS)..."
	DOCKER_ORG=$(DOCKER_ORG) DOCKER_PLATFORMS="$(DOCKER_PLATFORMS)" $(DOCKER_BAKE) -f docker/docker-bake.hcl $(DOCKER_PLATFORM_FLAGS) --push
