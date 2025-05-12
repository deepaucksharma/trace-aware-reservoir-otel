#!/bin/bash
# Enhanced build script for integrating trace-aware reservoir sampler with NR-DOT
# Based on validated approach from NR-DOT documentation

set -e

# Configuration variables - modify as needed
NRDOT_REPO="https://github.com/newrelic/opentelemetry-collector-releases.git"
RESERVOIR_MODULE="github.com/deepaucksharma/nr-trace-aware-reservoir-otel"
RESERVOIR_VERSION="v0.1.0"  # Update to your version
CUSTOM_IMAGE_NAME="nrdot-reservoir"
CUSTOM_IMAGE_TAG="v0.1.0"
REGISTRY="ghcr.io/your-org"  # Replace with your registry

# Color codes for output
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Building NR-DOT with trace-aware reservoir sampler integration...${NC}"

# Check required tools
for cmd in git go docker; do
  if ! command -v $cmd &> /dev/null; then
    echo -e "${RED}Error: $cmd is not installed. Please install it first.${NC}"
    exit 1
  fi
done

# Check Go version - should match NR-DOT requirements
GO_VERSION=$(go version | sed -E 's/.*go([0-9]+\.[0-9]+).*/\1/')
MIN_VERSION="1.21"
if [ $(echo "$GO_VERSION < $MIN_VERSION" | bc -l) -eq 1 ]; then
  echo -e "${RED}Error: Go version $GO_VERSION is too old. NR-DOT requires Go $MIN_VERSION or newer.${NC}"
  exit 1
fi
echo -e "${GREEN}Using Go $GO_VERSION${NC}"

# Create work directory
WORK_DIR="$(pwd)/build_nrdot"
mkdir -p $WORK_DIR
cd $WORK_DIR
echo -e "Working in directory: ${WORK_DIR}"

# Clone NR-DOT repository if it doesn't exist
if [ ! -d "opentelemetry-collector-releases" ]; then
  echo "Cloning NR-DOT repository..."
  git clone $NRDOT_REPO
fi

cd opentelemetry-collector-releases

# Create a new branch
BRANCH_NAME="feat/reservoir-sampler-$(date +%Y%m%d%H%M%S)"
git checkout -b $BRANCH_NAME
echo -e "${GREEN}Created branch: $BRANCH_NAME${NC}"

# Optional: Set up Go workspace for faster local iteration
echo "Setting up Go workspace for local development..."
cd ..
go work init
go work use ./opentelemetry-collector-releases
# Uncomment if you have the local reservoir sampler code available
# go work use ../../../internal/processor/reservoirsampler
cd opentelemetry-collector-releases

# Check for existing distribution.yaml
DIST_FILE="distributions/nrdot-collector/distribution.yaml"
if [ ! -f "$DIST_FILE" ]; then
  echo -e "${RED}Error: distribution.yaml not found at $DIST_FILE - is this the correct repo?${NC}"
  exit 1
fi

# Backup original file
cp $DIST_FILE ${DIST_FILE}.bak
echo "Backed up original distribution.yaml"

# Update distribution.yaml to include our processor
echo "Updating distribution.yaml to include reservoir sampler..."
cat $DIST_FILE | awk '
/^processors:/ {
  print;
  p = 1;
  next;
}
/^exporters:/ {
  if (p && !added) {
    print "  - github.com/deepaucksharma/nr-trace-aware-reservoir-otel/internal/processor/reservoirsampler";
    added = 1;
  }
  p = 0;
  print;
  next;
}
/^extensions:/ {
  print;
  e = 1;
  next;
}
/^receivers:/ {
  if (e && !ext_added) {
    print "  - github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension";
    ext_added = 1;
  }
  e = 0;
  print;
  next;
}
p && /^  -/ && !added {
  print;
  next;
}
e && /^  -/ && !ext_added {
  print;
  next;
}
{print}
END {
  if (p && !added) {
    print "  - github.com/deepaucksharma/nr-trace-aware-reservoir-otel/internal/processor/reservoirsampler";
  }
  if (e && !ext_added) {
    print "  - github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension";
  }
}' > ${DIST_FILE}.new

mv ${DIST_FILE}.new $DIST_FILE

# Add the dependency
echo "Adding reservoir sampler dependency..."
go get $RESERVOIR_MODULE@$RESERVOIR_VERSION
go mod tidy

# Commit the changes
git add $DIST_FILE go.mod go.sum
git commit -m "feat: include reservoir_sampler processor and filestorageextension"
echo -e "${GREEN}Changes committed to git${NC}"

# Build the distribution
echo "Building custom NR-DOT distribution..."
make dist

# Verify the build
BINARY="./_dist/nrdot/otelcol-nrdot"
if [ -f "$BINARY" ]; then
  echo -e "${GREEN}Build successful!${NC}"
  
  # Verify the reservoir sampler is included
  echo "Verifying reservoir sampler is included in the build:"
  RESULT=$($BINARY components | grep -E "processor.+reservoir_sampler" || echo "NOT FOUND")
  
  if [[ "$RESULT" != *"NOT FOUND"* ]]; then
    echo -e "${GREEN}✓ Reservoir sampler found in components!${NC}"
    echo "  $RESULT"
  else
    echo -e "${RED}✗ Warning: reservoir_sampler not found in components list!${NC}"
    echo "  This might indicate the processor wasn't properly linked"
  fi
else
  echo -e "${RED}Build failed! Output binary not found at $BINARY${NC}"
  exit 1
fi

# Build Docker image
echo "Building Docker image..."
cd ./_dist/nrdot/
docker build -t $REGISTRY/$CUSTOM_IMAGE_NAME:$CUSTOM_IMAGE_TAG .

# Create values file for Helm
echo "Creating Helm values for deployment..."
cat > values.reservoir.yaml << EOL
# NR-DOT Helm chart values for trace-aware reservoir sampling
image:
  repository: ${REGISTRY}/${CUSTOM_IMAGE_NAME}
  tag: ${CUSTOM_IMAGE_TAG}

# Required for checkpoint persistence
persistence:
  enabled: true
  size: 1Gi

collector:
  configOverride:
    processors:
      reservoir_sampler:
        size_k: 5000
        window_duration: "60s"
        trace_buffer_timeout: "30s"
        # Path within the standard persistence mount
        checkpoint_path: "/var/otelpersist/reservoir.db"
        checkpoint_interval: "10s"
        trace_aware: true
        trace_buffer_max_size: 100000
        db_compaction_schedule_cron: "0 2 * * 0"  # Weekly @ 2 AM Sunday

    service:
      extensions: [health_check, pprof, memory_ballast, file_storage]
      
      pipelines:
        traces:
          receivers: [otlp]
          processors: [memory_limiter, batch, reservoir_sampler]
          exporters: [otlphttp/newrelic]
EOL

echo -e "${GREEN}Docker image built successfully: $REGISTRY/$CUSTOM_IMAGE_NAME:$CUSTOM_IMAGE_TAG${NC}"
echo -e "${GREEN}Helm values created in: $(pwd)/values.reservoir.yaml${NC}"
echo ""
echo "To push the image to your registry:"
echo -e "${YELLOW}docker push $REGISTRY/$CUSTOM_IMAGE_NAME:$CUSTOM_IMAGE_TAG${NC}"
echo ""
echo "To deploy with Helm using the provided values:"
echo -e "${YELLOW}helm upgrade --install nr-otel newrelic/nrdot-collector \\"
echo "  --namespace observability \\"
echo "  --create-namespace \\"
echo "  --values values.reservoir.yaml \\"
echo "  --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY${NC}"
echo ""
echo "To verify deployment:"
echo -e "${YELLOW}POD=\$(kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')"
echo "kubectl exec -n observability \$POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir${NC}"
echo ""
echo -e "${GREEN}Integration complete!${NC}"