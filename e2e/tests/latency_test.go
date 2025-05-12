package tests

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/e2e"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LatencyTest tests the latency impact of the reservoir sampler under different loads
func LatencyTest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load test configuration
	config := e2e.DefaultTestConfig()
	
	// Override test configuration for latency test
	config.ReservoirSize = 50000
	config.WindowDuration = "1m"
	
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

	// Define test rates (spans per second)
	rates := []int{1000, 5000, 10000, 20000}
	samplesPerRate := 1000 // Number of latency samples to collect for each rate
	
	// Prepare results
	type LatencyResult struct {
		Rate      int
		Min       time.Duration
		Max       time.Duration
		Mean      time.Duration
		P50       time.Duration
		P90       time.Duration
		P99       time.Duration
		StdDev    time.Duration
	}
	
	var results []LatencyResult
	
	// Create tracer for direct span generation
	tracer, tp, err := framework.CreateTraceClient(ctx, "latency-test")
	if err != nil {
		t.Fatalf("Failed to create trace client: %v", err)
	}
	defer tp.Shutdown(ctx)
	
	// Test each rate
	for _, rate := range rates {
		log.Printf("Testing latency at rate: %d spans/second", rate)
		
		// Calculate time between spans to achieve the desired rate
		sendInterval := time.Second / time.Duration(rate)
		
		// Collect latency samples
		var latencies []time.Duration
		
		for i := 0; i < samplesPerRate; i++ {
			startTime := time.Now()
			
			// Create and send a span
			_, span := tracer.Start(
				ctx, 
				fmt.Sprintf("test-span-%d-%d", rate, i),
				trace.WithAttributes(
					attribute.Int("test.rate", rate),
					attribute.Int("test.sample", i),
				),
			)
			span.End()
			
			// Measure latency
			latency := time.Since(startTime)
			latencies = append(latencies, latency)
			
			// Sleep to maintain rate
			sleepTime := sendInterval - latency
			if sleepTime > 0 {
				time.Sleep(sleepTime)
			}
			
			// Log progress periodically
			if i%100 == 0 && i > 0 {
				log.Printf("Rate %d spans/second: Collected %d/%d latency samples", 
					rate, i, samplesPerRate)
			}
		}
		
		// Calculate latency statistics
		latencyStats := calculateLatencyStats(latencies)
		results = append(results, latencyStats)
		
		log.Printf("Rate %d spans/second: Min=%s, Mean=%s, P99=%s, Max=%s",
			rate, latencyStats.Min, latencyStats.Mean, latencyStats.P99, latencyStats.Max)
		
		// Allow the system to stabilize between tests
		time.Sleep(5 * time.Second)
	}
	
	// Print test results
	fmt.Println("\nLatency Test Results:")
	fmt.Println("====================")
	fmt.Printf("Configuration: Reservoir size: %d, Window: %s\n",
		config.ReservoirSize, config.WindowDuration)
	fmt.Printf("Samples per rate: %d\n", samplesPerRate)
	
	fmt.Println("\nResults by Input Rate:")
	fmt.Println("Rate (spans/s) | Min (ms) | P50 (ms) | P90 (ms) | P99 (ms) | Max (ms) | StdDev (ms)")
	fmt.Println("-----------------------------------------------------------------------------")
	for _, result := range results {
		fmt.Printf("%-14d | %-8.2f | %-8.2f | %-8.2f | %-8.2f | %-8.2f | %-11.2f\n",
			result.Rate, 
			float64(result.Min.Microseconds())/1000,
			float64(result.P50.Microseconds())/1000,
			float64(result.P90.Microseconds())/1000,
			float64(result.P99.Microseconds())/1000,
			float64(result.Max.Microseconds())/1000,
			float64(result.StdDev.Microseconds())/1000)
	}
	
	// Assert that P99 latency at highest rate is still reasonable (< 50ms)
	highestRateResult := results[len(results)-1]
	if highestRateResult.P99 > 50*time.Millisecond {
		t.Errorf("P99 latency too high at rate %d spans/second: %s (expected < 50ms)",
			highestRateResult.Rate, highestRateResult.P99)
	}
}

// calculateLatencyStats calculates latency statistics from a slice of latency samples
func calculateLatencyStats(latencies []time.Duration) LatencyResult {
	if len(latencies) == 0 {
		return LatencyResult{}
	}
	
	// Sort latencies for percentile calculations
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})
	
	// Calculate min and max
	min := latencies[0]
	max := latencies[len(latencies)-1]
	
	// Calculate mean
	var sum time.Duration
	for _, latency := range latencies {
		sum += latency
	}
	mean := sum / time.Duration(len(latencies))
	
	// Calculate percentiles
	p50 := latencies[len(latencies)*50/100]
	p90 := latencies[len(latencies)*90/100]
	p99 := latencies[len(latencies)*99/100]
	
	// Calculate standard deviation
	var variance float64
	for _, latency := range latencies {
		diff := float64(latency - mean)
		variance += diff * diff
	}
	variance /= float64(len(latencies))
	stdDev := time.Duration(math.Sqrt(variance))
	
	// Find the rate based on the highest latency
	// This is a placeholder; in a real test, rate would be known
	rate := 1000 // Default to lowest rate
	
	return LatencyResult{
		Rate:   rate,
		Min:    min,
		Max:    max,
		Mean:   mean,
		P50:    p50,
		P90:    p90,
		P99:    p99,
		StdDev: stdDev,
	}
}