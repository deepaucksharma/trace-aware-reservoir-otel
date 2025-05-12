package integration

import (
	"context"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestReservoirSampling_StressTest performs a stress test on the reservoir sampler
// with very high trace volumes to identify bottlenecks and failure modes
func TestReservoirSampling_StressTest(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Configure logger with high performance settings
	logConfig := zap.NewProductionConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logConfig.Sampling = &zap.SamplingConfig{
		Initial:    100,
		Thereafter: 100,
	}
	logConfig.OutputPaths = []string{"stdout"}
	logger, err := logConfig.Build()
	require.NoError(t, err, "Failed to build logger")

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB(), WithLogger(logger))
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Create test utilities
	utils := NewTestUtils(tf)

	// Setup processor with stress test settings
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(5000),
		WithWindowDuration("20s"),
		WithCheckpointInterval("10s"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(200000),
		WithTraceBufferTimeout("5s"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Define stress test parameters
	stressTestBatches := 20
	tracesPerBatch := 2000
	spansPerTrace := 4
	delayBetweenBatches := 100 * time.Millisecond

	// Measure memory before stress test
	var beforeMemory runtime.MemStats
	runtime.ReadMemStats(&beforeMemory)
	
	t.Logf("Memory before stress test: Alloc=%d MB, Sys=%d MB, NumGC=%d", 
		beforeMemory.Alloc/1024/1024, 
		beforeMemory.Sys/1024/1024, 
		beforeMemory.NumGC)

	// Start time measurement
	startTime := time.Now()

	// Run high load simulation
	err = utils.SimulateHighLoad(ctx, stressTestBatches, tracesPerBatch, spansPerTrace, delayBetweenBatches)
	require.NoError(t, err, "Stress test load simulation failed")

	// Wait for processing to complete
	utils.WaitForProcessing(15 * time.Second)

	// Force export
	utils.ForceExportAndWait(2 * time.Second)

	// Measure memory after stress test
	var afterMemory runtime.MemStats
	runtime.ReadMemStats(&afterMemory)
	
	t.Logf("Memory after stress test: Alloc=%d MB, Sys=%d MB, NumGC=%d", 
		afterMemory.Alloc/1024/1024, 
		afterMemory.Sys/1024/1024, 
		afterMemory.NumGC)

	// Calculate elapsed time and throughput
	elapsedTime := time.Since(startTime)
	totalTraces := stressTestBatches * tracesPerBatch
	throughput := float64(totalTraces) / elapsedTime.Seconds()
	
	// Log stress test results
	t.Logf("Stress test completed in %v", elapsedTime)
	t.Logf("Total traces: %d, Throughput: %.2f traces/second", totalTraces, throughput)
	t.Logf("GC cycles during test: %d", afterMemory.NumGC-beforeMemory.NumGC)
	t.Logf("Memory growth: %.2f MB", float64(afterMemory.Alloc-beforeMemory.Alloc)/1024/1024)

	// Analyze captured traces
	capturedTraces := tf.GetCapturedTraces()
	uniqueTraceCount := tf.CountUniqueTraces()
	
	t.Logf("Captured trace batches: %d", len(capturedTraces))
	t.Logf("Unique trace count: %d", uniqueTraceCount)
	
	// Get extracted statistics
	stats := utils.ExtractTraceStatistics()
	t.Logf("Trace statistics: %+v", stats)
	
	// Basic assertions
	assert.True(t, throughput > 1000, "Throughput should be greater than 1000 traces/second under stress")
	assert.NotEmpty(t, capturedTraces, "No traces were captured")
	assert.Greater(t, uniqueTraceCount, 0, "No unique traces were captured")
	assert.LessOrEqual(t, uniqueTraceCount, 5000, "Too many traces were captured, expected at most 5000")
	
	// Memory growth should be bounded
	memoryGrowthMB := float64(afterMemory.Alloc-beforeMemory.Alloc) / 1024 / 1024
	assert.True(t, memoryGrowthMB < 1000, "Memory growth exceeded 1000 MB threshold")
	
	// Verify trace completeness if trace-aware
	isComplete, _ := utils.VerifyTraceCompleteness(spansPerTrace)
	assert.True(t, isComplete, "Trace completeness check failed under stress")

	// Shutdown the processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}

// TestReservoirSampling_Longevity tests the behavior of the reservoir sampler
// during extended operation
func TestReservoirSampling_Longevity(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping longevity test in short mode")
	}

	// Configure logger
	logConfig := zap.NewProductionConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logger, err := logConfig.Build()
	require.NoError(t, err, "Failed to build logger")

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB(), WithLogger(logger))
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Create test utilities
	utils := NewTestUtils(tf)

	// Setup processor with configuration for long-running test
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(200),
		WithWindowDuration("5s"),
		WithCheckpointInterval("2s"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(10000),
		WithTraceBufferTimeout("2s"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Define longevity test parameters
	testDuration := 60 * time.Second  // 1 minute longevity test
	batchSize := 500
	batchInterval := 5 * time.Second
	spansPerTrace := 3
	
	// Track memory stats over time
	type memorySnapshot struct {
		timestamp   time.Time
		allocatedMB float64
		systemMB    float64
		gcCycles    uint32
	}
	
	memorySnapshots := []memorySnapshot{}
	
	// Take initial memory snapshot
	var initialMemory runtime.MemStats
	runtime.ReadMemStats(&initialMemory)
	memorySnapshots = append(memorySnapshots, memorySnapshot{
		timestamp:   time.Now(),
		allocatedMB: float64(initialMemory.Alloc) / 1024 / 1024,
		systemMB:    float64(initialMemory.Sys) / 1024 / 1024,
		gcCycles:    initialMemory.NumGC,
	})
	
	t.Logf("Starting longevity test for %v", testDuration)
	testStart := time.Now()
	batchCount := 0
	
	// Run until test duration is reached
	for time.Since(testStart) < testDuration {
		batchStart := time.Now()
		
		// Generate and send a batch of traces
		startIdx := batchCount * batchSize
		traces := generateTestTraces(startIdx, batchSize, spansPerTrace)
		err = tf.SendTraces(ctx, traces)
		require.NoError(t, err, "Failed to send traces in batch %d", batchCount)
		
		// Force export every 3 batches
		if batchCount%3 == 2 {
			utils.ForceExportAndWait(500 * time.Millisecond)
			
			// Take memory snapshot
			var currentMemory runtime.MemStats
			runtime.ReadMemStats(&currentMemory)
			memorySnapshots = append(memorySnapshots, memorySnapshot{
				timestamp:   time.Now(),
				allocatedMB: float64(currentMemory.Alloc) / 1024 / 1024,
				systemMB:    float64(currentMemory.Sys) / 1024 / 1024,
				gcCycles:    currentMemory.NumGC,
			})
			
			t.Logf("Memory at %v: Alloc=%.2f MB, Sys=%.2f MB, NumGC=%d",
				time.Since(testStart),
				float64(currentMemory.Alloc)/1024/1024,
				float64(currentMemory.Sys)/1024/1024,
				currentMemory.NumGC)
		}
		
		batchCount++
		
		// Wait for remaining interval time
		elapsed := time.Since(batchStart)
		if elapsed < batchInterval {
			time.Sleep(batchInterval - elapsed)
		}
	}
	
	// Final export
	utils.ForceExportAndWait(1 * time.Second)
	
	// Take final memory snapshot
	var finalMemory runtime.MemStats
	runtime.ReadMemStats(&finalMemory)
	memorySnapshots = append(memorySnapshots, memorySnapshot{
		timestamp:   time.Now(),
		allocatedMB: float64(finalMemory.Alloc) / 1024 / 1024,
		systemMB:    float64(finalMemory.Sys) / 1024 / 1024,
		gcCycles:    finalMemory.NumGC,
	})
	
	// Calculate total traces sent
	totalTracesSent := batchCount * batchSize
	
	// Log test results
	t.Logf("Longevity test completed after %v", time.Since(testStart))
	t.Logf("Total traces sent: %d, Batches: %d", totalTracesSent, batchCount)
	t.Logf("Final memory: Alloc=%.2f MB, Sys=%.2f MB, NumGC=%d",
		float64(finalMemory.Alloc)/1024/1024,
		float64(finalMemory.Sys)/1024/1024,
		finalMemory.NumGC)
	
	// Analyze memory stability
	// Calculate standard deviation of memory allocation
	var sum, sumSq float64
	for _, snapshot := range memorySnapshots {
		sum += snapshot.allocatedMB
		sumSq += snapshot.allocatedMB * snapshot.allocatedMB
	}
	
	mean := sum / float64(len(memorySnapshots))
	variance := (sumSq / float64(len(memorySnapshots))) - (mean * mean)
	stdDev := math.Sqrt(variance)
	
	t.Logf("Memory statistics - Mean: %.2f MB, StdDev: %.2f MB", mean, stdDev)
	
	// Calculate memory growth rate (MB per minute)
	initialMB := memorySnapshots[0].allocatedMB
	finalMB := memorySnapshots[len(memorySnapshots)-1].allocatedMB
	durationMinutes := time.Since(testStart).Minutes()
	growthRatePerMinute := (finalMB - initialMB) / durationMinutes
	
	t.Logf("Memory growth rate: %.2f MB/minute", growthRatePerMinute)
	
	// Calculate GC frequency (cycles per minute)
	initialGC := memorySnapshots[0].gcCycles
	finalGC := memorySnapshots[len(memorySnapshots)-1].gcCycles
	gcCyclesPerMinute := float64(finalGC-initialGC) / durationMinutes
	
	t.Logf("GC frequency: %.2f cycles/minute", gcCyclesPerMinute)
	
	// Analyze captured traces
	capturedTraces := tf.GetCapturedTraces()
	uniqueTraceCount := tf.CountUniqueTraces()
	t.Logf("Captured trace batches: %d", len(capturedTraces))
	t.Logf("Unique trace count: %d", uniqueTraceCount)
	
	// Get extracted statistics
	stats := utils.ExtractTraceStatistics()
	t.Logf("Trace statistics: %+v", stats)
	
	// Assertions for longevity
	assert.NotEmpty(t, capturedTraces, "No traces were captured during longevity test")
	assert.Greater(t, uniqueTraceCount, 0, "No unique traces were captured")
	
	// Memory stability assertions
	assert.True(t, stdDev/mean < 0.5, "Memory allocation standard deviation exceeds 50% of mean")
	assert.True(t, growthRatePerMinute < 100, "Memory growth rate exceeds 100 MB/minute")
	
	// Verify trace completeness
	isComplete, _ := utils.VerifyTraceCompleteness(spansPerTrace)
	assert.True(t, isComplete, "Trace completeness check failed after longevity test")
	
	// Shutdown the processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}

// TestReservoirSampling_Recovery tests the recovery capabilities
// of the reservoir sampler after simulated failures
func TestReservoirSampling_Recovery(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping recovery test in short mode")
	}

	// Configure logger
	logConfig := zap.NewProductionConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logger, err := logConfig.Build()
	require.NoError(t, err, "Failed to build logger")

	// Create test data directory for persistence
	tf, err := NewTestFramework(t, WithLogger(logger))
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Create test utilities
	utils := NewTestUtils(tf)

	// Common test parameters
	ctx := context.Background()
	reservoirSize := 200
	tracesPerBatch := 500
	spansPerTrace := 3

	// Phase 1: Initial processing with checkpointing
	t.Log("Phase 1: Initial processing with checkpointing")
	
	// Setup first processor with persistence
	err = tf.Setup(ctx,
		WithReservoirSize(reservoirSize),
		WithWindowDuration("5s"),
		WithCheckpointInterval("1s"), // Frequent checkpointing
		WithTraceAware(true),
		WithTraceBufferMaxSize(10000),
		WithTraceBufferTimeout("2s"),
	)
	require.NoError(t, err, "Failed to setup first processor")

	// Send traces to first processor
	traceIDs, err := utils.GenerateAndSendTraces(ctx, 0, tracesPerBatch, spansPerTrace)
	require.NoError(t, err, "Failed to send traces to first processor")

	// Wait for processing and checkpoint
	utils.WaitForProcessing(3 * time.Second)
	t.Logf("Sent %d traces to first processor", len(traceIDs))

	// Force export
	utils.ForceExportAndWait(1 * time.Second)

	// Verify first processor has captured traces
	capturedTraces1 := tf.GetCapturedTraces()
	assert.NotEmpty(t, capturedTraces1, "First processor should capture traces")
	
	// Verify checkpoint file exists and has data
	checkpointPath := tf.GetCheckpointPath()
	assert.True(t, tf.CheckpointFileExists(), "Checkpoint file should exist")
	t.Logf("Checkpoint file: %s", checkpointPath)
	
	// Get checkpoint file size
	size1, err := tf.GetCheckpointFileSize()
	require.NoError(t, err, "Failed to get checkpoint file size")
	assert.Greater(t, size1, int64(0), "Checkpoint file size should be greater than 0")
	t.Logf("Checkpoint file size: %d bytes", size1)

	// Shut down first processor, simulating a crash
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown first processor")

	// Phase 2: Recovery from checkpoint
	t.Log("Phase 2: Recovery from checkpoint")
	
	// Reset test framework for second processor
	err = tf.Reset(ctx)
	require.NoError(t, err, "Failed to reset test framework")
	
	// Setup second processor with same config to test recovery
	err = tf.Setup(ctx,
		WithReservoirSize(reservoirSize),
		WithWindowDuration("5s"),
		WithCheckpointInterval("1s"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(10000),
		WithTraceBufferTimeout("2s"),
	)
	require.NoError(t, err, "Failed to setup second processor")

	// Wait for processor to load from checkpoint
	utils.WaitForProcessing(2 * time.Second)

	// Force export to see what was recovered
	utils.ForceExportAndWait(1 * time.Second)

	// Verify second processor recovered traces from checkpoint
	capturedTraces2 := tf.GetCapturedTraces()
	assert.NotEmpty(t, capturedTraces2, "Second processor should recover traces from checkpoint")
	
	// Check that some traces were recovered
	recoveredTraceCount := tf.CountUniqueTraces()
	t.Logf("Recovered %d traces from checkpoint", recoveredTraceCount)
	assert.Greater(t, recoveredTraceCount, 0, "No traces were recovered from checkpoint")
	
	// Phase 3: Continue processing with recovered state
	t.Log("Phase 3: Continue processing with recovered state")
	
	// Send new traces to second processor
	newTraceIDs, err := utils.GenerateAndSendTraces(ctx, tracesPerBatch, tracesPerBatch, spansPerTrace)
	require.NoError(t, err, "Failed to send new traces to second processor")
	t.Logf("Sent %d new traces to second processor", len(newTraceIDs))

	// Wait for processing and checkpoint
	utils.WaitForProcessing(3 * time.Second)

	// Force export
	utils.ForceExportAndWait(1 * time.Second)

	// Get final checkpoint size
	size2, err := tf.GetCheckpointFileSize()
	require.NoError(t, err, "Failed to get final checkpoint file size")
	t.Logf("Final checkpoint file size: %d bytes", size2)
	
	// Get final trace statistics
	allCapturedTraces := tf.GetCapturedTraces()
	finalTraceCount := tf.CountUniqueTraces()
	t.Logf("Final trace count: %d", finalTraceCount)
	
	// Final size should be different from initial size due to new traces
	assert.NotEqual(t, size1, size2, "Checkpoint file size should change after processing new traces")
	
	// Final trace count should be capped by reservoir size
	assert.LessOrEqual(t, finalTraceCount, reservoirSize, 
		"Final trace count should not exceed reservoir size")
	
	// Check trace completeness
	isComplete, _ := utils.VerifyTraceCompleteness(spansPerTrace)
	assert.True(t, isComplete, "Trace completeness check failed after recovery")
	
	// Verify the processor can receive more traces
	additionalTraceIDs, err := utils.GenerateAndSendTraces(ctx, tracesPerBatch*2, 100, spansPerTrace)
	require.NoError(t, err, "Failed to send additional traces after recovery")
	t.Logf("Sent %d additional traces after recovery", len(additionalTraceIDs))
	
	// Wait for processing
	utils.WaitForProcessing(2 * time.Second)
	
	// Force export
	utils.ForceExportAndWait(1 * time.Second)
	
	// Final shutdown
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor after recovery test")
}