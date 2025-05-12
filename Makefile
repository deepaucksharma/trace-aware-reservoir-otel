.PHONY: all build test lint clean protobuf generate bench coverage ci install docs examples e2e-tests integration-tests integration-tests-short stress-tests performance-tests

# Project variables
PROJECT_NAME=trace-aware-reservoir-otel
BINARY_NAME=pte
BUILD_DIR=bin
GO_PKG=github.com/deepaksharma/trace-aware-reservoir-otel
VERSION=0.3.0
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)"

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

# All targets (default)
all: deps protobuf generate build test

# Build all binaries
build: build-pte build-examples

# Build main command-line tool
build-pte:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pte

# Build end-to-end test runner
build-e2e:
	@echo "Building e2e test runner..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/e2e-tests ./cmd/e2e-tests

# Build examples
build-examples:
	@echo "Building examples..."
	@if [ -d "./cmd/examples" ]; then \
		mkdir -p $(BUILD_DIR)/examples; \
		for example in $$(find ./cmd/examples -mindepth 1 -maxdepth 1 -type d); do \
			example_name=$$(basename $$example); \
			echo "  Building example: $$example_name"; \
			$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/examples/$$example_name ./cmd/examples/$$example_name; \
		done; \
	fi

# Run tests
test: unit-tests integration-tests-short lint

# Run unit tests
unit-tests:
	@echo "Running unit tests..."
	$(GOTEST) -race -cover -timeout=3m ./internal/...

# Run integration tests (short mode)
integration-tests-short:
	@echo "Running integration tests in short mode..."
	$(GOTEST) -race -cover -timeout=3m -short ./integration/...

# Run integration tests (full)
integration-tests:
	@echo "Running integration tests..."
	$(GOTEST) -race -cover -timeout=10m ./integration/...

# Run stress tests
stress-tests:
	@echo "Running stress tests..."
	$(GOTEST) -timeout=30m ./integration/stress_test.go

# Run performance tests
performance-tests:
	@echo "Running performance tests..."
	$(GOTEST) -timeout=15m ./integration/performance_test.go

# Run end-to-end tests
e2e-tests: build build-e2e
	@echo "Running e2e tests..."
	./$(BUILD_DIR)/e2e-tests --preset low-load --test standard

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchtime=2s -timeout=5m ./internal/processor/reservoirsampler/...
	@if [ -d "./performance" ]; then \
		echo "Running performance tests..."; \
		$(GOTEST) -timeout=5m ./performance/...; \
	fi

# Run linter
lint:
	@echo "Running linter..."
	$(GOFMT) ./...
	$(GOLINT) run ./...

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=coverage.out -covermode=atomic -timeout=5m ./internal/...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

# Clean build outputs
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# Generate protobuf files
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

# Install the collector locally
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin/$(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installation complete."

# Run the PTE tool with default info command
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME) info --config --metrics

# Generate documentation site
docs:
	@echo "Generating documentation site..."
	@mkdir -p $(BUILD_DIR)/docs
	@cp -r docs/* $(BUILD_DIR)/docs
	@echo "Documentation site generated at $(BUILD_DIR)/docs"

# Create example configurations
examples:
	@echo "Generating example configurations..."
	@mkdir -p examples/configs
	@./$(BUILD_DIR)/$(BINARY_NAME) generate-config --template default --output examples/configs/default.yaml
	@./$(BUILD_DIR)/$(BINARY_NAME) generate-config --template high-volume --output examples/configs/high-volume.yaml
	@./$(BUILD_DIR)/$(BINARY_NAME) generate-config --template low-resource --output examples/configs/low-resource.yaml
	@./$(BUILD_DIR)/$(BINARY_NAME) nrdot-integration --generate-config --output examples/configs/nrdot-default.yaml
	@echo "Example configurations generated in examples/configs/"

# Run CI workflow
ci: deps build test integration-tests bench

# Create a release package
release: all docs examples
	@echo "Creating release package..."
	@mkdir -p $(BUILD_DIR)/release/$(PROJECT_NAME)-$(VERSION)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/release/$(PROJECT_NAME)-$(VERSION)/
	@cp -r examples $(BUILD_DIR)/release/$(PROJECT_NAME)-$(VERSION)/
	@cp -r docs $(BUILD_DIR)/release/$(PROJECT_NAME)-$(VERSION)/
	@cp README.md LICENSE $(BUILD_DIR)/release/$(PROJECT_NAME)-$(VERSION)/
	@cd $(BUILD_DIR)/release && tar -czf $(PROJECT_NAME)-$(VERSION).tar.gz $(PROJECT_NAME)-$(VERSION)
	@echo "Release package created at $(BUILD_DIR)/release/$(PROJECT_NAME)-$(VERSION).tar.gz"