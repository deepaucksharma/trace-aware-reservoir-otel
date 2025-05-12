#!/bin/bash
# Simple script to send test traces to the OpenTelemetry Collector
# Uses the OTLP HTTP endpoint to send sample traces

set -e

# Configuration
ENDPOINT=${OTLP_ENDPOINT:-"http://localhost:4318"}
NUM_TRACES=${NUM_TRACES:-10}
SPANS_PER_TRACE=${SPANS_PER_TRACE:-5}

# Color codes for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Sending ${NUM_TRACES} test traces to ${ENDPOINT}/v1/traces${NC}"
echo -e "${YELLOW}Each trace will have ${SPANS_PER_TRACE} spans${NC}"
echo ""

for (( i=1; i<=${NUM_TRACES}; i++ ))
do
  # Generate unique trace ID
  TRACE_ID=$(openssl rand -hex 16)
  
  # Generate trace payload with multiple spans
  PAYLOAD='{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"test-service"}}]},"scopeSpans":[{"scope":{"name":"test-scope"},"spans":['
  
  for (( j=1; j<=${SPANS_PER_TRACE}; j++ ))
  do
    # Generate unique span ID
    SPAN_ID=$(openssl rand -hex 8)
    
    # Add comma if not the first span
    if [ $j -gt 1 ]; then
      PAYLOAD="${PAYLOAD},"
    fi
    
    # Create span with parent-child relationship
    if [ $j -eq 1 ]; then
      # Root span
      PAYLOAD="${PAYLOAD}{\"traceId\":\"${TRACE_ID}\",\"spanId\":\"${SPAN_ID}\",\"name\":\"root-span\",\"kind\":1,\"startTimeUnixNano\":\"$(date +%s)000000000\",\"endTimeUnixNano\":\"$(date +%s)000000000\",\"attributes\":[{\"key\":\"trace.index\",\"value\":{\"intValue\":${i}}},{\"key\":\"span.index\",\"value\":{\"intValue\":${j}}}]}"
      PARENT_SPAN_ID=$SPAN_ID
    else
      # Child span
      PAYLOAD="${PAYLOAD}{\"traceId\":\"${TRACE_ID}\",\"spanId\":\"${SPAN_ID}\",\"parentSpanId\":\"${PARENT_SPAN_ID}\",\"name\":\"child-span-${j}\",\"kind\":1,\"startTimeUnixNano\":\"$(date +%s)000000000\",\"endTimeUnixNano\":\"$(date +%s)000000000\",\"attributes\":[{\"key\":\"trace.index\",\"value\":{\"intValue\":${i}}},{\"key\":\"span.index\",\"value\":{\"intValue\":${j}}}]}"
    fi
  done
  
  PAYLOAD="${PAYLOAD}]}]}]}"
  
  # Send the trace
  echo -e "Sending trace ${i}/${NUM_TRACES} with ID ${TRACE_ID}..."
  
  # Use curl to send the trace
  curl -s -X POST "${ENDPOINT}/v1/traces" \
    -H "Content-Type: application/json" \
    -d "${PAYLOAD}" \
    > /dev/null
    
  echo -e "${GREEN}✓ Sent${NC}"
  
  # Small delay between traces
  sleep 0.5
done

echo ""
echo -e "${GREEN}All test traces sent successfully${NC}"
echo ""
echo "To verify traces are being processed, check:"
echo "1. Metrics endpoint: curl -s http://localhost:8888/metrics | grep pte_reservoir"
echo "2. Checkpoint file: ls -la /var/otelpersist/"
echo "3. New Relic UI (if NR_LICENSE_KEY was configured)"