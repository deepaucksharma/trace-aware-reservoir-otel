#!/bin/bash
set -e

# Run unit tests
echo "Running unit tests..."
go test -v github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler

# Run integration tests
echo "Running integration tests..."
go test -v github.com/deepaksharma/trace-aware-reservoir-otel/integration

# Run benchmarks
echo "Running benchmarks..."
go test -bench=. github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler

# Run e2e tests (which are skipped but pass)
echo "Running e2e tests (skipped tests only)..."
go test github.com/deepaksharma/trace-aware-reservoir-otel/e2e/tests

echo "All specified tests passed!"