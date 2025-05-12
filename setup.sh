#!/bin/bash
# Setup script for trace-aware reservoir sampler

set -e

YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Setting up Trace-Aware Reservoir Sampler...${NC}"

# Check for required tools
echo "Checking dependencies..."

# Check Go version
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is not installed. Please install Go 1.21 or later.${NC}"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
MIN_VERSION="1.21"

if [ "$(printf '%s\n' "$MIN_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$MIN_VERSION" ]; then
    echo -e "${RED}Go version $GO_VERSION is too old. Please install Go $MIN_VERSION or later.${NC}"
    exit 1
fi

echo -e "${GREEN}Go version $GO_VERSION detected.${NC}"

# Check for other dependencies
for cmd in make git; do
    if ! command -v $cmd &> /dev/null; then
        echo -e "${RED}$cmd is not installed. Please install it first.${NC}"
        exit 1
    fi
done

# Create directories for checkpoint storage
echo "Creating directories for checkpoint storage..."
mkdir -p ./data

# Set up configuration
echo "Setting up configuration..."
if [ ! -f ./config.yaml ]; then
    echo "Creating default configuration file..."
    cat > ./config.yaml << EOF
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    send_batch_size: 1000
    timeout: 10s
    
  memory_limiter:
    check_interval: 1s
    limit_percentage: 80
    spike_limit_percentage: 25

  reservoir_sampler:
    size_k: 5000
    window_duration: 60s
    checkpoint_path: ./data/reservoir_checkpoint.db
    checkpoint_interval: 10s
    trace_aware: true
    trace_buffer_max_size: 100000
    trace_buffer_timeout: 10s
    db_compaction_schedule_cron: "0 0 * * *"
    db_compaction_target_size: 104857600

exporters:
  debug:
    verbosity: detailed

  # Uncomment and configure for New Relic integration
  # otlphttp:
  #   endpoint: "https://otlp.nr-data.net:4318"
  #   headers:
  #     api-key: \${NEW_RELIC_LICENSE_KEY}

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, reservoir_sampler]
      exporters: [debug]
      # For New Relic: exporters: [debug, otlphttp]

  telemetry:
    metrics:
      level: detailed
EOF
    echo -e "${GREEN}Created default configuration in config.yaml${NC}"
else
    echo -e "${YELLOW}config.yaml already exists, not overwriting.${NC}"
fi

# Fix permissions on scripts
echo "Setting execute permissions on scripts..."
chmod +x ./setup.sh

# Download dependencies
echo "Downloading Go dependencies..."
go mod tidy

# Build
echo "Building the project..."
make build

echo -e "${GREEN}Setup complete!${NC}"
echo
echo "To run the collector with trace-aware reservoir sampling:"
echo -e "${YELLOW}./bin/pte-collector --config=config.yaml${NC}"
echo
echo "To enable New Relic integration:"
echo -e "${YELLOW}export NEW_RELIC_LICENSE_KEY=your-license-key${NC}"
echo "Then uncomment the otlphttp exporter section in config.yaml"
echo
echo "For more information, see:"
echo -e "${YELLOW}docs/TECHNICAL_GUIDE.md${NC}"
echo -e "${YELLOW}examples/config-examples.yaml${NC}"
echo
echo -e "${GREEN}Happy sampling!${NC}"