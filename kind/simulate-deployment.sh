#!/bin/bash

# This is a demonstration script that shows how the deployment would work
# without actually running kind commands

# Go up to the project directory to find the .env file
PROJECT_DIR="$(cd "$(dirname "$(dirname "${BASH_SOURCE[0]}")")" && pwd)"
ENV_FILE="${PROJECT_DIR}/.env"
K8S_DIR="${PROJECT_DIR}/k8s"

# Load environment variables
echo "Loading environment from ${ENV_FILE}"
while IFS='=' read -r key value; do
  # Skip comments and empty lines
  [[ $key == \#* ]] && continue
  [[ -z "$key" ]] && continue
  # Remove quotes from the value
  value="${value//\"/}"
  # Export the variable
  export "$key=$value"
  # Echo non-sensitive values for demonstration
  if [[ $key != *"KEY"* ]] && [[ $key != *"PASSWORD"* ]]; then
    echo "  $key = $value"
  else
    echo "  $key = ********** (hidden for security)"
  fi
done < "${ENV_FILE}"

echo ""
echo "=========================================="
echo "SIMULATION: Creating Kind Cluster"
echo "=========================================="
echo "Would create a Kind cluster 'monitoring-cluster' with:"
echo "  - Kubernetes version: ${K8S_VERSION:-1.27.0}"
echo "  - Exposing ports: ${OTLP_GRPC_PORT:-4317} (gRPC) and ${OTLP_HTTP_PORT:-4318} (HTTP)"
echo ""

echo "=========================================="
echo "SIMULATION: Applying Kubernetes Configuration"
echo "=========================================="
echo "Would create namespace: ${NAMESPACE:-monitoring-suite}"
echo ""

echo "SIMULATION: Creating environment ConfigMap and secrets for New Relic license key"
echo ""

echo "SIMULATION: Creating and applying templated Kubernetes manifests"
echo "  - Would template variables in all .yaml files in ${K8S_DIR}"
echo "  - Would apply the templates to the Kind cluster"
echo ""

echo "=========================================="
echo "SIMULATION: Deploying Components"
echo "=========================================="
if [ "${DEPLOY_INFRA:-true}" = "true" ]; then
  echo "1. New Relic Infrastructure Agent"
  echo "  - Deployment: ${NRIA_DEPLOYMENT_NAME:-nria-deployment}"
  echo "  - Display name: <hostname>_${NRIA_DISPLAY_NAME_SUFFIX:-infra}"
  echo "  - Configuration: ConfigMap with newrelic-infra.yml"
  echo ""
fi

if [ "${DEPLOY_OTEL:-true}" = "true" ]; then
  echo "2. OpenTelemetry Collector (Standard)"
  echo "  - Deployment: ${OTEL_DEPLOYMENT_NAME:-otel-deployment}"
  echo "  - Display name: <hostname>_${OTEL_DISPLAY_NAME_SUFFIX:-nrdot}"
  echo "  - Collection interval: ${OTEL_METRICS_COLLECTION_INTERVAL:-15s}"
  echo "  - Metrics pipeline with transform processor"
  echo ""
fi

if [ "${DEPLOY_OTEL_PROCESSOR:-true}" = "true" ]; then
  echo "3. OpenTelemetry Collector with Trace-Aware Reservoir Sampler"
  echo "  - Deployment: ${OTEL_PROCESSOR_DEPLOYMENT_NAME:-otel-processor-deployment}"
  echo "  - Display name: <hostname>_${OTEL_PROCESSOR_DISPLAY_NAME_SUFFIX:-nrdot-plus}"
  echo "  - Reservoir size: ${RESERVOIR_SIZE_K:-1000}"
  echo "  - Window duration: ${WINDOW_DURATION:-1m}"
  echo "  - Trace-aware: ${TRACE_AWARE:-true}"
  echo "  - Trace buffer max size: ${TRACE_BUFFER_MAX_SIZE:-10000}"
  echo "  - Trace buffer timeout: ${TRACE_BUFFER_TIMEOUT:-30s}"
  echo "  - Checkpoint path: ${CHECKPOINT_PATH:-/data/checkpoint}"
  echo "  - Checkpoint interval: ${CHECKPOINT_INTERVAL:-1m}"
  echo "  - Storage: PVC ${PVC_NAME:-checkpoint-pvc} (${PVC_SIZE:-1Gi})"
  echo ""
fi

echo "=========================================="
echo "SIMULATION: Access Points"
echo "=========================================="
echo "Services would be accessible at:"
echo "  - OTLP gRPC: localhost:${OTLP_GRPC_PORT:-4317}"
echo "  - OTLP HTTP: localhost:${OTLP_HTTP_PORT:-4318}"
echo ""

echo "=========================================="
echo "SIMULATION: New Relic Monitoring"
echo "=========================================="
echo "Data would be sent to New Relic:"
echo "  - Endpoint: ${NR_ENDPOINT:-https://otlp.nr-data.net:4318}"
echo "  - Using license key for authentication"
echo ""

echo "=========================================="
echo "SIMULATION: Kubernetes Resources"
echo "=========================================="
echo "A real deployment would include:"
echo "  - Namespace: ${NAMESPACE:-monitoring-suite}"
echo "  - ConfigMaps for configurations"
echo "  - Secret for New Relic License Key"
echo "  - Deployments for each component"
echo "  - Service for OTLP endpoints"
echo "  - PersistentVolumeClaim for checkpoint storage"
echo ""

echo "=========================================="
echo "SIMULATION: To Run This For Real"
echo "=========================================="
echo "When Docker Desktop is properly configured, you would run:"
echo "  1. ./kind/setup-kind.sh (create the Kind cluster)"
echo "  2. ./kind/deploy-monitoring.sh (deploy the monitoring stack)"
echo "  3. ./kind/cleanup.sh (clean up when done)"
echo ""
echo "This simulation used your actual environment variables from .env"
echo "to show how the deployment would be configured."