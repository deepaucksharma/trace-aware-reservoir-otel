#!/bin/bash
# Deployment script for trace-aware-reservoir-otel on Kubernetes

set -e

# Configuration 
REGISTRY="ghcr.io"
ORG="deepaucksharma"  # GitHub username or organization
IMAGE_NAME="nrdot-reservoir"
VERSION="v0.1.0"
LICENSE_KEY="${NEW_RELIC_KEY:-your_license_key_here}"  # Use env var if available
CLUSTER_NAME="reservoir-demo"
NAMESPACE="otel"

# Check if Helm is installed
if ! command -v helm &> /dev/null; then
    echo "Helm is not installed. Please install Helm first."
    echo "Visit https://helm.sh/docs/intro/install/ for installation instructions."
    exit 1
fi

# Create namespace before Helm deployment
echo "Creating namespace ${NAMESPACE}..."
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

# Add Helm repository
echo "Adding New Relic Helm repository..."
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update

# Deploy with Helm
echo "Deploying to Kubernetes..."
helm upgrade --install otel-reservoir newrelic/nri-bundle \
  -n ${NAMESPACE} --create-namespace \
  -f values.reservoir.yaml \
  --set global.licenseKey="${LICENSE_KEY}" \
  --set global.cluster="${CLUSTER_NAME}" \
  --set image.repository="${REGISTRY}/${ORG}/${IMAGE_NAME}" \
  --set image.tag="${VERSION}"

# Verify deployment
echo "Verifying deployment..."
kubectl get pods -n ${NAMESPACE} -w

echo ""
echo "Deployment complete!"
echo ""
echo "To access the collector metrics:"
echo "kubectl port-forward -n ${NAMESPACE} svc/otel-collector 8888:8888"
echo ""
echo "To check logs:"
echo "kubectl logs -n ${NAMESPACE} deployment/otel-collector"
echo ""
echo "To troubleshoot common issues:"
echo "1. If pods show CrashLoopBackOff & Badger permission denied:"
echo "   - Ensure security context has fsGroup: 10001"
echo "2. If pods show imagePullBackOff:"
echo "   - Verify tag matches between chart and pushed image"
echo "3. If processor_reservoir_sampler doesn't appear in metrics:"
echo "   - Verify manifest patch was applied correctly"