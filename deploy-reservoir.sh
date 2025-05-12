#!/bin/bash
# All-in-one script to build and deploy trace-aware reservoir sampling with NR-DOT
# Usage: ./deploy-reservoir.sh [build|deploy|validate|all]

set -e

# Configuration (edit these values)
# --------------------------------------
REGISTRY="ghcr.io/your-org"          # Your container registry
IMAGE_NAME="nrdot-reservoir"          # Image name
TAG="v0.1.0"                          # Image tag
NAMESPACE="observability"             # Kubernetes namespace
RELEASE_NAME="nr-otel"                # Helm release name
# --------------------------------------

# Color codes for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print section header
section() {
  echo ""
  echo -e "${BLUE}==>${NC} ${YELLOW}$1${NC}"
  echo ""
}

# Check prerequisites
check_prereqs() {
  section "Checking prerequisites"
  
  local missing=0
  for cmd in git go docker kubectl helm; do
    if ! command -v $cmd &> /dev/null; then
      echo -e "${RED}✗ $cmd not found${NC}"
      missing=1
    else
      echo -e "${GREEN}✓ $cmd found${NC}"
    fi
  done
  
  if [ $missing -eq 1 ]; then
    echo -e "${RED}Please install missing prerequisites and try again.${NC}"
    exit 1
  fi
  
  # Check for license key if deploying
  if [ "$1" == "deploy" ] || [ "$1" == "all" ]; then
    if [ -z "$NR_LICENSE_KEY" ]; then
      echo -e "${YELLOW}Warning: NR_LICENSE_KEY environment variable not set.${NC}"
      echo "Please set your New Relic license key with:"
      echo "export NR_LICENSE_KEY=your-license-key"
      echo ""
      read -p "Do you want to continue anyway? (y/n) " -n 1 -r
      echo ""
      if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
      fi
    else
      echo -e "${GREEN}✓ NR_LICENSE_KEY is set${NC}"
    fi
  fi
}

# Build custom NR-DOT with reservoir sampler
build() {
  section "Building custom NR-DOT with reservoir sampler"
  
  # Create build directory
  BUILD_DIR="$(pwd)/build_nrdot"
  mkdir -p $BUILD_DIR
  cd $BUILD_DIR
  
  # Clone NR-DOT repository if needed
  if [ ! -d "opentelemetry-collector-releases" ]; then
    echo "Cloning NR-DOT repository..."
    git clone https://github.com/newrelic/opentelemetry-collector-releases.git
  fi
  
  cd opentelemetry-collector-releases
  
  # Create a new branch
  BRANCH="feat/reservoir-sampler"
  git checkout -b $BRANCH 2>/dev/null || git checkout $BRANCH
  
  # Modify distribution.yaml to include our processor
  DIST_FILE="distributions/nrdot-collector/distribution.yaml"
  echo "Adding reservoir sampler to distribution manifest..."
  
  # Make backup if not already done
  if [ ! -f "${DIST_FILE}.bak" ]; then
    cp $DIST_FILE ${DIST_FILE}.bak
  fi
  
  # Add reservoir sampler to processors section
  # (If distribution.yaml format changes, this might need adjustment)
  RESERVOIR_PATH="github.com/deepaucksharma/nr-trace-aware-reservoir-otel/internal/processor/reservoirsampler"
  FILE_STORAGE_PATH="github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension"
  
  # Add processor if not already present
  if ! grep -q "$RESERVOIR_PATH" $DIST_FILE; then
    sed -i.tmp '/^processors:/,/^exporters:/{/^processors:/b;/^exporters:/b;s/^$/  - '"$RESERVOIR_PATH"'\n/}' $DIST_FILE
  fi
  
  # Add file storage extension if not already present
  if ! grep -q "$FILE_STORAGE_PATH" $DIST_FILE; then
    sed -i.tmp '/^extensions:/,/^receivers:/{/^extensions:/b;/^receivers:/b;s/^$/  - '"$FILE_STORAGE_PATH"'\n/}' $DIST_FILE
  fi
  
  # Clean up temporary files
  rm -f ${DIST_FILE}.tmp
  
  # Add the module dependency
  echo "Adding reservoir sampler module dependency..."
  go get github.com/deepaucksharma/nr-trace-aware-reservoir-otel@v0.1.0
  go mod tidy
  
  # Build the distribution
  echo "Building NR-DOT distribution..."
  make dist
  
  # Verify the build
  BINARY="./_dist/nrdot/otelcol-nrdot"
  if [ ! -f "$BINARY" ]; then
    echo -e "${RED}Build failed! Binary not found at $BINARY${NC}"
    exit 1
  fi
  
  echo -e "${GREEN}✓ Build successful!${NC}"
  
  # Check if reservoir sampler is included
  if ! $BINARY components | grep -q "processor.+reservoir_sampler"; then
    echo -e "${YELLOW}Warning: reservoir_sampler not found in components list${NC}"
    echo "This may indicate an issue with the build configuration."
  else
    echo -e "${GREEN}✓ Reservoir sampler found in components${NC}"
  fi
  
  # Build Docker image
  echo "Building Docker image: $REGISTRY/$IMAGE_NAME:$TAG"
  cd ./_dist/nrdot/
  docker build -t $REGISTRY/$IMAGE_NAME:$TAG .
  
  # Create values file in parent directory
  generate_values_file
  
  cd ../../../../
  echo -e "${GREEN}✓ Build and image creation complete!${NC}"
}

# Generate values file for Helm using reservoir-config.yaml
generate_values_file() {
  section "Generating Helm values file"

  VALUES_FILE="../../../values-reservoir.yaml"
  CONFIG_FILE="../../../reservoir-config.yaml"

  # Check if config file exists
  if [ ! -f "$CONFIG_FILE" ]; then
    echo -e "${YELLOW}Config file not found: $CONFIG_FILE${NC}"
    echo "Using default configuration..."

    # Default values if config file is missing
    SIZE_K=5000
    WINDOW_DURATION="60s"
    CHECKPOINT_PATH="/var/otelpersist/reservoir.db"
    CHECKPOINT_INTERVAL="10s"
    TRACE_AWARE=true
    TRACE_BUFFER_MAX_SIZE=100000
    TRACE_BUFFER_TIMEOUT="30s"
    DB_COMPACTION_CRON="0 2 * * 0"
    PERSISTENCE_SIZE="1Gi"
  else
    # Parse configuration from YAML file
    echo "Reading configuration from $CONFIG_FILE"

    # Extract values - this is a simple extraction method
    # For more robust YAML parsing, consider using a tool like yq
    SIZE_K=$(grep "^size_k:" "$CONFIG_FILE" | awk '{print $2}')
    WINDOW_DURATION=$(grep "^window_duration:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    CHECKPOINT_PATH=$(grep "^checkpoint_path:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    CHECKPOINT_INTERVAL=$(grep "^checkpoint_interval:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    TRACE_AWARE=$(grep "^trace_aware:" "$CONFIG_FILE" | awk '{print $2}')
    TRACE_BUFFER_MAX_SIZE=$(grep "^trace_buffer_max_size:" "$CONFIG_FILE" | awk '{print $2}')
    TRACE_BUFFER_TIMEOUT=$(grep "^trace_buffer_timeout:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    DB_COMPACTION_CRON=$(grep "^db_compaction_schedule_cron:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    PERSISTENCE_SIZE=$(grep -A 3 "^persistence:" "$CONFIG_FILE" | grep "size:" | awk '{print $2}')

    # Set defaults for any missing values
    [ -z "$SIZE_K" ] && SIZE_K=5000
    [ -z "$WINDOW_DURATION" ] && WINDOW_DURATION="60s"
    [ -z "$CHECKPOINT_PATH" ] && CHECKPOINT_PATH="/var/otelpersist/reservoir.db"
    [ -z "$CHECKPOINT_INTERVAL" ] && CHECKPOINT_INTERVAL="10s"
    [ -z "$TRACE_AWARE" ] && TRACE_AWARE=true
    [ -z "$TRACE_BUFFER_MAX_SIZE" ] && TRACE_BUFFER_MAX_SIZE=100000
    [ -z "$TRACE_BUFFER_TIMEOUT" ] && TRACE_BUFFER_TIMEOUT="30s"
    [ -z "$DB_COMPACTION_CRON" ] && DB_COMPACTION_CRON="0 2 * * 0"
    [ -z "$PERSISTENCE_SIZE" ] && PERSISTENCE_SIZE="1Gi"
  fi

  # Create values file with the configuration
  cat > $VALUES_FILE << EOL
# NR-DOT Helm values for trace-aware reservoir sampling
image:
  repository: ${REGISTRY}/${IMAGE_NAME}
  tag: ${TAG}

# Required for checkpoint persistence
persistence:
  enabled: true
  size: ${PERSISTENCE_SIZE}

collector:
  configOverride:
    processors:
      reservoir_sampler:
        size_k: ${SIZE_K}
        window_duration: "${WINDOW_DURATION}"
        checkpoint_path: "${CHECKPOINT_PATH}"
        checkpoint_interval: "${CHECKPOINT_INTERVAL}"
        trace_aware: ${TRACE_AWARE}
        trace_buffer_max_size: ${TRACE_BUFFER_MAX_SIZE}
        trace_buffer_timeout: "${TRACE_BUFFER_TIMEOUT}"
        db_compaction_schedule_cron: "${DB_COMPACTION_CRON}"

    service:
      extensions: [health_check, pprof, memory_ballast, file_storage]

      pipelines:
        traces:
          receivers: [otlp]
          processors: [memory_limiter, batch, reservoir_sampler]
          exporters: [otlphttp/newrelic]
EOL

  echo -e "${GREEN}✓ Values file created: $VALUES_FILE${NC}"
}

# Deploy using Helm
deploy() {
  section "Deploying NR-DOT with reservoir sampler"
  
  # Add Helm repo
  helm repo add newrelic https://helm-charts.newrelic.com 2>/dev/null || true
  helm repo update
  
  # Create namespace if needed
  kubectl get namespace $NAMESPACE &> /dev/null || kubectl create namespace $NAMESPACE
  echo -e "${GREEN}✓ Namespace $NAMESPACE ready${NC}"
  
  # Install/upgrade with Helm
  if [ -n "$NR_LICENSE_KEY" ]; then
    helm upgrade --install $RELEASE_NAME newrelic/nrdot-collector \
      --namespace $NAMESPACE \
      -f values-reservoir.yaml \
      --set licenseKey=$NR_LICENSE_KEY
  else
    helm upgrade --install $RELEASE_NAME newrelic/nrdot-collector \
      --namespace $NAMESPACE \
      -f values-reservoir.yaml
  fi
  
  echo -e "${GREEN}✓ Helm deployment complete${NC}"
  
  # Wait for pod to be ready
  echo "Waiting for pod to be ready..."
  if ! kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=nrdot-collector -n $NAMESPACE --timeout=120s; then
    echo -e "${RED}Pod not ready after 2 minutes. Check logs for issues:${NC}"
    kubectl get pods -n $NAMESPACE
    exit 1
  fi
}

# Validate the deployment
validate() {
  section "Validating deployment"
  
  # Get pod name
  POD=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')
  if [ -z "$POD" ]; then
    echo -e "${RED}No pod found for nrdot-collector in namespace $NAMESPACE${NC}"
    exit 1
  fi
  
  echo "Pod: $POD"
  
  # Check configuration
  echo -n "Checking for reservoir_sampler in configuration: "
  if kubectl exec -n $NAMESPACE $POD -- grep -q "reservoir_sampler" /etc/otelcol-config.yaml; then
    echo -e "${GREEN}✓ Found${NC}"
  else
    echo -e "${RED}✗ Not found${NC}"
    echo "This indicates a configuration issue."
  fi
  
  # Check metrics
  echo -n "Checking for reservoir metrics: "
  if kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep -q "pte_reservoir"; then
    echo -e "${GREEN}✓ Found${NC}"
  else
    echo -e "${YELLOW}✗ Not found yet${NC}"
    echo "Metrics may take a moment to appear. Try again shortly."
  fi
  
  # Check checkpoint file
  echo -n "Checking for checkpoint file: "
  if kubectl exec -n $NAMESPACE $POD -- ls -la /var/otelpersist/ 2>/dev/null | grep -q "reservoir.db"; then
    echo -e "${GREEN}✓ Found${NC}"
  else
    echo -e "${YELLOW}✗ Not found yet${NC}"
    echo "The checkpoint file will be created after the first checkpoint interval (10s)."
  fi
  
  # Display status info
  echo ""
  echo "Deployment status:"
  kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=nrdot-collector
  
  echo ""
  echo -e "${GREEN}Validation complete.${NC}"
  echo ""
  echo "To monitor the processor metrics:"
  echo "kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir"
  echo ""
  echo "To check the logs:"
  echo "kubectl logs -n $NAMESPACE $POD"
}

# Show help
show_help() {
  echo "Usage: $0 [command]"
  echo ""
  echo "Commands:"
  echo "  build     - Build custom NR-DOT with reservoir sampler"
  echo "  deploy    - Deploy using Helm (requires values-reservoir.yaml file)"
  echo "  validate  - Validate the deployment"
  echo "  all       - Execute all steps (build, deploy, validate)"
  echo "  help      - Show this help message"
  echo ""
  echo "Examples:"
  echo "  $0 all                 # Run complete process"
  echo "  $0 build               # Only build the image"
  echo "  $0 deploy              # Only deploy (requires values file)"
  echo "  $0 validate            # Only validate existing deployment"
  echo ""
  echo "Configuration:"
  echo "  Edit the variables at the top of this script to customize:"
  echo "  - REGISTRY=\"$REGISTRY\""
  echo "  - IMAGE_NAME=\"$IMAGE_NAME\""
  echo "  - TAG=\"$TAG\""
  echo "  - NAMESPACE=\"$NAMESPACE\""
  echo "  - RELEASE_NAME=\"$RELEASE_NAME\""
  echo ""
  echo "Before deploying, set your New Relic license key:"
  echo "  export NR_LICENSE_KEY=your-license-key"
}

# Main execution
case "$1" in
  build)
    check_prereqs
    build
    ;;
  deploy)
    check_prereqs deploy
    deploy
    ;;
  validate)
    check_prereqs
    validate
    ;;
  all)
    check_prereqs all
    build
    deploy
    validate
    ;;
  help|--help|-h)
    show_help
    ;;
  *)
    show_help
    exit 1
    ;;
esac

exit 0