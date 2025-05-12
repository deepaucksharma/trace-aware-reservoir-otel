#!/bin/bash
set -e

# Go up to the project directory to find the .env file
PROJECT_DIR="$(cd "$(dirname "$(dirname "${BASH_SOURCE[0]}")")" && pwd)"
ENV_FILE="${PROJECT_DIR}/.env"
K8S_DIR="${PROJECT_DIR}/k8s"

# Load environment variables
echo "Looking for .env file at: ${ENV_FILE}"
if [ -f "${ENV_FILE}" ]; then
  echo "Loading environment from ${ENV_FILE}"
  # Load variables
  while IFS='=' read -r key value; do
    # Skip comments and empty lines
    [[ $key == \#* ]] && continue
    [[ -z "$key" ]] && continue
    # Remove quotes from the value
    value="${value//\"/}"
    # Export the variable
    export "$key=$value"
  done < "${ENV_FILE}"
else
  echo "No .env file found at ${ENV_FILE}. Using default values."
fi

# Set default values if not found in .env
NAMESPACE="${NAMESPACE:-monitoring-suite}"
DEPLOY_INFRA="${DEPLOY_INFRA:-true}"
DEPLOY_OTEL="${DEPLOY_OTEL:-true}"
DEPLOY_OTEL_PROCESSOR="${DEPLOY_OTEL_PROCESSOR:-true}"
NR_LICENSE_KEY="${NR_LICENSE_KEY:-your_license_key_here}"
NR_ENDPOINT="${NR_ENDPOINT:-https://otlp.nr-data.net:4318}"

# For OTLP, we use the license key as the API key in headers
export NR_API_KEY="${NR_LICENSE_KEY}"

# Check if kind cluster exists
if ! kind get clusters 2>/dev/null | grep -q "monitoring-cluster"; then
  echo "Kind cluster 'monitoring-cluster' is not running. Please start it first with:"
  echo "  ./kind/setup-kind.sh"
  exit 1
fi

# Create a temporary directory for templated manifests
TMP_DIR=$(mktemp -d)
echo "Creating templated Kubernetes manifests in ${TMP_DIR}..."

# Template function to replace {{ .VARIABLE }} patterns in files
template_file() {
  local input_file="$1"
  local output_file="$2"
  
  # Read the input file
  local template=$(cat "$input_file")
  
  # Create a temporary file
  cp "$input_file" "$output_file"
  
  # For each variable in the form {{ .VAR_NAME }}, replace it with its value
  for var in $(grep -o '{{ \.[A-Za-z0-9_]* }}' "$input_file" 2>/dev/null || echo ""); do
    # Extract variable name without {{ . and }}
    var_name=$(echo "$var" | sed 's/{{ \.\([A-Za-z0-9_]*\) }}/\1/')
    
    # Get variable value
    var_value="${!var_name}"
    
    # Replace in output file
    if [ -n "$var_value" ]; then
      sed -i.bak "s|$var|$var_value|g" "$output_file"
    else
      echo "WARNING: Variable $var_name not found or empty"
    fi
  done
  
  # Clean up backup files
  rm -f "${output_file}.bak" 2>/dev/null || true
}

# Create env-config ConfigMap
cat > "${TMP_DIR}/00-env-config.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-env-config
  namespace: ${NAMESPACE}
data:
  NAMESPACE: "${NAMESPACE}"
  NRIA_DEPLOYMENT_NAME: "${NRIA_DEPLOYMENT_NAME:-nria-deployment}"
  OTEL_DEPLOYMENT_NAME: "${OTEL_DEPLOYMENT_NAME:-otel-deployment}"
  OTEL_PROCESSOR_DEPLOYMENT_NAME: "${OTEL_PROCESSOR_DEPLOYMENT_NAME:-otel-processor-deployment}"
  NRIA_DISPLAY_NAME_SUFFIX: "${NRIA_DISPLAY_NAME_SUFFIX:-infra}"
  OTEL_DISPLAY_NAME_SUFFIX: "${OTEL_DISPLAY_NAME_SUFFIX:-nrdot}"
  OTEL_PROCESSOR_DISPLAY_NAME_SUFFIX: "${OTEL_PROCESSOR_DISPLAY_NAME_SUFFIX:-nrdot-plus}"
  OTEL_SERVICE_NAME: "${OTEL_SERVICE_NAME:-vm-monitoring-service}"
  OTEL_METRICS_COLLECTION_INTERVAL: "${OTEL_METRICS_COLLECTION_INTERVAL:-15s}"
  RESERVOIR_SIZE_K: "${RESERVOIR_SIZE_K:-1000}"
  WINDOW_DURATION: "${WINDOW_DURATION:-1m}"
  TRACE_AWARE: "${TRACE_AWARE:-true}"
  TRACE_BUFFER_MAX_SIZE: "${TRACE_BUFFER_MAX_SIZE:-10000}"
  TRACE_BUFFER_TIMEOUT: "${TRACE_BUFFER_TIMEOUT:-30s}"
  CHECKPOINT_PATH: "${CHECKPOINT_PATH:-/data/checkpoint}"
  CHECKPOINT_INTERVAL: "${CHECKPOINT_INTERVAL:-1m}"
  DB_COMPACTION_SCHEDULE: "${DB_COMPACTION_SCHEDULE:-0 0 * * *}"
  DB_COMPACTION_TARGET_SIZE: "${DB_COMPACTION_TARGET_SIZE:-104857600}"
  PVC_NAME: "${PVC_NAME:-checkpoint-pvc}"
  PVC_SIZE: "${PVC_SIZE:-1Gi}"
  OTLP_GRPC_PORT: "${OTLP_GRPC_PORT:-4317}"
  OTLP_HTTP_PORT: "${OTLP_HTTP_PORT:-4318}"
EOF

# Create New Relic secrets
if [ "${NR_LICENSE_KEY}" = "your_license_key_here" ]; then
  echo "WARNING: No valid New Relic license key provided in .env file"
  echo "For this demo, we'll use a placeholder value, but it won't send data to New Relic"
  NR_LICENSE_KEY="placeholder_license_key"
fi

# Base64 encode the license key
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS
  LICENSE_KEY_B64=$(echo -n "${NR_LICENSE_KEY}" | base64)
else
  # Linux
  LICENSE_KEY_B64=$(echo -n "${NR_LICENSE_KEY}" | base64 -w 0)
fi

# Create the secret
cat > "${TMP_DIR}/00-newrelic-secrets.yaml" << EOF
apiVersion: v1
kind: Secret
metadata:
  name: newrelic-secrets
  namespace: ${NAMESPACE}
type: Opaque
data:
  license-key: ${LICENSE_KEY_B64}
EOF

# Template all Kubernetes manifests
echo "Templating Kubernetes manifests..."
for file in "${K8S_DIR}"/*.yaml; do
  filename=$(basename "$file")
  output_file="${TMP_DIR}/${filename}"
  template_file "$file" "$output_file"
  echo "  Templated: ${filename}"
done

# Apply the manifests
echo "Applying Kubernetes manifests..."
kubectl apply -f "${TMP_DIR}/00-env-config.yaml"
kubectl apply -f "${TMP_DIR}/00-newrelic-secrets.yaml"

# Deploy components based on configuration
if [ "${DEPLOY_INFRA}" = "true" ]; then
  echo "Deploying New Relic Infrastructure Agent..."
  kubectl apply -f "${TMP_DIR}/10-nria-config.yaml"
  kubectl apply -f "${TMP_DIR}/11-nria-deployment.yaml"
fi

if [ "${DEPLOY_OTEL}" = "true" ]; then
  echo "Deploying OpenTelemetry Collector (standard)..."
  kubectl apply -f "${TMP_DIR}/20-otel-config.yaml"
  kubectl apply -f "${TMP_DIR}/21-otel-deployment.yaml"
fi

if [ "${DEPLOY_OTEL_PROCESSOR}" = "true" ]; then
  echo "Deploying OpenTelemetry Collector with custom processor..."
  kubectl apply -f "${TMP_DIR}/30-otel-processor-config.yaml"
  kubectl apply -f "${TMP_DIR}/31-otel-processor-deployment.yaml"
fi

# Clean up the temporary directory
rm -rf "${TMP_DIR}"

# Check deployment status
echo "Checking deployment status..."
kubectl get pods -n "${NAMESPACE}"

echo ""
echo "Monitoring stack deployed!"
echo "-------------------------"
echo "Access the services at:"
echo "  - localhost:${OTLP_GRPC_PORT:-4317} (OTLP gRPC)"
echo "  - localhost:${OTLP_HTTP_PORT:-4318} (OTLP HTTP)"
echo ""
echo "To check the status of the deployments:"
echo "  kubectl get pods -n ${NAMESPACE}"
echo ""
echo "To view logs from a specific pod:"
echo "  kubectl logs -n ${NAMESPACE} POD_NAME -f"
echo ""
echo "To clean up the deployment:"
echo "  ./kind/cleanup.sh"