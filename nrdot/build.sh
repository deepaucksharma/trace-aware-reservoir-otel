#!/bin/bash
# Build script for integrating trace-aware reservoir sampler with NRDOT

set -e

# Configuration variables - modify as needed
NRDOT_REPO="https://github.com/newrelic/opentelemetry-collector-releases.git"
RESERVOIR_MODULE="github.com/deepaksharma/trace-aware-reservoir-otel"
RESERVOIR_VERSION="v0.1.0"  # Update to your version
CUSTOM_IMAGE_NAME="nrdot-reservoir-sampler"
CUSTOM_IMAGE_TAG="v0.1.0"
REGISTRY="your-registry"  # Replace with your registry

# Color codes for output
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Building NRDOT with trace-aware reservoir sampler integration...${NC}"

# Check required tools
for cmd in git go docker; do
  if ! command -v $cmd &> /dev/null; then
    echo -e "${RED}Error: $cmd is not installed. Please install it first.${NC}"
    exit 1
  fi
done

# Create work directory
WORK_DIR="$(pwd)/build_tmp"
mkdir -p $WORK_DIR
cd $WORK_DIR

# Clone NRDOT repository if it doesn't exist
if [ ! -d "opentelemetry-collector-releases" ]; then
  echo "Cloning NRDOT repository..."
  git clone $NRDOT_REPO
fi

cd opentelemetry-collector-releases

# Create a new branch
BRANCH_NAME="feature/reservoir-sampler-$(date +%Y%m%d%H%M%S)"
git checkout -b $BRANCH_NAME

# Copy distribution.yaml from our project
echo "Updating distribution.yaml..."
DIST_FILE="../../../nrdot/distribution.yaml"
if [ -f "$DIST_FILE" ]; then
  cp $DIST_FILE distributions/nrdot-collector/
else
  echo -e "${RED}Error: distribution.yaml not found at $DIST_FILE${NC}"
  exit 1
fi

# Add dependency for the reservoir sampler
echo "Adding reservoir sampler dependency..."
go get $RESERVOIR_MODULE@$RESERVOIR_VERSION
go mod tidy

# Build the distribution
echo "Building custom NRDOT distribution..."
make dist

# Verify the build
if [ -f "./_dist/nrdot-reservoir/otelcol-nrdot-reservoir" ]; then
  echo -e "${GREEN}Build successful!${NC}"
  
  # Verify the reservoir sampler is included
  echo "Verifying reservoir sampler is included in the build:"
  ./_dist/nrdot-reservoir/otelcol-nrdot-reservoir components | grep reservoir_sampler
  
  if [ $? -ne 0 ]; then
    echo -e "${RED}Warning: reservoir_sampler not found in components list!${NC}"
  fi
else
  echo -e "${RED}Build failed! Output binary not found.${NC}"
  exit 1
fi

# Build Docker image
echo "Building Docker image..."
cd ./_dist/nrdot-reservoir/
docker build -t $REGISTRY/$CUSTOM_IMAGE_NAME:$CUSTOM_IMAGE_TAG .

echo -e "${GREEN}Docker image built successfully: $REGISTRY/$CUSTOM_IMAGE_NAME:$CUSTOM_IMAGE_TAG${NC}"
echo ""
echo "To push the image to your registry:"
echo -e "${YELLOW}docker push $REGISTRY/$CUSTOM_IMAGE_NAME:$CUSTOM_IMAGE_TAG${NC}"
echo ""
echo "To deploy with Helm using the provided values:"
echo -e "${YELLOW}helm upgrade --install nrdot newrelic/nrdot-collector \\"
echo "  --namespace observability \\"
echo "  --create-namespace \\"
echo "  --values values-reservoir.yaml \\"
echo "  --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY${NC}"
echo ""
echo -e "${GREEN}Integration complete!${NC}"

# Clean up (optional)
# rm -rf $WORK_DIR