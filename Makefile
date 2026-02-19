BINARY_NAME=govard
BUILD_DIR=bin
TEST_BINARY=$(BUILD_DIR)/govard-test
UNIT_PACKAGES=$(shell go list ./... | grep -v '^govard/tests/integration$$')
VERSION_RAW ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo 1.0.0)
VERSION ?= $(patsubst v%,%,$(VERSION_RAW))
LDFLAGS ?= -s -w -X govard/internal/cmd.Version=$(VERSION)

.PHONY: build clean test test-fast test-unit test-integration test-integration-ci test-frontend build-test-binary install lint fmt vet

build:
	@echo "Building Govard..."
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/govard-darwin-arm64 cmd/govard/main.go
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/govard-linux-amd64 cmd/govard/main.go

test: test-frontend test-unit test-integration

test-fast: test-frontend test-unit

test-unit:
	@echo "Running unit tests..."
	go test $(UNIT_PACKAGES) -v -short

test-integration: build-test-binary
	@echo "Running integration tests..."
	go test -tags integration ./tests/integration/... -v -timeout 30m

test-integration-ci: build-test-binary
	@echo "Running integration tests (CI mode)..."
	go test -tags integration ./tests/integration/... -v -timeout 30m -parallel 4

test-frontend:
	@echo "Running frontend unit tests..."
	node --test tests/frontend/*.test.mjs

build-test-binary:
	@echo "Building test binary..."
	mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -tags integration -o $(TEST_BINARY) cmd/govard/main.go

lint:
	@echo "Running linter..."
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

install:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) cmd/govard/main.go
	sudo mv $(BINARY_NAME) /usr/local/bin/

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean -testcache

images:
	@echo "Building Govard Docker Images..."
	docker build -f docker/php/Dockerfile -t govard/php:8.4 --build-arg PHP_VERSION=8.4 docker/php
	docker build -f docker/php/Dockerfile -t govard/php:8.3 --build-arg PHP_VERSION=8.3 docker/php
	docker build -f docker/php/Dockerfile -t govard/php:8.1 --build-arg PHP_VERSION=8.1 --build-arg PHP_MEMORY_LIMIT=2G docker/php
	docker build -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.4 --build-arg PHP_VERSION=8.4 docker/php
	docker build -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.3 --build-arg PHP_VERSION=8.3 docker/php
	docker build -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.1 --build-arg PHP_VERSION=8.1 docker/php
	docker build -t govard/nginx:latest docker/nginx
	docker build -t govard/apache:latest docker/apache
	docker build -t govard/varnish:latest docker/varnish
