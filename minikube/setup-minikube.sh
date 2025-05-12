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
VM_MEMORY="${VM_MEMORY:-4096}" # Minikube uses MB instead of GB
K8S_VERSION="${K8S_VERSION:-1.27.0}"
NAMESPACE="${NAMESPACE:-monitoring-suite}"

echo "Starting Minikube with the following configuration:"
echo "  CPU: ${VM_CPU}"
echo "  Memory: ${VM_MEMORY}MB"
echo "  Kubernetes Version: ${K8S_VERSION}"

# Check if minikube is already running
if minikube status &>/dev/null; then
  echo "Minikube is already running. Stopping it first..."
  minikube stop
fi

# Start minikube with the specified configuration
echo "Starting Minikube..."
minikube start --cpus "${VM_CPU}" --memory "${VM_MEMORY}" --kubernetes-version "v${K8S_VERSION}" --driver=docker

# Create the namespace
echo "Creating namespace ${NAMESPACE}..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Display success message
echo "Minikube is now running with Kubernetes v${K8S_VERSION}"
echo "kubectl is configured to use the Minikube cluster"
echo "You can deploy the monitoring stack by running:"
echo "  ./minikube/deploy-monitoring.sh"