#!/bin/bash
# Deployment script for trace-aware-reservoir-otel with NR-DOT

set -e

# Configuration
export IMAGE=ghcr.io/yourusername/nrdot-reservoir:v0.1.0
export LICENSE_KEY="your_license_key_here"
export CLUSTER_NAME="reservoir-demo"

# Step 1: Build the NR-DOT distribution
echo "Building NR-DOT with reservoir sampler..."
cd opentelemetry-collector-releases
make dist

# Step 2: Verify the processor is included
echo "Verifying processor is included..."
_dist/nrdot/otelcol-nrdot components | grep reservoir_sampler

# Step 3: Build and push Docker image
echo "Building and pushing Docker image..."
docker build -t $IMAGE _dist/nrdot/
docker push $IMAGE

# Step 4: Set up Kind cluster
echo "Creating Kind cluster..."
cd ../trace-aware-reservoir-otel
kind create cluster --config kind-config.yaml

# Step 5: Deploy with Helm
echo "Deploying with Helm..."
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update
helm install otel-reservoir newrelic/nri-bundle \
  -f values.reservoir.yaml \
  --set global.licenseKey=$LICENSE_KEY \
  --set global.cluster=$CLUSTER_NAME

# Step 6: Verify deployment
echo "Verifying deployment..."
kubectl get pods -w

echo "Deployment complete! Use 'kubectl port-forward svc/otel-collector 8888:8888' to access metrics."
