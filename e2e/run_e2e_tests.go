package e2e

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "", "Path to test configuration file")
	testName   = flag.String("test", "all", "Test to run (all, throughput, latency, durability, resource_usage, trace_preservation)")
)

func Run() {
	flag.Parse()

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling to gracefully shut down
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		sig := <-sigs
		log.Printf("Received signal %s, shutting down", sig)
		cancel()
	}()

	// Load or create test configuration
	var config *TestConfig
	var err error
	
	if *configPath != "" {
		config, err = LoadTestConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load test configuration: %v", err)
		}
	} else {
		config = DefaultTestConfig()
	}

	// Ensure checkpoint directory exists
	checkpointDir := filepath.Dir(config.CheckpointPath)
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		log.Fatalf("Failed to create checkpoint directory: %v", err)
	}

	// Generate collector configuration file
	configFile, err := config.GenerateConfigFile()
	if err != nil {
		log.Fatalf("Failed to generate collector config file: %v", err)
	}
	log.Printf("Generated collector config file: %s", configFile)

	// Create test framework
	framework, err := NewTestFramework(configFile)
	if err != nil {
		log.Fatalf("Failed to create test framework: %v", err)
	}

	// Run the requested test
	switch *testName {
	case "all":
		runAllTests(ctx, framework, config)
	case "throughput":
		runThroughputTest(ctx, framework, config)
	case "latency":
		runLatencyTest(ctx, framework, config)
	case "durability":
		runDurabilityTest(ctx, framework, config)
	case "resource_usage":
		runResourceUsageTest(ctx, framework, config)
	case "trace_preservation":
		runTracePreservationTest(ctx, framework, config)
	default:
		log.Fatalf("Unknown test: %s", *testName)
	}
}

func runAllTests(ctx context.Context, framework *TestFramework, config *TestConfig) {
	log.Println("Running all tests")
	
	// Run tests sequentially
	runThroughputTest(ctx, framework, config)
	runLatencyTest(ctx, framework, config)
	runDurabilityTest(ctx, framework, config)
	runResourceUsageTest(ctx, framework, config)
	runTracePreservationTest(ctx, framework, config)
}

func runThroughputTest(ctx context.Context, framework *TestFramework, config *TestConfig) {
	log.Println("Running throughput test")
	
	testDuration, err := time.ParseDuration(config.TestDuration)
	if err != nil {
		log.Fatalf("Invalid test duration: %v", err)
	}
	
	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}
	defer framework.StopCollector()
	
	// Allow collector to initialize
	time.Sleep(2 * time.Second)
	
	// Calculate total spans to send
	totalSpans := int(float64(config.InputRate) * testDuration.Seconds())
	log.Printf("Sending %d spans over %s at %d spans/second", totalSpans, config.TestDuration, config.InputRate)
	
	// Generate spans
	traces := framework.GenerateTestSpans(totalSpans, config.SpansPerTrace)
	
	// Send spans
	startTime := time.Now()
	err = framework.SendTraces(ctx, traces)
	if err != nil {
		log.Fatalf("Failed to send traces: %v", err)
	}
	elapsedTime := time.Since(startTime)
	
	// Calculate achieved rate
	actualRate := float64(totalSpans) / elapsedTime.Seconds()
	log.Printf("Sent %d spans in %s (%.2f spans/second)", totalSpans, elapsedTime, actualRate)
	
	// Wait for processing to complete
	time.Sleep(5 * time.Second)
	
	// Get stats
	receivedSpans, sampledSpans, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		log.Fatalf("Failed to verify trace stats: %v", err)
	}
	
	log.Printf("Throughput test results:")
	log.Printf("  Sent spans: %d", totalSpans)
	log.Printf("  Received spans: %d", receivedSpans)
	log.Printf("  Sampled spans: %d", sampledSpans)
	log.Printf("  Sampling rate: %.2f%%", float64(sampledSpans)/float64(totalSpans)*100)
	log.Printf("  Achieved input rate: %.2f spans/second", actualRate)
}

func runLatencyTest(ctx context.Context, framework *TestFramework, config *TestConfig) {
	log.Println("Running latency test")
	
	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}
	defer framework.StopCollector()
	
	// Allow collector to initialize
	time.Sleep(2 * time.Second)
	
	// Create tracer for direct span generation
	tracer, tp, err := framework.CreateTraceClient(ctx, "latency-test")
	if err != nil {
		log.Fatalf("Failed to create trace client: %v", err)
	}
	defer tp.Shutdown(ctx)
	
	// Send spans and measure latency
	testDuration, err := time.ParseDuration(config.TestDuration)
	if err != nil {
		log.Fatalf("Invalid test duration: %v", err)
	}
	
	// Set up test end time
	endTime := time.Now().Add(testDuration)
	
	// Calculate pause between span sends to achieve target rate
	sendInterval := time.Second / time.Duration(config.InputRate)
	
	var latencies []time.Duration
	totalSpans := 0
	
	log.Printf("Running latency test for %s at %d spans/second", testDuration, config.InputRate)
	
	for time.Now().Before(endTime) {
		startTime := time.Now()
		
		_, span := tracer.Start(ctx, "test-span")
		span.End()
		
		totalSpans++
		latencies = append(latencies, time.Since(startTime))
		
		// Sleep to maintain rate
		sleepTime := sendInterval - time.Since(startTime)
		if sleepTime > 0 {
			time.Sleep(sleepTime)
		}
	}
	
	// Calculate latency statistics
	var totalLatency time.Duration
	maxLatency := time.Duration(0)
	
	for _, latency := range latencies {
		totalLatency += latency
		if latency > maxLatency {
			maxLatency = latency
		}
	}
	
	avgLatency := totalLatency / time.Duration(len(latencies))
	
	log.Printf("Latency test results:")
	log.Printf("  Total spans: %d", totalSpans)
	log.Printf("  Average latency: %s", avgLatency)
	log.Printf("  Maximum latency: %s", maxLatency)
}

func runDurabilityTest(ctx context.Context, framework *TestFramework, config *TestConfig) {
	log.Println("Running durability test")
	
	// Create a logger
	logger, _ := zap.NewDevelopment()
	
	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}
	
	// Send initial batch of spans
	const initialBatchSize = 10000
	logger.Info("Sending initial batch of spans", zap.Int("count", initialBatchSize))
	
	traces := framework.GenerateTestSpans(initialBatchSize, config.SpansPerTrace)
	if err := framework.SendTraces(ctx, traces); err != nil {
		log.Fatalf("Failed to send initial traces: %v", err)
	}
	
	// Wait for processing and checkpointing
	checkpointInterval, err := time.ParseDuration(config.CheckpointInterval)
	if err != nil {
		log.Fatalf("Invalid checkpoint interval: %v", err)
	}
	
	// Wait a bit longer than the checkpoint interval
	waitTime := checkpointInterval + (5 * time.Second)
	logger.Info("Waiting for checkpointing", zap.Duration("wait_time", waitTime))
	time.Sleep(waitTime)
	
	// Get stats before restart
	beforeReceivedSpans, beforeSampledSpans, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		log.Fatalf("Failed to verify trace stats before restart: %v", err)
	}
	
	// Stop the collector
	logger.Info("Stopping collector for restart test")
	framework.StopCollector()
	
	// Wait a bit before restarting
	time.Sleep(2 * time.Second)
	
	// Restart the collector
	logger.Info("Restarting collector")
	if err := framework.StartCollector(ctx); err != nil {
		log.Fatalf("Failed to restart collector: %v", err)
	}
	defer framework.StopCollector()
	
	// Wait for collector to initialize
	time.Sleep(5 * time.Second)
	
	// Get stats after restart
	afterReceivedSpans, afterSampledSpans, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		log.Fatalf("Failed to verify trace stats after restart: %v", err)
	}
	
	// Send another batch of spans
	const secondBatchSize = 5000
	logger.Info("Sending second batch of spans", zap.Int("count", secondBatchSize))
	
	traces = framework.GenerateTestSpans(secondBatchSize, config.SpansPerTrace)
	if err := framework.SendTraces(ctx, traces); err != nil {
		log.Fatalf("Failed to send second batch of traces: %v", err)
	}
	
	// Wait for processing
	time.Sleep(5 * time.Second)
	
	// Get final stats
	finalReceivedSpans, finalSampledSpans, err := framework.VerifyTraceStats(ctx)
	if err != nil {
		log.Fatalf("Failed to verify final trace stats: %v", err)
	}
	
	log.Printf("Durability test results:")
	log.Printf("  Initial batch size: %d", initialBatchSize)
	log.Printf("  Before restart - received: %d, sampled: %d", beforeReceivedSpans, beforeSampledSpans)
	log.Printf("  After restart - received: %d, sampled: %d", afterReceivedSpans, afterSampledSpans)
	log.Printf("  Second batch size: %d", secondBatchSize)
	log.Printf("  Final - received: %d, sampled: %d", finalReceivedSpans, finalSampledSpans)
}

func runResourceUsageTest(ctx context.Context, framework *TestFramework, config *TestConfig) {
	log.Println("Running resource usage test")
	
	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}
	defer framework.StopCollector()
	
	// Allow collector to initialize
	time.Sleep(2 * time.Second)
	
	// Send spans at increasing rates to measure resource usage
	rates := []int{1000, 5000, 10000, 20000, 50000}
	spansPerBatch := 1000
	
	for _, rate := range rates {
		log.Printf("Testing at rate: %d spans/second", rate)
		
		// Send spans for 30 seconds at this rate
		startTime := time.Now()
		endTime := startTime.Add(30 * time.Second)
		
		sentSpans := 0
		
		for time.Now().Before(endTime) {
			batchStartTime := time.Now()
			
			// Send a batch
			traces := framework.GenerateTestSpans(spansPerBatch, config.SpansPerTrace)
			if err := framework.SendTraces(ctx, traces); err != nil {
				log.Fatalf("Failed to send traces: %v", err)
			}
			
			sentSpans += spansPerBatch
			
			// Calculate sleep time to maintain rate
			elapsed := time.Since(batchStartTime)
			targetInterval := time.Duration(float64(time.Second) * float64(spansPerBatch) / float64(rate))
			
			if elapsed < targetInterval {
				time.Sleep(targetInterval - elapsed)
			}
		}
		
		log.Printf("  Sent %d spans at target rate %d spans/second", sentSpans, rate)
		
		// Wait for processing to complete
		time.Sleep(5 * time.Second)
	}
}

func runTracePreservationTest(ctx context.Context, framework *TestFramework, config *TestConfig) {
	log.Println("Running trace preservation test")
	
	// For this test, ensure trace-aware mode is enabled
	originalTraceAware := config.TraceAware
	config.TraceAware = true
	
	// Regenerate config file with trace-aware mode enabled
	configFile, err := config.GenerateConfigFile()
	if err != nil {
		log.Fatalf("Failed to generate collector config file: %v", err)
	}
	
	// Update framework with new config
	framework, err = NewTestFramework(configFile)
	if err != nil {
		log.Fatalf("Failed to create test framework: %v", err)
	}
	
	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}
	defer framework.StopCollector()
	
	// Allow collector to initialize
	time.Sleep(2 * time.Second)
	
	// Create traces with varying numbers of spans per trace
	spanCounts := []int{2, 5, 10, 25, 50}
	tracesPerCount := 100
	
	for _, spanCount := range spanCounts {
		log.Printf("Testing trace preservation with %d spans per trace", spanCount)
		
		totalSpans := spanCount * tracesPerCount
		traces := framework.GenerateTestSpans(totalSpans, spanCount)
		
		if err := framework.SendTraces(ctx, traces); err != nil {
			log.Fatalf("Failed to send traces: %v", err)
		}
	}
	
	// Wait for processing to complete
	time.Sleep(10 * time.Second)
	
	// Restore original trace-aware setting
	config.TraceAware = originalTraceAware
}

func main() {
	Run()
}