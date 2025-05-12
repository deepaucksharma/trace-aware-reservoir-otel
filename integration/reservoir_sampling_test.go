package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReservoirSampling_BasicOperation(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework
	tf, err := NewTestFramework(t, WithInMemoryDB())
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Setup processor with small reservoir
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(10),
		WithWindowDuration("1s"),
		WithCheckpointInterval("500ms"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(1000),
		WithTraceBufferTimeout("500ms"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Send more traces than reservoir size
	err = tf.SendTestTraces(ctx, 0, 20, 2)
	require.NoError(t, err, "Failed to send test traces")

	// Wait for processor to process traces
	time.Sleep(2 * time.Second)

	// Force export
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Verify results
	capturedTraces := tf.GetCapturedTraces()
	assert.NotEmpty(t, capturedTraces, "No traces were captured")

	uniqueTraceCount := tf.CountUniqueTraces()
	assert.Greater(t, uniqueTraceCount, 0, "No unique traces were captured")
	assert.LessOrEqual(t, uniqueTraceCount, 10, "Too many traces were captured, expected at most 10")

	// Shutdown processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}

func TestReservoirSampling_Persistence(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework
	tf, err := NewTestFramework(t)
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// --- Phase 1: Create and populate processor ---
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(15),
		WithWindowDuration("1s"),
		WithCheckpointInterval("500ms"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(1000),
		WithTraceBufferTimeout("500ms"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Send traces to first processor
	err = tf.SendTestTraces(ctx, 0, 30, 2)
	require.NoError(t, err, "Failed to send test traces")

	// Wait for processor to process traces and checkpoint
	time.Sleep(2 * time.Second)

	// Force export
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Verify results from first processor
	capturedTraces1 := tf.GetCapturedTraces()
	assert.NotEmpty(t, capturedTraces1, "No traces were captured by first processor")

	uniqueTraceCount1 := tf.CountUniqueTraces()
	assert.Greater(t, uniqueTraceCount1, 0, "No unique traces were captured by first processor")

	// Verify checkpoint file exists
	assert.True(t, tf.CheckpointFileExists(), "Checkpoint file should exist")
	checkpointPath := tf.GetCheckpointPath()

	// Get checkpoint file size
	size1, err := tf.GetCheckpointFileSize()
	require.NoError(t, err, "Failed to get checkpoint file size")
	assert.Greater(t, size1, int64(0), "Checkpoint file size should be greater than 0")

	// Shutdown first processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown first processor")

	// Reset test framework for second processor
	err = tf.Reset(ctx)
	require.NoError(t, err, "Failed to reset test framework")

	// --- Phase 2: Create a new processor that loads from checkpoint ---
	err = tf.Setup(ctx,
		WithReservoirSize(15),
		WithWindowDuration("1s"),
		WithCheckpointInterval("500ms"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(1000),
		WithTraceBufferTimeout("500ms"),
	)
	require.NoError(t, err, "Failed to setup second processor")

	// Verify checkpoint path is the same
	assert.Equal(t, checkpointPath, tf.GetCheckpointPath(), "Checkpoint path should be the same")

	// Wait for processor to restore from checkpoint
	time.Sleep(1 * time.Second)

	// Force export to get traces from restored processor
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Verify results from second processor
	capturedTraces2 := tf.GetCapturedTraces()
	assert.NotEmpty(t, capturedTraces2, "No traces were captured by second processor")

	uniqueTraceCount2 := tf.CountUniqueTraces()
	assert.Greater(t, uniqueTraceCount2, 0, "No unique traces were restored from checkpoint")

	// Send more traces to second processor
	err = tf.SendTestTraces(ctx, 100, 20, 2)
	require.NoError(t, err, "Failed to send more test traces")

	// Wait for processor to process new traces
	time.Sleep(2 * time.Second)

	// Force export
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Verify more traces were captured
	capturedTraces3 := tf.GetCapturedTraces()
	assert.Greater(t, len(capturedTraces3), len(capturedTraces2), "Second processor should capture more traces")

	// Shutdown second processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown second processor")
}

func TestReservoirSampling_TraceAwareness(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB())
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Setup processor with trace-aware mode
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(20),
		WithWindowDuration("1s"),
		WithCheckpointInterval("500ms"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(1000),
		WithTraceBufferTimeout("500ms"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Send traces with multiple spans per trace
	const numTraces = 30
	const spansPerTrace = 5
	err = tf.SendTestTraces(ctx, 0, numTraces, spansPerTrace)
	require.NoError(t, err, "Failed to send test traces")

	// Wait for processor to process traces
	time.Sleep(2 * time.Second)

	// Force export
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Get captured traces
	capturedTraces := tf.GetCapturedTraces()
	
	// Check each trace batch for parent-child relationships
	// In trace-aware mode, if a span from a trace is sampled, 
	// all spans from that trace should be sampled
	for _, batch := range capturedTraces {
		// Create a map of trace IDs to count spans per trace
		traceSpanCounts := make(map[string]int)
		
		// Count spans per trace ID
		for i := 0; i < batch.ResourceSpans().Len(); i++ {
			rs := batch.ResourceSpans().At(i)
			for j := 0; j < rs.ScopeSpans().Len(); j++ {
				ss := rs.ScopeSpans().At(j)
				for k := 0; k < ss.Spans().Len(); k++ {
					span := ss.Spans().At(k)
					traceID := span.TraceID().String()
					traceSpanCounts[traceID]++
				}
			}
		}
		
		// Verify that each trace has all spans sampled
		for traceID, count := range traceSpanCounts {
			// In our test, we have spansPerTrace spans for each trace
			// If a trace is sampled, all spans from that trace should be sampled
			assert.Equal(t, spansPerTrace, count, 
				"Trace-aware mode should sample all spans from a trace: %s", traceID)
		}
	}

	// Shutdown processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}

func TestReservoirSampling_WindowRollover(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB())
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Setup processor with short window duration
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(10),
		WithWindowDuration("1s"), // Short window for testing
		WithCheckpointInterval("500ms"),
		WithTraceAware(true),
		WithTraceBufferMaxSize(1000),
		WithTraceBufferTimeout("500ms"),
	)
	require.NoError(t, err, "Failed to setup processor")

	// Send first batch of traces
	err = tf.SendTestTraces(ctx, 0, 20, 2)
	require.NoError(t, err, "Failed to send first batch of traces")

	// Wait for first window to complete
	time.Sleep(1500 * time.Millisecond)

	// Send second batch of traces
	err = tf.SendTestTraces(ctx, 100, 20, 2)
	require.NoError(t, err, "Failed to send second batch of traces")

	// Wait for second window to complete
	time.Sleep(1500 * time.Millisecond)

	// Force export
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Get captured traces
	capturedTraces := tf.GetCapturedTraces()
	assert.GreaterOrEqual(t, len(capturedTraces), 2, "At least two windows should have been captured")

	// Verify that for each window, the number of traces is less than or equal to the reservoir size
	for i, window := range capturedTraces {
		uniqueTraces := countUniqueTraces([]ptrace.Traces{window})
		assert.LessOrEqual(t, uniqueTraces, 10, 
			"Window %d should have at most 10 traces, got %d", i, uniqueTraces)
	}

	// Shutdown processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}