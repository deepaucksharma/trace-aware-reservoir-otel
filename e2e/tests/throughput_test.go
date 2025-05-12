package tests

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/e2e"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// ThroughputTest tests the maximum throughput the reservoir sampler can handle
func ThroughputTest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load test configuration
	config := e2e.DefaultTestConfig()
	
	// Override test configuration for throughput test
	config.ReservoirSize = 50000
	config.WindowDuration = "1m"
	config.InputRate = 100000 // High rate for stress testing
	config.TestDuration = "2m"
	config.Concurrency = 8
	
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
	defer framework.StopCollector()

	// Allow collector to initialize
	time.Sleep(5 * time.Second)

	// Parse test duration
	testDuration, err := time.ParseDuration(config.TestDuration)
	if err != nil {
		t.Fatalf("Invalid test duration: %v", err)
	}

	// Calculate total spans to send
	totalSpans := int(float64(config.InputRate) * testDuration.Seconds())
	spansPerWorker := totalSpans / config.Concurrency
	
	log.Printf("Throughput test: sending %d spans over %s at target rate of %d spans/second with %d workers",
		totalSpans, config.TestDuration, config.InputRate, config.Concurrency)

	// Track timing and actual throughput
	startTime := time.Now()
	
	// Use a wait group to track completion of all worker goroutines
	var wg sync.WaitGroup
	
	// Launch worker goroutines
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			// Calculate rate for this worker
			workerRate := config.InputRate / config.Concurrency
			
			// Generate spans for this worker
			workerSpans := spansPerWorker
			if workerID == config.Concurrency-1 {
				// Last worker gets any remainder
				workerSpans = totalSpans - (spansPerWorker * (config.Concurrency - 1))
			}
			
			// Generate traces
			traces := framework.GenerateTestSpans(workerSpans, config.SpansPerTrace)
			
			// Calculate time between sends to maintain desired rate
			sendInterval := time.Second / time.Duration(workerRate)
			
			// Send spans at the specified rate
			sentSpans := 0
			for sentSpans < workerSpans {
				batchStartTime := time.Now()
				
				// Determine batch size (how many spans to send at once)
				batchSize := 1000 // Default batch size
				if sentSpans+batchSize > workerSpans {
					batchSize = workerSpans - sentSpans
				}
				
				// Extract a subset of traces to send
				batchTraces := extractTraceBatch(traces, sentSpans, batchSize)
				
				// Send the batch
				if err := framework.SendTraces(ctx, batchTraces); err != nil {
					log.Printf("Worker %d: Failed to send traces: %v", workerID, err)
					return
				}
				
				sentSpans += batchSize
				
				// Sleep to maintain rate
				elapsed := time.Since(batchStartTime)
				targetInterval := time.Duration(float64(time.Second) * float64(batchSize) / float64(workerRate))
				
				if elapsed < targetInterval {
					time.Sleep(targetInterval - elapsed)
				}
			}
			
			log.Printf("Worker %d: Sent %d spans", workerID, sentSpans)
		}(i)
	}
	
	// Wait for all workers to complete
	wg.Wait()
	
	// Calculate actual throughput
	elapsedTime := time.Since(startTime)
	actualRate := float64(totalSpans) / elapsedTime.Seconds()
	
	log.Printf("All spans sent in %s (%.2f spans/second)", elapsedTime, actualRate)
	
	// Wait for processing to complete
	time.Sleep(10 * time.Second)
	
	// Get stats
	receivedSpans, sampledSpans, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		t.Fatalf("Failed to verify trace stats: %v", err)
	}
	
	// Print test results
	fmt.Println("\nThroughput Test Results:")
	fmt.Println("========================")
	fmt.Printf("Configuration: %d spans, window: %s, reservoir size: %d\n", 
		totalSpans, config.WindowDuration, config.ReservoirSize)
	fmt.Printf("Target Rate: %d spans/second\n", config.InputRate)
	fmt.Printf("Actual Rate: %.2f spans/second\n", actualRate)
	fmt.Printf("Test Duration: %s\n", elapsedTime)
	fmt.Printf("Total Spans Sent: %d\n", totalSpans)
	fmt.Printf("Spans Received: %d\n", receivedSpans)
	fmt.Printf("Spans Sampled: %d\n", sampledSpans)
	fmt.Printf("Sampling Rate: %.2f%%\n", float64(sampledSpans)/float64(totalSpans)*100)
	
	// Assert that the actual rate is at least 80% of the target rate
	if actualRate < float64(config.InputRate)*0.8 {
		t.Errorf("Throughput too low: %.2f spans/second (target: %d)", actualRate, config.InputRate)
	}
}

// extractTraceBatch extracts a subset of traces from the given traces
func extractTraceBatch(traces ptrace.Traces, offset, count int) ptrace.Traces {
	// In a real implementation, you would extract a subset of traces
	// For now, we'll just return the original traces
	// This is a placeholder
	return traces
}