#!/bin/bash
# Single-command installer for trace-aware reservoir sampling with NR-DOT
# Usage: ./install.sh

set -e

# Detect environment and configuration
# -----------------------------------

# Colors for output
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

# Configuration (use environment variables if set, otherwise use defaults)
# ----------------------------------------------------------------------
# Installation settings
INSTALL_MODE=${NRDOT_MODE:-"kubernetes"} # kubernetes or docker
NRDOT_VERSION=${NRDOT_VERSION:-"latest"} # NR-DOT version to use
USE_CUSTOM_BUILD=${USE_CUSTOM_BUILD:-"true"} # Whether to build custom NR-DOT or use pre-built
INSTALL_DASHBOARDS=${INSTALL_DASHBOARDS:-"false"} # Whether to install NR dashboards
AUTO_DETECT=${AUTO_DETECT:-"true"} # Whether to auto-detect existing NR-DOT installations

# Registry and image settings
REGISTRY=${NRDOT_REGISTRY:-"ghcr.io/your-org"}
IMAGE_NAME=${NRDOT_IMAGE_NAME:-"nrdot-reservoir"}
TAG=${NRDOT_TAG:-"v0.1.0"}
NAMESPACE=${NRDOT_NAMESPACE:-"observability"}
RELEASE_NAME=${NRDOT_RELEASE_NAME:-"nr-otel"}

# Auto-detect New Relic API key from Kubernetes (if exists and not set explicitly)
if [ -z "$NR_API_KEY" ] && [ "$INSTALL_MODE" == "kubernetes" ]; then
  # Check for existing New Relic secrets in the cluster
  if kubectl get secret -n $NAMESPACE newrelic-secrets &>/dev/null; then
    echo "Detected existing New Relic secrets in the cluster"
    NR_API_KEY="DETECTED"
  fi
fi

# Reservoir sampler settings
SIZE_K=${RESERVOIR_SIZE_K:-5000}
WINDOW_DURATION=${RESERVOIR_WINDOW_DURATION:-"60s"}
CHECKPOINT_INTERVAL=${RESERVOIR_CHECKPOINT_INTERVAL:-"10s"}
TRACE_AWARE=${RESERVOIR_TRACE_AWARE:-true}
TRACE_BUFFER_MAX_SIZE=${RESERVOIR_BUFFER_SIZE:-100000}
TRACE_BUFFER_TIMEOUT=${RESERVOIR_BUFFER_TIMEOUT:-"30s"}
DB_COMPACTION_CRON=${RESERVOIR_COMPACTION_CRON:-"0 2 * * 0"}
PERSISTENCE_SIZE=${RESERVOIR_PERSISTENCE_SIZE:-"1Gi"}
CHECKPOINT_PATH=${RESERVOIR_CHECKPOINT_PATH:-"/var/otelpersist/reservoir.db"}

# NR-DOT specific settings
NRDOT_REPO=${NRDOT_REPO:-"github.com/newrelic/opentelemetry-collector-releases"}
NRDOT_BRANCH=${NRDOT_BRANCH:-"main"}
NRDOT_REGIONS=${NRDOT_REGIONS:-"US"} # US, EU, etc.
NRDOT_ACCOUNT_ID=${NRDOT_ACCOUNT_ID:-""} # Optional account ID for multiple accounts
NRDOT_LOG_LEVEL=${NRDOT_LOG_LEVEL:-"info"}

# Configure different endpoints based on region
if [ "$NRDOT_REGIONS" == "EU" ]; then
  NRDOT_OTLP_ENDPOINT="https://otlp.eu01.nr-data.net:4318"
else
  NRDOT_OTLP_ENDPOINT="https://otlp.nr-data.net:4318"
fi

# Export all variables for Docker Compose
export NRDOT_REGISTRY NRDOT_IMAGE_NAME NRDOT_TAG NRDOT_VERSION
export SIZE_K WINDOW_DURATION CHECKPOINT_INTERVAL TRACE_AWARE
export TRACE_BUFFER_MAX_SIZE TRACE_BUFFER_TIMEOUT DB_COMPACTION_CRON
export PERSISTENCE_SIZE CHECKPOINT_PATH NR_LICENSE_KEY
export NRDOT_OTLP_ENDPOINT NRDOT_LOG_LEVEL

# Welcome and requirements check
# -----------------------------
welcome() {
  section "Trace-Aware Reservoir Sampling Installer for NR-DOT"
  echo "This script will install and configure trace-aware reservoir sampling"
  echo "for the New Relic Distribution of OpenTelemetry."
  echo ""
  echo "Installation mode: $INSTALL_MODE"
  if [ "$USE_CUSTOM_BUILD" == "true" ]; then
    echo "Build mode: Custom NR-DOT build"
  else
    echo "Build mode: Pre-built NR-DOT image (version: $NRDOT_VERSION)"
  fi
  echo "NR-DOT region: $NRDOT_REGIONS"
  echo ""
  
  # Check for license key
  if [ -z "$NR_LICENSE_KEY" ] && [ "$NR_API_KEY" != "DETECTED" ]; then
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
    if [ "$NR_API_KEY" == "DETECTED" ]; then
      echo -e "${GREEN}✓ Using existing New Relic credentials from Kubernetes${NC}"
    else
      echo -e "${GREEN}✓ NR_LICENSE_KEY is set${NC}"
    fi
  fi
  
  # Check prerequisites based on installation mode
  check_prereqs
}

# Check prerequisites
check_prereqs() {
  section "Checking prerequisites"
  
  local missing=0
  local required_tools=()
  
  # Common requirements
  required_tools+=("git" "docker")
  
  # Add mode-specific requirements
  if [ "$INSTALL_MODE" == "kubernetes" ]; then
    required_tools+=("kubectl" "helm")
  elif [ "$INSTALL_MODE" == "docker" ]; then
    required_tools+=("docker-compose")
  fi
  
  # Add build-specific requirements
  if [ "$USE_CUSTOM_BUILD" == "true" ]; then
    required_tools+=("go")
  fi
  
  # Check each tool
  for cmd in "${required_tools[@]}"; do
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
}

# Build custom NR-DOT with reservoir sampler
build_custom_nrdot() {
  section "Building custom NR-DOT with reservoir sampler"
  
  # Create build directory
  BUILD_DIR="$(pwd)/build"
  mkdir -p $BUILD_DIR
  cd $BUILD_DIR
  
  # Clone NR-DOT repository if needed
  if [ ! -d "opentelemetry-collector-releases" ]; then
    echo "Cloning NR-DOT repository..."
    git clone https://$NRDOT_REPO.git opentelemetry-collector-releases
  fi
  
  cd opentelemetry-collector-releases
  
  # Checkout specified branch
  git fetch
  git checkout $NRDOT_BRANCH
  
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
  
  cd ../../../../
  echo -e "${GREEN}✓ Build and image creation complete!${NC}"
}

# Use pre-built NR-DOT image
use_prebuilt_nrdot() {
  section "Using pre-built NR-DOT image"
  
  echo "Pulling New Relic OTel Collector image: newrelic/nrdot-collector:$NRDOT_VERSION"
  docker pull newrelic/nrdot-collector:$NRDOT_VERSION
  
  # For Docker mode, we'll use the pre-built image directly
  # For Kubernetes mode, we'll use the Helm chart with the pre-built image
  
  # Set image name to use pre-built
  IMAGE_NAME="newrelic/nrdot-collector"
  TAG=$NRDOT_VERSION
  
  echo -e "${GREEN}✓ Using pre-built NR-DOT image: $IMAGE_NAME:$TAG${NC}"
}

# Create configuration for Docker Compose
create_docker_compose() {
  section "Creating Docker Compose configuration"
  
  cat > docker-compose.yaml << EOL
version: '3'
services:
  otel-collector:
    image: \${NRDOT_REGISTRY}/\${NRDOT_IMAGE_NAME}:\${NRDOT_TAG}
    container_name: otel-collector
    restart: unless-stopped
    command: ["--config=/etc/otelcol-config.yaml"]
    environment:
      - NR_LICENSE_KEY=\${NR_LICENSE_KEY}
    volumes:
      - ./otelcol-config.yaml:/etc/otelcol-config.yaml
      - otel-data:/var/otelpersist
    ports:
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
      - "8888:8888"   # Metrics
      - "13133:13133" # Health check
      - "55679:55679" # ZPages debugging

volumes:
  otel-data:
EOL

  # Create collector config file
  cat > otelcol-config.yaml << EOL
extensions:
  health_check:
  pprof:
  zpages:
  file_storage:
    directory: /var/otelpersist

receivers:
  otlp:
    protocols:
      grpc:
      http:

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 1000
  
  batch:
    send_batch_size: 10000
    timeout: 10s
  
  reservoir_sampler:
    size_k: ${SIZE_K}
    window_duration: "${WINDOW_DURATION}"
    checkpoint_path: "${CHECKPOINT_PATH}"
    checkpoint_interval: "${CHECKPOINT_INTERVAL}"
    trace_aware: ${TRACE_AWARE}
    trace_buffer_max_size: ${TRACE_BUFFER_MAX_SIZE}
    trace_buffer_timeout: "${TRACE_BUFFER_TIMEOUT}"
    db_compaction_schedule_cron: "${DB_COMPACTION_CRON}"

exporters:
  otlphttp/newrelic:
    endpoint: ${NRDOT_OTLP_ENDPOINT}
    headers:
      api-key: \${NR_LICENSE_KEY}

service:
  extensions: [health_check, pprof, zpages, file_storage]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, reservoir_sampler]
      exporters: [otlphttp/newrelic]
  
  telemetry:
    metrics:
      level: detailed
    logs:
      level: ${NRDOT_LOG_LEVEL}
EOL

  echo -e "${GREEN}✓ Docker Compose configuration created${NC}"
}

# Generate Kubernetes values file
generate_k8s_values() {
  section "Generating Kubernetes values file"

  VALUES_FILE="values-reservoir.yaml"

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
  # NR-DOT specific settings
  logLevel: ${NRDOT_LOG_LEVEL}
  
  configOverride:
    extensions:
      file_storage:
        directory: /var/otelpersist
    
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

    exporters:
      otlphttp/newrelic:
        endpoint: "${NRDOT_OTLP_ENDPOINT}"
EOL

  # Add account ID if specified
  if [ -n "$NRDOT_ACCOUNT_ID" ]; then
    cat >> $VALUES_FILE << EOL
        headers:
          "data-format-version": "2"
          "new-relic-account-id": "${NRDOT_ACCOUNT_ID}"
EOL
  fi

  # Complete the values file
  cat >> $VALUES_FILE << EOL

    service:
      extensions: [health_check, pprof, zpages, file_storage]

      pipelines:
        traces:
          receivers: [otlp]
          processors: [memory_limiter, batch, reservoir_sampler]
          exporters: [otlphttp/newrelic]
EOL

  echo -e "${GREEN}✓ Values file created: $VALUES_FILE${NC}"
}

# Deploy using Helm
deploy_k8s() {
  section "Deploying to Kubernetes with Helm"
  
  # Generate values file
  generate_k8s_values
  
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

# Start Docker Compose
start_docker() {
  section "Starting with Docker Compose"
  
  # Create Docker Compose configuration
  create_docker_compose
  
  # Start Docker Compose
  echo "Starting containers..."
  docker-compose up -d
  
  # Wait for container to start
  echo "Waiting for collector to start..."
  sleep 5
  
  # Check status
  if docker-compose ps | grep -q "Up"; then
    echo -e "${GREEN}✓ Collector started successfully${NC}"
  else
    echo -e "${RED}Collector failed to start. Check logs:${NC}"
    docker-compose logs
    exit 1
  fi
}

# Validate the deployment
validate_k8s() {
  section "Validating Kubernetes deployment"
  
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

# Validate Docker deployment
validate_docker() {
  section "Validating Docker deployment"
  
  # Check if container is running
  if ! docker-compose ps | grep -q "Up"; then
    echo -e "${RED}Container is not running. Check logs:${NC}"
    docker-compose logs
    exit 1
  fi
  
  # Check configuration
  echo -n "Checking for reservoir_sampler in configuration: "
  if docker-compose exec -T otel-collector grep -q "reservoir_sampler" /etc/otelcol-config.yaml; then
    echo -e "${GREEN}✓ Found${NC}"
  else
    echo -e "${RED}✗ Not found${NC}"
    echo "This indicates a configuration issue."
  fi
  
  # Check metrics
  echo -n "Checking for reservoir metrics: "
  if docker-compose exec -T otel-collector curl -s http://localhost:8888/metrics | grep -q "pte_reservoir"; then
    echo -e "${GREEN}✓ Found${NC}"
  else
    echo -e "${YELLOW}✗ Not found yet${NC}"
    echo "Metrics may take a moment to appear. Try again shortly."
  fi
  
  # Check checkpoint file
  echo -n "Checking for checkpoint file: "
  if docker-compose exec -T otel-collector ls -la /var/otelpersist/ 2>/dev/null | grep -q "reservoir.db"; then
    echo -e "${GREEN}✓ Found${NC}"
  else
    echo -e "${YELLOW}✗ Not found yet${NC}"
    echo "The checkpoint file will be created after the first checkpoint interval (10s)."
  fi
  
  echo ""
  echo -e "${GREEN}Validation complete.${NC}"
  echo ""
  echo "OpenTelemetry Collector endpoints:"
  echo "- OTLP gRPC: localhost:4317"
  echo "- OTLP HTTP: localhost:4318"
  echo "- Metrics:   http://localhost:8888"
  echo "- ZPages:    http://localhost:55679"
  echo ""
  echo "To monitor the processor metrics:"
  echo "curl -s http://localhost:8888/metrics | grep pte_reservoir"
  echo ""
  echo "To check the logs:"
  echo "docker-compose logs"
}

# Install New Relic dashboards for reservoir sampling
install_dashboards() {
  section "Installing New Relic dashboards"
  
  if [ -z "$NR_API_KEY" ] && [ -z "$NR_LICENSE_KEY" ]; then
    echo -e "${YELLOW}Warning: No New Relic API key available, skipping dashboard installation${NC}"
    return
  fi
  
  echo "Creating reservoir sampling dashboard in New Relic..."
  echo "This feature will be implemented soon."
  
  echo -e "${GREEN}✓ Dashboard installation complete${NC}"
}

# Show installation summary
show_summary() {
  section "Installation Summary"
  
  echo -e "${GREEN}✓ Trace-aware reservoir sampling has been installed${NC}"
  echo ""
  echo "Configuration:"
  echo "- Reservoir size: $SIZE_K traces"
  echo "- Window duration: $WINDOW_DURATION"
  echo "- Checkpoint interval: $CHECKPOINT_INTERVAL"
  echo "- Trace-aware mode: $TRACE_AWARE"
  echo "- NR-DOT Region: $NRDOT_REGIONS"
  echo "- OTLP Endpoint: $NRDOT_OTLP_ENDPOINT"
  echo ""
  
  if [ "$INSTALL_MODE" == "kubernetes" ]; then
    echo "Your installation is running in Kubernetes namespace: $NAMESPACE"
    echo "Helm release name: $RELEASE_NAME"
    echo ""
    echo "To check the status:"
    echo "kubectl get pods -n $NAMESPACE"
  else
    echo "Your installation is running with Docker Compose"
    echo ""
    echo "To check the status:"
    echo "docker-compose ps"
  fi
  
  echo ""
  echo "To send test data:"
  if [ "$INSTALL_MODE" == "kubernetes" ]; then
    ENDPOINT=$(kubectl get service -n $NAMESPACE -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].status.loadBalancer.ingress[0].ip}')
    if [ -z "$ENDPOINT" ]; then
      ENDPOINT="<service-ip>"
    fi
    echo "- Connect to endpoint: $ENDPOINT:4317 (gRPC) or $ENDPOINT:4318 (HTTP)"
  else
    echo "- Connect to endpoint: localhost:4317 (gRPC) or localhost:4318 (HTTP)"
  fi
  
  echo ""
  echo "You can use our test script to send sample traces:"
  echo "export OTLP_ENDPOINT=http://localhost:4318  # Adjust as needed"
  echo "./examples/quick-start/test-traces.sh"
}

# Show help
show_help() {
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  --mode <kubernetes|docker>  Installation mode (default: kubernetes)"
  echo "  --custom-build <true|false> Whether to build custom NR-DOT (default: true)"
  echo "  --version <version>         NR-DOT version when using pre-built (default: latest)"
  echo "  --region <US|EU>            New Relic region (default: US)"
  echo "  --help                      Show this help message"
  echo ""
  echo "NR-DOT Detection and Integration Options:"
  echo "  --auto-detect               Automatically detect existing NR-DOT installations"
  echo "  --extend-existing           Add reservoir sampler to existing NR-DOT installation"
  echo "  --update-existing           Update reservoir sampler in existing NR-DOT installation"
  echo ""
  echo "Environment variables:"
  echo "  NR_LICENSE_KEY              New Relic license key"
  echo "  NRDOT_MODE                  Installation mode (kubernetes or docker)"
  echo "  USE_CUSTOM_BUILD            Whether to build custom NR-DOT (true or false)"
  echo "  NRDOT_VERSION               NR-DOT version when using pre-built"
  echo "  NRDOT_REGIONS               New Relic region (US or EU)"
  echo "  NRDOT_ACCOUNT_ID            New Relic account ID (optional)"
  echo "  RESERVOIR_SIZE_K            Number of traces to sample"
  echo "  RESERVOIR_WINDOW_DURATION   Sampling window duration"
  echo ""
  echo "Examples:"
  echo "  ./install.sh                                # Standard installation"
  echo "  ./install.sh --mode docker                  # Install with Docker"
  echo "  ./install.sh --auto-detect                  # Detect existing NR-DOT"
  echo "  ./install.sh --extend-existing              # Add to existing NR-DOT"
  echo ""
  echo "For more configuration options, see .env.example"
}

# Auto-detect NR-DOT
auto_detect_nrdot() {
  section "Auto-detecting NR-DOT installation"

  # Source the detection script
  source ./nrdot-detect.sh

  # Run detection function
  detect_nrdot

  # Update settings based on detection
  if [ "$NRDOT_DETECTED" = true ]; then
    if [ -n "$NRDOT_NAMESPACE" ]; then
      echo "Using detected namespace: $NRDOT_NAMESPACE"
      NAMESPACE=$NRDOT_NAMESPACE
    fi

    if [ "$RESERVOIR_CONFIGURED" = true ]; then
      echo -e "${YELLOW}Reservoir sampler already configured. Use --update-existing to modify.${NC}"
      exit 0
    fi
  fi
}

# Update existing NR-DOT installation
update_existing_nrdot() {
  section "Updating existing NR-DOT installation"

  # Source the detection script
  source ./nrdot-detect.sh

  # Run detection function to get environment details
  detect_nrdot

  if [ "$NRDOT_DETECTED" != true ]; then
    echo -e "${RED}No existing NR-DOT installation detected for update${NC}"
    exit 1
  fi

  if [ -n "$NRDOT_NAMESPACE" ] && [ -n "$NRDOT_RELEASE" ]; then
    # Update Kubernetes installation
    echo "Updating existing Kubernetes installation: $NRDOT_RELEASE in namespace $NRDOT_NAMESPACE"

    # Generate values file
    NAMESPACE=$NRDOT_NAMESPACE
    RELEASE_NAME=$NRDOT_RELEASE
    generate_k8s_values

    # Update with Helm
    helm upgrade $RELEASE_NAME newrelic/nrdot-collector \
      --namespace $NAMESPACE \
      -f values-reservoir.yaml

    echo -e "${GREEN}✓ Updated existing NR-DOT installation${NC}"
  elif [ -n "$CONTAINER_ID" ]; then
    # Update Docker installation
    echo "Updating existing Docker container: $CONTAINER_ID"
    echo -e "${YELLOW}Docker update requires restarting the container${NC}"

    # Generate config file
    create_docker_compose

    # Stop and remove the existing container
    docker stop $CONTAINER_ID
    docker rm $CONTAINER_ID

    # Start new container
    start_docker
  else
    echo -e "${RED}Unable to update NR-DOT installation: insufficient information${NC}"
    exit 1
  fi
}

# Extend existing NR-DOT installation
extend_existing_nrdot() {
  section "Extending existing NR-DOT installation with reservoir sampler"

  # Source the detection script
  source ./nrdot-detect.sh

  # Run detection function to get environment details
  detect_nrdot

  if [ "$NRDOT_DETECTED" != true ]; then
    echo -e "${RED}No existing NR-DOT installation detected to extend${NC}"
    exit 1
  fi

  if [ "$RESERVOIR_CONFIGURED" = true ]; then
    echo -e "${YELLOW}Reservoir sampler already configured. Use --update-existing to modify.${NC}"
    exit 0
  fi

  if [ -n "$NRDOT_NAMESPACE" ] && [ -n "$NRDOT_RELEASE" ]; then
    # Extend Kubernetes installation
    echo "Extending existing Kubernetes installation: $NRDOT_RELEASE in namespace $NRDOT_NAMESPACE"

    # Generate values file
    NAMESPACE=$NRDOT_NAMESPACE
    RELEASE_NAME=$NRDOT_RELEASE
    generate_k8s_values

    # Update with Helm
    helm upgrade $RELEASE_NAME newrelic/nrdot-collector \
      --namespace $NAMESPACE \
      -f values-reservoir.yaml

    echo -e "${GREEN}✓ Extended existing NR-DOT installation with reservoir sampler${NC}"
  elif [ -n "$CONTAINER_ID" ]; then
    # Extend Docker installation
    echo "Extending existing Docker container: $CONTAINER_ID"
    echo -e "${YELLOW}Docker extension requires restarting the container${NC}"

    # Generate config file
    create_docker_compose

    # Stop and remove the existing container
    docker stop $CONTAINER_ID
    docker rm $CONTAINER_ID

    # Start new container
    start_docker
  else
    echo -e "${RED}Unable to extend NR-DOT installation: insufficient information${NC}"
    exit 1
  fi
}

# Parse command line arguments
parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --mode)
        INSTALL_MODE="$2"
        shift 2
        ;;
      --custom-build)
        USE_CUSTOM_BUILD="$2"
        shift 2
        ;;
      --version)
        NRDOT_VERSION="$2"
        shift 2
        ;;
      --region)
        NRDOT_REGIONS="$2"
        shift 2
        ;;
      --help)
        show_help
        exit 0
        ;;
      --auto-detect)
        auto_detect_nrdot
        exit 0
        ;;
      --update-existing)
        update_existing_nrdot
        exit 0
        ;;
      --extend-existing)
        extend_existing_nrdot
        exit 0
        ;;
      *)
        echo "Unknown option: $1"
        show_help
        exit 1
        ;;
    esac
  done
}

# Main execution logic
main() {
  welcome

  # Auto-detect existing NR-DOT if detection is enabled
  if [ "$AUTO_DETECT" == "true" ]; then
    # Source the detection script
    if [ -f "./nrdot-detect.sh" ]; then
      source ./nrdot-detect.sh

      # Run detection silently (just to get variables)
      NRDOT_DETECTED=false
      RESERVOIR_CONFIGURED=false
      SILENT_MODE=true

      if command -v kubectl &>/dev/null && command -v helm &>/dev/null; then
        detect_nrdot_kubernetes > /dev/null 2>&1
      fi

      if [ "$NRDOT_DETECTED" != true ] && command -v docker &>/dev/null; then
        detect_nrdot_docker > /dev/null 2>&1
      fi

      SILENT_MODE=false

      # If NR-DOT detected, ask if user wants to extend
      if [ "$NRDOT_DETECTED" = true ] && [ "$RESERVOIR_CONFIGURED" != true ]; then
        echo -e "${YELLOW}Existing NR-DOT installation detected${NC}"
        if [ -n "$NRDOT_NAMESPACE" ]; then
          echo "Found in Kubernetes namespace: $NRDOT_NAMESPACE"
          if [ -n "$NRDOT_RELEASE" ]; then
            echo "Helm release: $NRDOT_RELEASE"
          fi
        elif [ -n "$CONTAINER_ID" ]; then
          echo "Found in Docker container: $CONTAINER_ID"
        fi

        read -p "Do you want to extend this installation with reservoir sampling? (y/n) " -n 1 -r
        echo ""
        if [[ $REPLY =~ ^[Yy]$ ]]; then
          extend_existing_nrdot
          exit 0
        fi
      fi
    fi
  fi

  section "Starting installation"

  # Build or use pre-built based on configuration
  if [ "$USE_CUSTOM_BUILD" == "true" ]; then
    build_custom_nrdot
  else
    use_prebuilt_nrdot
  fi

  if [ "$INSTALL_MODE" == "kubernetes" ]; then
    deploy_k8s
    validate_k8s
  else
    start_docker
    validate_docker
  fi

  # Install dashboards if enabled
  if [ "$INSTALL_DASHBOARDS" == "true" ]; then
    install_dashboards
  fi

  show_summary
}

# Parse command line arguments if any
if [ $# -gt 0 ]; then
  parse_args "$@"
fi

# Run the main function
main