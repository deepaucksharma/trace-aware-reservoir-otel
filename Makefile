.PHONY: all build test lint clean protobuf generate

# Project variables
BINARY_NAME=pte-collector
BUILD_DIR=bin
GO_PKG=github.com/deepaksharma/trace-aware-reservoir-otel
VERSION=0.1.0
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

all: protobuf generate build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pte

test:
	@echo "Running tests..."
	$(GOTEST) -race -cover ./...

lint:
	@echo "Running linter..."
	$(GOFMT) ./...
	$(GOLINT) run ./...

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

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

# Run the collector
run:
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME) --config=config.yaml