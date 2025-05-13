#!/bin/bash
# Integration test script for trace-aware-reservoir-otel

set -e

NAMESPACE="otel"

echo "Running integration tests for trace-aware-reservoir-otel..."

# Test 1: Check if the collector pod is running
echo "Test 1: Checking if the collector pod is running..."
PODS=$(kubectl get pods -n ${NAMESPACE} -l app.kubernetes.io/component=opentelemetry-collector -o jsonpath='{.items[*].status.phase}')
if [[ "$PODS" == *"Running"* ]]; then
  echo "✅ Collector pod is running"
else
  echo "❌ Collector pod is not running"
  exit 1
fi

# Test 2: Check if the reservoir_sampler processor is registered
echo "Test 2: Checking if the reservoir_sampler processor is registered..."
# Port-forward in the background
kubectl port-forward -n ${NAMESPACE} svc/otel-collector 8888:8888 &
PF_PID=$!
sleep 5  # Give it time to establish the port-forwarding

# Check metrics
METRICS=$(curl -s http://localhost:8888/metrics | grep reservoir_sampler)
kill $PF_PID  # Kill the port-forwarding process

if [[ -n "$METRICS" ]]; then
  echo "✅ reservoir_sampler processor is registered"
else
  echo "❌ reservoir_sampler processor is not registered"
  exit 1
fi

# Test 3: Check if the Badger database is accessible
echo "Test 3: Checking if the Badger database is accessible..."
DB_CHECK=$(kubectl exec -n ${NAMESPACE} -l app.kubernetes.io/component=opentelemetry-collector -- ls -la /var/otelpersist/badger 2>/dev/null || echo "Failed")
if [[ "$DB_CHECK" != "Failed" ]]; then
  echo "✅ Badger database is accessible"
else
  echo "❌ Badger database is not accessible"
  exit 1
fi

echo "All integration tests passed! ✅"