BINARY_NAME=govard
BUILD_DIR=bin
TEST_BINARY=$(BUILD_DIR)/govard-test
UNIT_PACKAGES=$(shell go list ./... | grep -v '^govard/tests/integration$$')
COVER_PACKAGES=$(shell go list ./internal/... | tr '\n' ',' | sed 's/,$$//')
VERSION_RAW ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo 1.0.0)
VERSION ?= $(patsubst v%,%,$(VERSION_RAW))
LDFLAGS ?= -s -w -X govard/internal/cmd.Version=$(VERSION)

.PHONY: build clean test test-fast test-unit test-coverage test-integration test-integration-ci test-frontend build-test-binary install lint fmt vet push

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
	docker build --pull -t govard/apache:2.4 --build-arg APACHE_VERSION=2.4.66 docker/apache
	docker build --pull -t govard/apache:latest --build-arg APACHE_VERSION=2.4.66 docker/apache
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:8.15.0 --build-arg ELASTICSEARCH_VERSION=8.15.0 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:7.17.10 --build-arg ELASTICSEARCH_VERSION=7.17.10 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:7.16.3 --build-arg ELASTICSEARCH_VERSION=7.16.3 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:7.10.2 --build-arg ELASTICSEARCH_VERSION=7.10.2 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:7.9.3 --build-arg ELASTICSEARCH_VERSION=7.9.3 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:7.7.1 --build-arg ELASTICSEARCH_VERSION=7.7.1 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:7.6.2 --build-arg ELASTICSEARCH_VERSION=7.6.2 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:6.8.23 --build-arg ELASTICSEARCH_VERSION=6.8.23 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:5.6.16 --build-arg ELASTICSEARCH_VERSION=5.6.16 docker/elasticsearch
	docker build --pull -f docker/elasticsearch/Dockerfile -t govard/elasticsearch:2.4.6 --build-arg ELASTICSEARCH_VERSION=2.4.6 --build-arg ELASTICSEARCH_IMAGE=elasticsearch docker/elasticsearch
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:11.4 --build-arg MARIADB_VERSION=11.4 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.11 --build-arg MARIADB_VERSION=10.11 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.6 --build-arg MARIADB_VERSION=10.6 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.5 --build-arg MARIADB_VERSION=10.5 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.4 --build-arg MARIADB_VERSION=10.4 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.3 --build-arg MARIADB_VERSION=10.3 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.2 --build-arg MARIADB_VERSION=10.2 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.1 --build-arg MARIADB_VERSION=10.1 docker/mariadb
	docker build --pull -f docker/mariadb/Dockerfile -t govard/mariadb:10.0 --build-arg MARIADB_VERSION=10.0 docker/mariadb
	docker build --pull -f docker/mysql/Dockerfile -t govard/mysql:8.4 --build-arg MYSQL_VERSION=8.4 docker/mysql
	docker build --pull -f docker/mysql/Dockerfile -t govard/mysql:8.0 --build-arg MYSQL_VERSION=8.0 docker/mysql
	docker build --pull -f docker/mysql/Dockerfile -t govard/mysql:5.7 --build-arg MYSQL_VERSION=5.7 docker/mysql
	docker build --pull -t govard/nginx:1.28 --build-arg NGINX_VERSION=1.28.0 docker/nginx
	docker build --pull -t govard/nginx:latest --build-arg NGINX_VERSION=1.28.0 docker/nginx
	docker build --pull -f docker/opensearch/Dockerfile -t govard/opensearch:3.0.0 --build-arg OPENSEARCH_VERSION=3.0.0 docker/opensearch
	docker build --pull -f docker/opensearch/Dockerfile -t govard/opensearch:2.19.0 --build-arg OPENSEARCH_VERSION=2.19.0 docker/opensearch
	docker build --pull -f docker/opensearch/Dockerfile -t govard/opensearch:2.12.0 --build-arg OPENSEARCH_VERSION=2.12.0 docker/opensearch
	docker build --pull -f docker/opensearch/Dockerfile -t govard/opensearch:2.5.0 --build-arg OPENSEARCH_VERSION=2.5.0 docker/opensearch
	docker build --pull -f docker/opensearch/Dockerfile -t govard/opensearch:1.3.20 --build-arg OPENSEARCH_VERSION=1.3.20 docker/opensearch
	docker build --pull -f docker/opensearch/Dockerfile -t govard/opensearch:1.2.0 --build-arg OPENSEARCH_VERSION=1.2.0 docker/opensearch
	docker build --pull -f docker/php/Dockerfile -t govard/php:8.4 --build-arg PHP_VERSION=8.4 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:8.3 --build-arg PHP_VERSION=8.3 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:8.2 --build-arg PHP_VERSION=8.2 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:8.1 --build-arg PHP_VERSION=8.1 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:7.4 --build-arg PHP_VERSION=7.4 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:7.3 --build-arg PHP_VERSION=7.3 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:7.2 --build-arg PHP_VERSION=7.2 docker/php
	docker build --pull -f docker/php/Dockerfile -t govard/php:7.1 --build-arg PHP_VERSION=7.1 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.4 --build-arg PHP_VERSION=8.4 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.3 --build-arg PHP_VERSION=8.3 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.2 --build-arg PHP_VERSION=8.2 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.1 --build-arg PHP_VERSION=8.1 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:7.4 --build-arg PHP_VERSION=7.4 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:7.3 --build-arg PHP_VERSION=7.3 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:7.2 --build-arg PHP_VERSION=7.2 docker/php
	docker build --pull -f docker/php/magento2/Dockerfile -t govard/php-magento2:7.1 --build-arg PHP_VERSION=7.1 docker/php
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:4.1 --build-arg RABBITMQ_VERSION=4.1 docker/rabbitmq
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:3.13 --build-arg RABBITMQ_VERSION=3.13 docker/rabbitmq
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:3.12 --build-arg RABBITMQ_VERSION=3.12 docker/rabbitmq
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:3.11 --build-arg RABBITMQ_VERSION=3.11 docker/rabbitmq
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:3.9 --build-arg RABBITMQ_VERSION=3.9 docker/rabbitmq
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:3.8 --build-arg RABBITMQ_VERSION=3.8 docker/rabbitmq
	docker build --pull -f docker/rabbitmq/Dockerfile -t govard/rabbitmq:3.7 --build-arg RABBITMQ_VERSION=3.7 docker/rabbitmq
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:7.4 --build-arg REDIS_VERSION=7.4 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:7.2 --build-arg REDIS_VERSION=7.2 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:7.0 --build-arg REDIS_VERSION=7.0 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:6.2 --build-arg REDIS_VERSION=6.2 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:6.0 --build-arg REDIS_VERSION=6.0 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:5.0 --build-arg REDIS_VERSION=5.0 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:4.0 --build-arg REDIS_VERSION=4.0 docker/redis
	docker build --pull -f docker/redis/Dockerfile -t govard/redis:3.2 --build-arg REDIS_VERSION=3.2 docker/redis
	docker build --pull -f docker/valkey/Dockerfile -t govard/valkey:8.0 --build-arg VALKEY_VERSION=8.0 docker/valkey
	docker build --pull -f docker/valkey/Dockerfile -t govard/valkey:7.2 --build-arg VALKEY_VERSION=7.2 docker/valkey
	docker build --pull -t govard/varnish:7.6 --build-arg VARNISH_VERSION=7.6 docker/varnish
	docker build --pull -t govard/varnish:7.4 --build-arg VARNISH_VERSION=7.4 docker/varnish
	docker build --pull -t govard/varnish:7.0 --build-arg VARNISH_VERSION=7.0 docker/varnish
	docker build --pull -t govard/varnish:6.0 --build-arg VARNISH_VERSION=6.0 docker/varnish
	docker build --pull -t govard/varnish:latest --build-arg VARNISH_VERSION=7.6 docker/varnish

push:
	@echo "Pushing Govard Docker Images..."
	docker push govard/apache:2.4
	docker push govard/apache:latest
	docker push govard/elasticsearch:8.15.0
	docker push govard/elasticsearch:7.17.10
	docker push govard/elasticsearch:7.16.3
	docker push govard/elasticsearch:7.10.2
	docker push govard/elasticsearch:7.9.3
	docker push govard/elasticsearch:7.7.1
	docker push govard/elasticsearch:7.6.2
	docker push govard/elasticsearch:6.8.23
	docker push govard/elasticsearch:5.6.16
	docker push govard/elasticsearch:2.4.6
	docker push govard/mariadb:11.4
	docker push govard/mariadb:10.11
	docker push govard/mariadb:10.6
	docker push govard/mariadb:10.5
	docker push govard/mariadb:10.4
	docker push govard/mariadb:10.3
	docker push govard/mariadb:10.2
	docker push govard/mariadb:10.1
	docker push govard/mariadb:10.0
	docker push govard/mysql:8.4
	docker push govard/mysql:8.0
	docker push govard/mysql:5.7
	docker push govard/nginx:1.28
	docker push govard/nginx:latest
	docker push govard/opensearch:3.0.0
	docker push govard/opensearch:2.19.0
	docker push govard/opensearch:2.12.0
	docker push govard/opensearch:2.5.0
	docker push govard/opensearch:1.3.20
	docker push govard/opensearch:1.2.0
	docker push govard/php:8.4
	docker push govard/php:8.3
	docker push govard/php:8.2
	docker push govard/php:8.1
	docker push govard/php:7.4
	docker push govard/php:7.3
	docker push govard/php:7.2
	docker push govard/php:7.1
	docker push govard/php-magento2:8.4
	docker push govard/php-magento2:8.3
	docker push govard/php-magento2:8.2
	docker push govard/php-magento2:8.1
	docker push govard/php-magento2:7.4
	docker push govard/php-magento2:7.3
	docker push govard/php-magento2:7.2
	docker push govard/php-magento2:7.1
	docker push govard/rabbitmq:4.1
	docker push govard/rabbitmq:3.13
	docker push govard/rabbitmq:3.12
	docker push govard/rabbitmq:3.11
	docker push govard/rabbitmq:3.9
	docker push govard/rabbitmq:3.8
	docker push govard/rabbitmq:3.7
	docker push govard/redis:7.4
	docker push govard/redis:7.2
	docker push govard/redis:7.0
	docker push govard/redis:6.2
	docker push govard/redis:6.0
	docker push govard/redis:5.0
	docker push govard/redis:4.0
	docker push govard/redis:3.2
	docker push govard/valkey:8.0
	docker push govard/valkey:7.2
	docker push govard/varnish:7.6
	docker push govard/varnish:7.4
	docker push govard/varnish:7.0
	docker push govard/varnish:6.0
	docker push govard/varnish:latest
