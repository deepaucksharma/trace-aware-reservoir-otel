#!/bin/bash
# Updated deployment script for trace-aware-reservoir-otel with NR-DOT

set -e

# Configuration 
# UPDATE THESE VALUES
export REGISTRY="ghcr.io"
export ORG="your-org"  # Replace with your GitHub username or organization
export IMAGE_NAME="nrdot-reservoir"
export VERSION="v0.1.0"
export LICENSE_KEY="your_license_key_here"  # Replace with your actual New Relic license key
export CLUSTER_NAME="reservoir-demo"

export IMAGE="${REGISTRY}/${ORG}/${IMAGE_NAME}:${VERSION}"

# Step 1: Deploy to Docker Desktop Kubernetes
echo "Deploying to Kubernetes..."

# Create namespace
kubectl create namespace otel --dry-run=client -o yaml | kubectl apply -f -

# Add Helm repository
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update

# Deploy with Helm
helm upgrade --install otel-reservoir newrelic/nri-bundle \
  -f values.reservoir.yaml \
  --set global.licenseKey=$LICENSE_KEY \
  --set global.cluster=$CLUSTER_NAME \
  --set image.repository=${REGISTRY}/${ORG}/${IMAGE_NAME} \
  --set image.tag=${VERSION} \
  --namespace otel

# Step 2: Verify deployment
echo "Verifying deployment..."
kubectl get pods -n otel -w

echo "Deployment complete!"
echo ""
echo "To access the collector metrics:"
echo "kubectl port-forward -n otel svc/otel-collector 8888:8888"
echo ""
echo "To check logs:"
echo "kubectl logs -n otel deployment/otel-collector"
