.PHONY: all build test lint clean protobuf generate bench coverage ci

# Project variables
BINARY_NAME=pte-collector
BUILD_DIR=bin
GO_PKG=github.com/deepaksharma/trace-aware-reservoir-otel
VERSION=0.2.0
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Go commands
GO=go
GOBUILD=$(GO) build
GOTEST=$(GO) test
GOFMT=$(GO) fmt
GOLINT=golangci-lint
PROTOC=protoc

# Protobuf variables
PROTO_DIR=internal/processor/reservoirsampler/spanprotos
PROTO_FILES=$(wildcard $(PROTO_DIR)/*.proto)

all: protobuf generate build test

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pte

test:
	@echo "Running tests..."
	$(GOTEST) -race -cover -timeout=5m ./...

lint:
	@echo "Running linter..."
	$(GOFMT) ./...
	$(GOLINT) run ./...

bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchtime=2s -timeout=5m github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler/...
	@echo "Running performance tests..."
	$(GOTEST) -timeout=5m ./performance/...

coverage:
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=coverage.out -covermode=atomic -timeout=5m github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler/...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# Generate protobuf files
# Note: The reservoir sampler now uses custom binary serialization instead of protobuf for performance
protobuf:
	@echo "Generating protobuf files..."
	@mkdir -p $(PROTO_DIR)
	$(PROTOC) --go_out=. --go_opt=paths=source_relative $(PROTO_FILES)

# Generate mocks and other auto-generated files
generate:
	@echo "Running go generate..."
	$(GO) generate ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GO) mod tidy
	$(GO) mod download

# Run the collector
run:
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME) --config=config.yaml

# Test targets
unit-tests:
	@echo "Running unit tests..."
	$(GOTEST) -cover -timeout=3m ./internal/...

integration-tests:
	@echo "Running integration tests..."
	$(GOTEST) -cover -timeout=10m ./integration/...

integration-tests-short:
	@echo "Running integration tests in short mode..."
	$(GOTEST) -cover -timeout=3m -short ./integration/...

e2e-tests: build
	@echo "Running e2e tests..."
	$(GOTEST) -timeout=10m ./e2e/tests/...

# Run full CI locally
ci: deps build unit-tests integration-tests lint bench