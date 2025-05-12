#!/bin/bash
# Integration testing script for the reservoir sampler

set -e

echo "=== Trace-Aware Reservoir Sampler Integration Test ==="

# Step 1: Check if the collector is running
echo "Checking collector status..."
kubectl get pods -n otel | grep otel-collector

# Step 2: Port-forward the metrics endpoint
echo "Setting up port-forward to metrics endpoint..."
kubectl port-forward -n otel svc/otel-collector 8888:8888 &
PF_PID=$!

# Give it a moment to establish the connection
sleep 3

# Step 3: Check if the processor is registered
echo "Checking if reservoir_sampler is registered..."
curl -s http://localhost:8888/metrics | grep "processor_reservoir_sampler"

# Step 4: Check reservoir metrics
echo "Checking reservoir metrics..."
curl -s http://localhost:8888/metrics | grep "reservoir_sampler"

# Step 5: Check logs for window rollovers
echo "Checking logs for window rollovers..."
kubectl logs -n otel deployment/otel-collector | grep "Started new sampling window"

# Cleanup
kill $PF_PID
wait $PF_PID 2>/dev/null || true

echo "=== Test Complete ==="
echo "For detailed testing:"
echo "1. Use 'kubectl port-forward -n otel svc/otel-collector 8888:8888' to access metrics"
echo "2. Send sample traces to 'localhost:4317' using the OTLP protocol"
echo "3. Check 'curl http://localhost:8888/metrics | grep reservoir_sampler' for detailed metrics"
