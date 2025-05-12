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

# Check if minikube is running
if ! minikube status &>/dev/null; then
  echo "Minikube is not running."
  exit 1
fi

# Ask about what to clean up
echo "Select cleanup option:"
echo "1. Remove monitoring stack only"
echo "2. Remove monitoring stack and stop Minikube"
echo "3. Remove monitoring stack, stop Minikube, and delete Minikube VM"
echo "4. Cancel"
read -p "Enter option (1-4): " -n 1 -r
echo

case $REPLY in
  1)
    echo "Removing monitoring stack..."
    # Delete the monitoring namespace
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found
    echo "Monitoring stack removed."
    ;;
    
  2)
    echo "Removing monitoring stack and stopping Minikube..."
    # Delete the monitoring namespace
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found
    # Stop Minikube
    minikube stop
    echo "Monitoring stack removed and Minikube stopped."
    ;;
    
  3)
    echo "Removing monitoring stack, stopping Minikube, and deleting Minikube VM..."
    # Delete the monitoring namespace
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found
    # Delete Minikube
    minikube delete
    echo "Monitoring stack removed and Minikube deleted."
    ;;
    
  *)
    echo "Cleanup cancelled."
    exit 0
    ;;
esac

echo ""
echo "Cleanup complete!"