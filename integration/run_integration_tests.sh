#!/bin/bash
# Run all integration tests for the trace-aware reservoir sampler

set -e  # Exit on error

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}===== Running Integration Test Suite =====${NC}"

# Function to run a test suite and track results
run_test_suite() {
    local suite_name=$1
    local command=$2
    local start_time=$(date +%s)
    
    echo -e "\n${YELLOW}===== Running $suite_name =====${NC}"
    
    if $command; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        echo -e "${GREEN}✓ $suite_name completed successfully in ${duration}s${NC}"
        return 0
    else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        echo -e "${RED}✗ $suite_name failed after ${duration}s${NC}"
        return 1
    fi
}

# Track overall success/failure
SUCCESSES=0
FAILURES=0
SKIPPED=0

# Record start time
SUITE_START_TIME=$(date +%s)

# Create results directory
RESULTS_DIR="test-results"
mkdir -p $RESULTS_DIR

# Basic integration tests (short mode)
if run_test_suite "Basic Integration Tests (Short)" "go test -v -short -race ./... -timeout=3m 2>&1 | tee $RESULTS_DIR/integration-short.log"; then
    ((SUCCESSES++))
else
    ((FAILURES++))
fi

# Full integration tests
echo -e "\nDo you want to run the full integration tests? This may take 5-10 minutes. (y/n)"
read -r run_full
if [[ $run_full == "y" || $run_full == "Y" ]]; then
    if run_test_suite "Full Integration Tests" "go test -v -race ./... -timeout=10m 2>&1 | tee $RESULTS_DIR/integration-full.log"; then
        ((SUCCESSES++))
    else
        ((FAILURES++))
    fi
else
    echo -e "${YELLOW}⚠ Full Integration Tests skipped${NC}"
    ((SKIPPED++))
fi

# Performance tests
echo -e "\nDo you want to run the performance tests? This may take 5-15 minutes. (y/n)"
read -r run_perf
if [[ $run_perf == "y" || $run_perf == "Y" ]]; then
    if run_test_suite "Performance Tests" "go test -v ./performance_test.go -timeout=15m 2>&1 | tee $RESULTS_DIR/performance.log"; then
        ((SUCCESSES++))
    else
        ((FAILURES++))
    fi
else
    echo -e "${YELLOW}⚠ Performance Tests skipped${NC}"
    ((SKIPPED++))
fi

# Stress tests
echo -e "\nDo you want to run the stress tests? This may take 20-30 minutes. (y/n)"
read -r run_stress
if [[ $run_stress == "y" || $run_stress == "Y" ]]; then
    if run_test_suite "Stress Tests" "go test -v ./stress_test.go -timeout=30m 2>&1 | tee $RESULTS_DIR/stress.log"; then
        ((SUCCESSES++))
    else
        ((FAILURES++))
    fi
else
    echo -e "${YELLOW}⚠ Stress Tests skipped${NC}"
    ((SKIPPED++))
fi

# Calculate total duration
SUITE_END_TIME=$(date +%s)
SUITE_DURATION=$((SUITE_END_TIME - SUITE_START_TIME))
MINUTES=$((SUITE_DURATION / 60))
SECONDS=$((SUITE_DURATION % 60))

# Summary
echo -e "\n${GREEN}===== Test Suite Summary =====${NC}"
echo -e "Total duration: ${MINUTES}m ${SECONDS}s"
echo -e "Tests passed: ${GREEN}${SUCCESSES}${NC}"
echo -e "Tests failed: ${RED}${FAILURES}${NC}"
echo -e "Tests skipped: ${YELLOW}${SKIPPED}${NC}"
echo -e "Test logs saved to: ${RESULTS_DIR}/"

# Set exit code based on failures
if [ $FAILURES -gt 0 ]; then
    echo -e "\n${RED}Integration test suite failed with ${FAILURES} suite(s) reporting errors${NC}"
    exit 1
else
    echo -e "\n${GREEN}Integration test suite completed successfully!${NC}"
    exit 0
fi