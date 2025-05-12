package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/e2e"
)

// DurabilityTest tests the durability of the reservoir sampling state across restarts
func DurabilityTest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a temporary directory for checkpoint file
	tempDir, err := os.MkdirTemp("", "reservoir-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	checkpointPath := filepath.Join(tempDir, "checkpoint.db")

	// Load test configuration
	config := e2e.DefaultTestConfig()
	
	// Override test configuration for durability test
	config.ReservoirSize = 5000
	config.WindowDuration = "5m" // Longer window so it's still valid after restart
	config.CheckpointPath = checkpointPath
	config.CheckpointInterval = "5s" // Short interval for quick checkpointing
	
	// Generate collector configuration file
	configFile, err := config.GenerateConfigFile()
	if err != nil {
		t.Fatalf("Failed to generate collector config: %v", err)
	}

	// Create test framework
	framework, err := e2e.NewTestFramework(configFile)
	if err != nil {
		t.Fatalf("Failed to create test framework: %v", err)
	}

	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		t.Fatalf("Failed to start collector: %v", err)
	}

	// Allow collector to initialize
	time.Sleep(5 * time.Second)

	// ===== Phase 1: Send initial data =====
	
	log.Println("Phase 1: Sending initial data")
	
	// Send 10000 spans (1000 traces with 10 spans each)
	initialSpanCount := 10000
	spansPerTrace := 10
	
	// Generate and send traces
	traces := framework.GenerateTestSpans(initialSpanCount, spansPerTrace)
	if err := framework.SendTraces(ctx, traces); err != nil {
		t.Fatalf("Failed to send initial traces: %v", err)
	}
	
	// Wait for processing and checkpointing
	checkpointInterval, _ := time.ParseDuration(config.CheckpointInterval)
	waitTime := checkpointInterval * 2 // Wait for at least 2 checkpoint intervals
	log.Printf("Waiting %s for checkpointing...", waitTime)
	time.Sleep(waitTime)
	
	// Get stats before restart
	receivedBefore, sampledBefore, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		t.Fatalf("Failed to verify trace stats before restart: %v", err)
	}
	
	// ===== Phase 2: Restart collector =====
	
	log.Println("Phase 2: Restarting collector")
	
	// Stop the collector
	if err := framework.StopCollector(); err != nil {
		t.Fatalf("Failed to stop collector: %v", err)
	}
	
	// Verify checkpoint file exists
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		t.Fatalf("Checkpoint file not created: %s", checkpointPath)
	}
	
	// Wait a moment before restarting
	time.Sleep(2 * time.Second)
	
	// Restart the collector
	if err := framework.StartCollector(ctx); err != nil {
		t.Fatalf("Failed to restart collector: %v", err)
	}
	defer framework.StopCollector()
	
	// Wait for collector to initialize
	time.Sleep(5 * time.Second)
	
	// Get stats after restart
	receivedAfter, sampledAfter, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		t.Fatalf("Failed to verify trace stats after restart: %v", err)
	}
	
	// ===== Phase 3: Send more data =====
	
	log.Println("Phase 3: Sending additional data")
	
	// Send 5000 more spans
	additionalSpanCount := 5000
	
	// Generate and send traces
	traces = framework.GenerateTestSpans(additionalSpanCount, spansPerTrace)
	if err := framework.SendTraces(ctx, traces); err != nil {
		t.Fatalf("Failed to send additional traces: %v", err)
	}
	
	// Wait for processing
	time.Sleep(5 * time.Second)
	
	// Get final stats
	receivedFinal, sampledFinal, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		t.Fatalf("Failed to verify final trace stats: %v", err)
	}
	
	// ===== Validate results =====
	
	// In a real test, we would check specific metrics
	// For now, we'll make some reasonable assertions based on expected behavior
	
	// Calculate sampling rates
	samplingRateBefore := float64(sampledBefore) / float64(initialSpanCount) * 100
	samplingRateAfter := float64(sampledAfter) / float64(initialSpanCount) * 100
	samplingRateFinal := float64(sampledFinal) / float64(initialSpanCount+additionalSpanCount) * 100
	
	// Print test results
	fmt.Println("\nDurability Test Results:")
	fmt.Println("=======================")
	fmt.Printf("Configuration: Reservoir size: %d, Window: %s, Checkpoint interval: %s\n",
		config.ReservoirSize, config.WindowDuration, config.CheckpointInterval)
	fmt.Printf("Checkpoint path: %s\n", config.CheckpointPath)
	
	fmt.Println("\nPhase 1 - Initial Data:")
	fmt.Printf("  Spans sent: %d\n", initialSpanCount)
	fmt.Printf("  Spans received: %d\n", receivedBefore)
	fmt.Printf("  Spans sampled: %d (%.2f%%)\n", sampledBefore, samplingRateBefore)
	
	fmt.Println("\nPhase 2 - After Restart:")
	fmt.Printf("  Spans received: %d\n", receivedAfter)
	fmt.Printf("  Spans sampled: %d (%.2f%%)\n", sampledAfter, samplingRateAfter)
	fmt.Printf("  State preserved: %t\n", sampledAfter > 0)
	
	fmt.Println("\nPhase 3 - After Additional Data:")
	fmt.Printf("  Additional spans sent: %d\n", additionalSpanCount)
	fmt.Printf("  Total spans sent: %d\n", initialSpanCount+additionalSpanCount)
	fmt.Printf("  Spans received: %d\n", receivedFinal)
	fmt.Printf("  Spans sampled: %d (%.2f%%)\n", sampledFinal, samplingRateFinal)
	
	// Verify state was preserved across restart
	// We expect the number of sampled spans to remain non-zero after restart
	if sampledAfter == 0 {
		t.Errorf("No spans sampled after restart - state not preserved")
	}
	
	// Verify the collector continues to sample spans after restart
	if sampledFinal <= sampledAfter {
		t.Errorf("No additional spans sampled after restart - sampling not working")
	}
}