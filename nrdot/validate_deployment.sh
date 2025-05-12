#!/bin/bash
# Validation script for trace-aware reservoir sampler deployment with NR-DOT

set -e

# Color codes for output
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Validating NR-DOT Reservoir Sampler deployment...${NC}"

# Configuration
NAMESPACE="observability"
SELECTOR="app.kubernetes.io/name=nrdot-collector"

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl is not installed. Please install it first.${NC}"
    exit 1
fi

# Check if the namespace exists
if ! kubectl get namespace $NAMESPACE &> /dev/null; then
    echo -e "${RED}Error: Namespace '$NAMESPACE' not found.${NC}"
    echo "Make sure the collector is deployed in the correct namespace."
    exit 1
fi

# Get pod name
echo "Looking for NR-DOT collector pod..."
POD=$(kubectl get pods -n $NAMESPACE -l $SELECTOR -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$POD" ]; then
    echo -e "${RED}Error: No pods found with selector '$SELECTOR' in namespace '$NAMESPACE'.${NC}"
    echo "Make sure the NR-DOT collector is deployed and running."
    exit 1
fi

echo -e "${GREEN}Found pod: $POD${NC}"

# Check pod status
echo "Checking pod status..."
POD_STATUS=$(kubectl get pod -n $NAMESPACE $POD -o jsonpath='{.status.phase}')

if [ "$POD_STATUS" != "Running" ]; then
    echo -e "${RED}Error: Pod is not running. Current status: $POD_STATUS${NC}"
    echo "Check pod events and logs:"
    echo "  kubectl describe pod -n $NAMESPACE $POD"
    echo "  kubectl logs -n $NAMESPACE $POD"
    exit 1
fi

echo -e "${GREEN}Pod is running.${NC}"

# Check that reservoir sampler is in the config
echo "Checking for reservoir_sampler in collector config..."
if ! kubectl exec -n $NAMESPACE $POD -- grep -q "reservoir_sampler" /etc/otelcol-config.yaml; then
    echo -e "${RED}Error: 'reservoir_sampler' not found in collector configuration.${NC}"
    echo "Check your Helm values and make sure the configOverride section is correct."
    exit 1
fi

echo -e "${GREEN}Reservoir sampler found in configuration.${NC}"

# Check for reservoir sampler metrics
echo "Checking for reservoir sampler metrics..."
METRICS=$(kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep -E "pte_reservoir|reservoir_sampler")

if [ -z "$METRICS" ]; then
    echo -e "${RED}Warning: No reservoir sampler metrics found.${NC}"
    echo "The processor may not be active or metrics are not being exported."
    echo "Check that the processor is correctly configured in the pipeline."
else
    echo -e "${GREEN}Found reservoir sampler metrics:${NC}"
    echo "$METRICS" | head -n 10
    if [ $(echo "$METRICS" | wc -l) -gt 10 ]; then
        echo "... ($(echo "$METRICS" | wc -l) total lines)"
    fi
fi

# Check for checkpoint file
echo "Checking for checkpoint file..."
if ! kubectl exec -n $NAMESPACE $POD -- ls -l /var/otelpersist/ | grep -q "reservoir.db"; then
    echo -e "${YELLOW}Warning: Checkpoint file 'reservoir.db' not found.${NC}"
    echo "This is normal if the processor just started and hasn't reached its first checkpoint interval."
    echo "Check persistence mount and permissions:"
    kubectl exec -n $NAMESPACE $POD -- ls -la /var/otelpersist/
else
    echo -e "${GREEN}Checkpoint file found.${NC}"
    CHECKPOINT_SIZE=$(kubectl exec -n $NAMESPACE $POD -- du -h /var/otelpersist/reservoir.db | awk '{print $1}')
    echo "Checkpoint file size: $CHECKPOINT_SIZE"
fi

# Check for checkpoint errors
echo "Checking for checkpoint errors..."
CHECKPOINT_ERRORS=$(kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep "pte_reservoir_checkpoint_errors_total")
if [[ "$CHECKPOINT_ERRORS" == *"pte_reservoir_checkpoint_errors_total 0"* ]] || [ -z "$CHECKPOINT_ERRORS" ]; then
    echo -e "${GREEN}No checkpoint errors detected.${NC}"
else
    echo -e "${RED}Warning: Checkpoint errors detected:${NC}"
    echo "$CHECKPOINT_ERRORS"
    echo "Check PV mount and permissions. The collector may not have write access."
fi

# Check send/receive metrics
echo "Checking trace processing metrics..."
SPANS_RECEIVED=$(kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep "otelcol_receiver_accepted_spans_total")
SPANS_EXPORTED=$(kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep "otelcol_exporter_sent_spans_total")

echo "Spans received: $SPANS_RECEIVED"
echo "Spans exported: $SPANS_EXPORTED"

# Perform a simple trace generation test if otc tool is available
if command -v otel-cli &> /dev/null; then
    echo "Testing trace generation with otel-cli..."
    # Forward the OTLP port
    kubectl port-forward -n $NAMESPACE svc/nr-otel 4317:4317 &
    PF_PID=$!
    
    # Wait for port-forward to establish
    sleep 2
    
    # Generate a test trace
    otel-cli spans --endpoint localhost:4317 --insecure \
      --service test-service \
      --name test-span \
      --count 10
    
    # Stop port-forwarding
    kill $PF_PID
    
    # Check metrics again to see if spans were received
    echo "Checking if test spans were received..."
    sleep 2
    NEW_SPANS_RECEIVED=$(kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep "otelcol_receiver_accepted_spans_total")
    echo "Updated spans received: $NEW_SPANS_RECEIVED"
fi

echo -e "${GREEN}Validation complete.${NC}"
echo ""
echo "For more detailed monitoring, check these metrics:"
echo "- pte_reservoir_traces_in_reservoir_count - Number of traces in reservoir"
echo "- pte_reservoir_checkpoint_age_seconds - Age of last checkpoint"
echo "- pte_reservoir_db_size_bytes - Size of checkpoint database"
echo "- pte_reservoir_lru_evictions_total - Number of trace buffer evictions"
echo "- pte_reservoir_sampled_spans_total - Number of spans added to reservoir"
echo ""
echo "To see all reservoir metrics:"
echo "kubectl exec -n $NAMESPACE $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir"