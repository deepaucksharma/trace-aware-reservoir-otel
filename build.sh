#!/bin/bash
# Simplified build script for trace-aware-reservoir-otel using the multistage Dockerfile

set -e

# Configuration
REGISTRY="ghcr.io"
ORG="deepaucksharma"  # GitHub username or organization
IMAGE_NAME="nrdot-reservoir"
VERSION="v0.1.0"
IMAGE="${REGISTRY}/${ORG}/${IMAGE_NAME}:${VERSION}"

echo "Building ${IMAGE} using multistage Dockerfile..."

# Build the Docker image
docker build -t ${IMAGE} \
  --build-arg NRDOT_VERSION=v0.91.0 \
  --build-arg RS_VERSION=${VERSION} \
  -f Dockerfile.multistage .

echo "Build complete!"
echo ""
echo "To push the image:"
echo "  docker push ${IMAGE}"
echo ""
echo "To deploy to Kubernetes:"
echo "  ./deploy-k8s.sh"