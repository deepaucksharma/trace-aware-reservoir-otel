package integration

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestReservoirSampling_Performance tests the performance characteristics of the reservoir sampler
func TestReservoirSampling_Performance(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
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

	// Setup processor with performance-optimized settings
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(1000),
		WithWindowDuration("30s"),
		WithCheckpointInterval("10s"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(100000),
		WithTraceBufferTimeout("5s"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Create a slice to store memory stats snapshots
	var memStats []runtime.MemStats

	// Take initial memory snapshot
	var initialMemory runtime.MemStats
	runtime.ReadMemStats(&initialMemory)
	memStats = append(memStats, initialMemory)
	
	// Log initial memory usage
	t.Logf("Initial memory usage: Alloc=%d MB, Sys=%d MB, NumGC=%d", 
		initialMemory.Alloc/1024/1024, 
		initialMemory.Sys/1024/1024, 
		initialMemory.NumGC)

	// Define batch parameters
	batchCount := 10
	tracesPerBatch := 1000
	spansPerTrace := 5
	delayBetweenBatches := 100 * time.Millisecond

	// Start time measurement
	startTime := time.Now()

	// Run high load simulation
	err = utils.SimulateHighLoad(ctx, batchCount, tracesPerBatch, spansPerTrace, delayBetweenBatches)
	require.NoError(t, err, "High load simulation failed")

	// Wait for processing to complete
	utils.WaitForProcessing(5 * time.Second)

	// Take memory snapshot after load
	var afterLoadMemory runtime.MemStats
	runtime.ReadMemStats(&afterLoadMemory)
	memStats = append(memStats, afterLoadMemory)
	
	// Log memory usage after load
	t.Logf("After load memory usage: Alloc=%d MB, Sys=%d MB, NumGC=%d", 
		afterLoadMemory.Alloc/1024/1024, 
		afterLoadMemory.Sys/1024/1024, 
		afterLoadMemory.NumGC)

	// Force export
	utils.ForceExportAndWait(1 * time.Second)

	// Take memory snapshot after export
	var afterExportMemory runtime.MemStats
	runtime.ReadMemStats(&afterExportMemory)
	memStats = append(memStats, afterExportMemory)
	
	// Log memory usage after export
	t.Logf("After export memory usage: Alloc=%d MB, Sys=%d MB, NumGC=%d", 
		afterExportMemory.Alloc/1024/1024, 
		afterExportMemory.Sys/1024/1024, 
		afterExportMemory.NumGC)

	// Calculate elapsed time
	elapsedTime := time.Since(startTime)
	throughput := float64(batchCount*tracesPerBatch) / elapsedTime.Seconds()

	// Log performance results
	t.Logf("Performance test completed in %v", elapsedTime)
	t.Logf("Throughput: %.2f traces/second", throughput)
	t.Logf("GC cycles during test: %d", afterExportMemory.NumGC-initialMemory.NumGC)

	// Verify memory usage is within acceptable bounds
	// Memory should not grow uncontrollably
	memoryGrowthRatio := float64(afterExportMemory.Alloc) / float64(initialMemory.Alloc)
	t.Logf("Memory growth ratio: %.2f", memoryGrowthRatio)
	
	// Analyze captured traces
	capturedTraces := tf.GetCapturedTraces()
	uniqueTraceCount := tf.CountUniqueTraces()
	
	// Get extracted statistics
	stats := utils.ExtractTraceStatistics()
	t.Logf("Trace statistics: %+v", stats)
	
	// Performance assertions
	assert.True(t, throughput > 100, "Throughput should be greater than 100 traces/second")
	assert.True(t, memoryGrowthRatio < 10, "Memory growth should be less than 10x")
	assert.NotEmpty(t, capturedTraces, "No traces were captured")
	assert.Greater(t, uniqueTraceCount, 0, "No unique traces were captured")
	assert.LessOrEqual(t, uniqueTraceCount, 1000, "Too many traces were captured, expected at most 1000")

	// Shutdown the processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}

// TestReservoirSampling_Scalability tests the scalability of the reservoir sampler
// with increasingly large trace volumes
func TestReservoirSampling_Scalability(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	// Configure logger with high performance settings
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

	// Define test scale levels
	scaleLevels := []struct {
		name           string
		tracesPerBatch int
		batchCount     int
		reservoirSize  int
	}{
		{"small", 100, 5, 50},
		{"medium", 500, 10, 100},
		{"large", 1000, 20, 200},
	}

	// Common configuration
	ctx := context.Background()
	spansPerTrace := 3
	delayBetweenBatches := 50 * time.Millisecond

	for _, level := range scaleLevels {
		t.Run(level.name, func(t *testing.T) {
			// Reset the framework for this scale level
			err = tf.Reset(ctx)
			require.NoError(t, err, "Failed to reset test framework")

			// Setup processor with appropriate reservoir size for this scale level
			err = tf.Setup(ctx,
				WithReservoirSize(level.reservoirSize),
				WithWindowDuration("10s"),
				WithCheckpointInterval("5s"),
				WithTraceAware(true),
				WithTraceBufferMaxSize(level.reservoirSize*20),
				WithTraceBufferTimeout("2s"),
			)
			require.NoError(t, err, "Failed to setup processor")

			// Take initial memory snapshot
			var initialMemory runtime.MemStats
			runtime.ReadMemStats(&initialMemory)
			
			// Start time measurement
			startTime := time.Now()

			// Run load simulation
			err = utils.SimulateHighLoad(ctx, level.batchCount, level.tracesPerBatch, spansPerTrace, delayBetweenBatches)
			require.NoError(t, err, "Load simulation failed")

			// Wait for processing to complete
			utils.WaitForProcessing(5 * time.Second)

			// Force export
			utils.ForceExportAndWait(1 * time.Second)

			// Take memory snapshot after test
			var finalMemory runtime.MemStats
			runtime.ReadMemStats(&finalMemory)

			// Calculate elapsed time and throughput
			elapsedTime := time.Since(startTime)
			throughput := float64(level.batchCount*level.tracesPerBatch) / elapsedTime.Seconds()
			memoryGrowth := float64(finalMemory.Alloc-initialMemory.Alloc) / 1024 / 1024 // MB

			// Log performance results
			t.Logf("Scale level %s completed in %v", level.name, elapsedTime)
			t.Logf("Throughput: %.2f traces/second", throughput)
			t.Logf("Memory growth: %.2f MB", memoryGrowth)
			t.Logf("GC cycles: %d", finalMemory.NumGC-initialMemory.NumGC)

			// Verify results
			uniqueTraceCount := tf.CountUniqueTraces()
			t.Logf("Unique trace count: %d", uniqueTraceCount)
			
			// Check reservoir bounds
			assert.LessOrEqual(t, uniqueTraceCount, level.reservoirSize, 
				"Captured traces exceeds reservoir size")
			
			// Verify reasonable memory usage
			assert.Less(t, memoryGrowth, float64(level.tracesPerBatch*spansPerTrace*10)/1024,
				"Memory growth exceeds expected bounds")
			
			// Shutdown the processor
			err = tf.Shutdown(ctx)
			require.NoError(t, err, "Failed to shutdown processor")
		})
	}
}

// TestReservoirSampling_ResourceUsage tests the resource usage (CPU, memory)
// of the reservoir sampler under different configurations
func TestReservoirSampling_ResourceUsage(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping resource usage test in short mode")
	}

	// Configure logger with high performance settings
	logConfig := zap.NewProductionConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logger, err := logConfig.Build()
	require.NoError(t, err, "Failed to build logger")

	// Test configurations
	configs := []struct {
		name                string
		traceAware          bool
		reservoirSize       int
		traceBufferMaxSize  int
		traceBufferTimeout  string
		expectedPerformance float64 // relative to baseline (1.0)
	}{
		{
			name:                "baseline",
			traceAware:          false,
			reservoirSize:       100,
			traceBufferMaxSize:  0,
			traceBufferTimeout:  "0s",
			expectedPerformance: 1.0,
		},
		{
			name:                "trace_aware",
			traceAware:          true,
			reservoirSize:       100,
			traceBufferMaxSize:  10000,
			traceBufferTimeout:  "5s",
			expectedPerformance: 0.8, // Expected to be slower than baseline
		},
		{
			name:                "large_reservoir",
			traceAware:          false,
			reservoirSize:       1000,
			traceBufferMaxSize:  0,
			traceBufferTimeout:  "0s",
			expectedPerformance: 0.9, // Expected to be slightly slower than baseline
		},
		{
			name:                "trace_aware_large_reservoir",
			traceAware:          true,
			reservoirSize:       1000,
			traceBufferMaxSize:  50000,
			traceBufferTimeout:  "5s",
			expectedPerformance: 0.7, // Expected to be significantly slower than baseline
		},
	}

	// Common test parameters
	ctx := context.Background()
	tracesPerBatch := 500
	batchCount := 5
	spansPerTrace := 3

	// Store baseline performance for comparison
	var baselineThroughput float64

	for i, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			// Create test framework with in-memory DB for each config
			tf, err := NewTestFramework(t, WithInMemoryDB(), WithLogger(logger))
			require.NoError(t, err, "Failed to create test framework")
			defer tf.Cleanup()

			// Create test utilities
			utils := NewTestUtils(tf)

			// Setup processor with this configuration
			err = tf.Setup(ctx,
				WithReservoirSize(config.reservoirSize),
				WithWindowDuration("10s"),
				WithCheckpointInterval("5s"),
				WithTraceAware(config.traceAware),
				WithTraceBufferMaxSize(config.traceBufferMaxSize),
				WithTraceBufferTimeout(config.traceBufferTimeout),
			)
			require.NoError(t, err, "Failed to setup processor")

			// Take initial memory snapshot
			var initialMemory runtime.MemStats
			runtime.ReadMemStats(&initialMemory)
			
			// Start time measurement
			startTime := time.Now()

			// Run load simulation
			err = utils.SimulateHighLoad(ctx, batchCount, tracesPerBatch, spansPerTrace, 50*time.Millisecond)
			require.NoError(t, err, "Load simulation failed")

			// Wait for processing to complete
			utils.WaitForProcessing(5 * time.Second)

			// Force export
			utils.ForceExportAndWait(1 * time.Second)

			// Take memory snapshot after test
			var finalMemory runtime.MemStats
			runtime.ReadMemStats(&finalMemory)

			// Calculate elapsed time and throughput
			elapsedTime := time.Since(startTime)
			throughput := float64(batchCount*tracesPerBatch) / elapsedTime.Seconds()
			memoryGrowth := float64(finalMemory.Alloc-initialMemory.Alloc) / 1024 / 1024 // MB

			// Store baseline throughput for comparison
			if i == 0 {
				baselineThroughput = throughput
			}

			// Log performance results
			t.Logf("Config %s completed in %v", config.name, elapsedTime)
			t.Logf("Throughput: %.2f traces/second (%.2f relative to baseline)", 
				throughput, throughput/baselineThroughput)
			t.Logf("Memory growth: %.2f MB", memoryGrowth)
			t.Logf("GC cycles: %d", finalMemory.NumGC-initialMemory.NumGC)

			// Verify trace awareness if enabled
			if config.traceAware {
				traceStats := utils.ExtractTraceStatistics()
				t.Logf("Trace statistics for config %s: %+v", config.name, traceStats)
				
				// Verify trace completeness
				isComplete, spanCounts := utils.VerifyTraceCompleteness(spansPerTrace)
				assert.True(t, isComplete, 
					"Trace completeness check failed: some traces have missing spans: %v", spanCounts)
			}

			// Verify relative performance
			if i > 0 {
				relativePerformance := throughput / baselineThroughput
				t.Logf("Relative performance: %.2f (expected >= %.2f)", 
					relativePerformance, config.expectedPerformance)
				
				// Allow some variance in performance metrics
				assert.GreaterOrEqual(t, relativePerformance, config.expectedPerformance*0.7, 
					"Performance is significantly worse than expected")
			}

			// Shutdown the processor
			err = tf.Shutdown(ctx)
			require.NoError(t, err, "Failed to shutdown processor")
		})
	}
}