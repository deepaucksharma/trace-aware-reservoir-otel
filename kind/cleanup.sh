#!/bin/bash
set -e

# Go up to the project directory to find the .env file
PROJECT_DIR="$(cd "$(dirname "$(dirname "${BASH_SOURCE[0]}")")" && pwd)"
ENV_FILE="${PROJECT_DIR}/.env"

# Load environment variables
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

# Set default namespace if not found in .env
NAMESPACE="${NAMESPACE:-monitoring-suite}"

# Check if kind cluster exists
if ! kind get clusters 2>/dev/null | grep -q "monitoring-cluster"; then
  echo "Kind cluster 'monitoring-cluster' does not exist."
  exit 1
fi

# Ask about what to clean up
echo "Select cleanup option:"
echo "1. Remove monitoring stack only (delete namespace)"
echo "2. Remove monitoring stack and delete Kind cluster"
echo "3. Cancel"
read -p "Enter option (1-3): " -n 1 -r
echo

case $REPLY in
  1)
    echo "Removing monitoring stack..."
    # Delete the monitoring namespace
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found
    echo "Monitoring stack removed."
    ;;
    
  2)
    echo "Removing monitoring stack and deleting Kind cluster..."
    # Delete the kind cluster (this will delete all resources in it)
    kind delete cluster --name monitoring-cluster
    echo "Kind cluster 'monitoring-cluster' deleted."
    ;;
    
  *)
    echo "Cleanup cancelled."
    exit 0
    ;;
esac

echo ""
echo "Cleanup complete!"