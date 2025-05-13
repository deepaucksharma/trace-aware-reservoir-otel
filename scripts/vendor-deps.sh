# Vendor dependencies script
# This ensures deterministic, reproducible builds by vendoring all dependencies

set -e

echo "Ensuring Go module dependencies are up-to-date..."
go mod tidy

echo "Vendoring Go modules for deterministic builds..."
go mod vendor

echo "Verifying build with vendored dependencies..."
go build -mod=vendor -o bin/otelcol-reservoir ./cmd/otelcol-reservoir

echo "✅ Go modules successfully vendored"
