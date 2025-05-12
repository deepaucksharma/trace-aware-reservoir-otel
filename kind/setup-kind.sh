#!/bin/bash
set -e

# Go up to the project directory to find the .env file
PROJECT_DIR="$(cd "$(dirname "$(dirname "${BASH_SOURCE[0]}")")" && pwd)"
ENV_FILE="${PROJECT_DIR}/.env"

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
VM_CPU="${VM_CPU:-2}"
VM_MEMORY="${VM_MEMORY:-4}"  # Kind uses GB instead of MB
K8S_VERSION="${K8S_VERSION:-1.27.0}"
NAMESPACE="${NAMESPACE:-monitoring-suite}"

echo "Creating Kind cluster with the following configuration:"
echo "  Name: monitoring-cluster"
echo "  Kubernetes Version: ${K8S_VERSION}"

# Check if kind is already running
if kind get clusters 2>/dev/null | grep -q "monitoring-cluster"; then
  echo "Kind cluster 'monitoring-cluster' already exists. Deleting it first..."
  kind delete cluster --name monitoring-cluster
fi

# Create kind cluster config
cat > /tmp/kind-config.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: monitoring-cluster
nodes:
- role: control-plane
  image: kindest/node:v${K8S_VERSION}
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
  - containerPort: ${OTLP_GRPC_PORT:-4317}
    hostPort: ${OTLP_GRPC_PORT:-4317}
    protocol: TCP
  - containerPort: ${OTLP_HTTP_PORT:-4318}
    hostPort: ${OTLP_HTTP_PORT:-4318}
    protocol: TCP
EOF

# Create kind cluster
echo "Creating Kind cluster..."
kind create cluster --config=/tmp/kind-config.yaml

# Create the namespace
echo "Creating namespace ${NAMESPACE}..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Display success message
echo "Kind cluster 'monitoring-cluster' is now running with Kubernetes v${K8S_VERSION}"
echo "kubectl is configured to use the Kind cluster"
echo "You can deploy the monitoring stack by running:"
echo "  ./kind/deploy-monitoring.sh"