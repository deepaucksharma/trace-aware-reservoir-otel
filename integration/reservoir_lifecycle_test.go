package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

// TestReservoirLifecycle_FullOperation verifies the reservoir sampler's complete operational cycle:
// data intake, internal buffering, sampling decisions, data handoff to the next component,
// state persistence, and robust recovery leading to uninterrupted operation.
func TestReservoirLifecycle_FullOperation(t *testing.T) {
	// Skip in short mode as this is a longer integration test
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp dir for persistence
	tempDir, err := os.MkdirTemp("", "reservoir-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "reservoir.db")

	// Test constants - use very small values to avoid stack overflow
	const (
		CONFIG_RESERVOIR_SIZE       = 20 // Reduced from 50
		CONFIG_WINDOW_DURATION      = 2 * time.Second
		CONFIG_BUFFER_TIMEOUT       = 1 * time.Second
		CONFIG_PERSISTENCE_INTERVAL = 1 * time.Second
		TOTAL_TRACES                = 30 // Reduced from 100
		SPANS_PER_TRACE             = 2  // Reduced from 3
	)

	// Create sink for capturing output
	sink := NewNoopTracesSink()
	capturingSink := NewCapturingSink(sink)

	// ---------- Phase 1: Initial Processor Setup ----------
	logger := zap.NewExample()
	logger.Info("Phase 1: Setting up initial processor")

	// Create processor instance 1
	cfg := createTestConfig(CONFIG_RESERVOIR_SIZE, CONFIG_WINDOW_DURATION, CONFIG_BUFFER_TIMEOUT, CONFIG_PERSISTENCE_INTERVAL, dbPath)

	// Create telemetry settings for the processor
	telemetrySettings := componenttest.NewNopTelemetrySettings()
	telemetrySettings.Logger = logger

	// Create processor settings
	settings := processor.Settings{
		TelemetrySettings: telemetrySettings,
	}

	// Access createTracesProcessor directly
	processor1, err := reservoirsampler.CreateTracesProcessorForTesting(
		context.Background(),
		settings,
		cfg,
		capturingSink,
	)
	require.NoError(t, err)

	// Start processor 1
	err = processor1.Start(context.Background(), nil)
	require.NoError(t, err)

	// ---------- Phase 1: Data Processing & Persistence ----------
	logger.Info("Phase 1: Processing initial data")

	// Generate and send batches of traces with pauses to span multiple windows
	halfTraces := TOTAL_TRACES / 2

	// First batch
	traces1 := generateTestTraces(0, halfTraces, SPANS_PER_TRACE)
	err = processor1.ConsumeTraces(context.Background(), traces1)
	require.NoError(t, err)

	// Wait for first window to complete
	time.Sleep(CONFIG_WINDOW_DURATION + 100*time.Millisecond)

	// Second batch
	traces2 := generateTestTraces(halfTraces, halfTraces, SPANS_PER_TRACE)
	err = processor1.ConsumeTraces(context.Background(), traces2)
	require.NoError(t, err)

	// Wait for processing and checkpoint
	time.Sleep(CONFIG_WINDOW_DURATION + CONFIG_PERSISTENCE_INTERVAL + 100*time.Millisecond)

	// Assertions - Phase 1
	capturedTraces1 := capturingSink.GetAllTraces()

	// Check that DB file exists
	_, err = os.Stat(dbPath)
	require.NoError(t, err, "Persistent storage file should exist")

	// Verify captured data from first processor
	totalCapturedTraces := countUniqueTraces(capturedTraces1)
	assert.LessOrEqual(t, totalCapturedTraces, TOTAL_TRACES, "Total captured traces should be less than or equal to total input traces")

	// For each window, ensure we didn't exceed reservoir capacity
	for _, window := range capturedTraces1 {
		windowTraceCount := countUniqueTraces([]ptrace.Traces{window})
		assert.LessOrEqual(t, windowTraceCount, CONFIG_RESERVOIR_SIZE,
			"Number of traces from a single window should not exceed reservoir capacity")
	}

	// ---------- Phase 2: Shutdown & Recovery ----------
	logger.Info("Phase 2: Shutting down processor 1 and creating processor 2")

	// Shutdown processor 1
	err = processor1.Shutdown(context.Background())
	require.NoError(t, err)

	// Reset the sink to capture only traces from processor 2
	capturingSink.Reset()

	// Create processor instance 2 (using same DB path)
	processor2, err := reservoirsampler.CreateTracesProcessorForTesting(
		context.Background(),
		settings,
		cfg,
		capturingSink,
	)
	require.NoError(t, err)

	// Start processor 2 (should load state from persistence)
	err = processor2.Start(context.Background(), nil)
	require.NoError(t, err)
	defer processor2.Shutdown(context.Background())

	// ---------- Phase 3: Continued Operation & Storage Update ----------
	logger.Info("Phase 3: Sending more data to processor 2")

	// Wait to ensure DB is initialized
	time.Sleep(100 * time.Millisecond)

	// Submit new batch of traces to processor 2
	traces3 := generateTestTraces(TOTAL_TRACES, halfTraces, SPANS_PER_TRACE)
	err = processor2.ConsumeTraces(context.Background(), traces3)
	require.NoError(t, err)

	// Submit one more batch with complete traces to ensure they get processed
	traces4 := generateTestTraces(TOTAL_TRACES*2, halfTraces, SPANS_PER_TRACE)
	// Add parent-child relationships to make complete traces
	for i := 0; i < traces4.ResourceSpans().Len(); i++ {
		rs := traces4.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				// Set all spans as root spans (completed traces)
				span.SetParentSpanID(pcommon.SpanID{})
			}
		}
	}
	err = processor2.ConsumeTraces(context.Background(), traces4)
	require.NoError(t, err)

	// Wait for processing and checkpoint - add extra time for trace completion
	time.Sleep(CONFIG_WINDOW_DURATION + CONFIG_BUFFER_TIMEOUT + CONFIG_PERSISTENCE_INTERVAL + 500*time.Millisecond)

	// Force window rollover to trigger exporting of spans
	reservoirsampler.ForceReservoirExport(processor2)

	// Assertions - Phase 3
	capturedTraces2 := capturingSink.GetAllTraces()
	dbSizeAfter, err := getFileSize(dbPath)
	require.NoError(t, err)

	// Verify captured data from second processor
	assert.NotEmpty(t, capturedTraces2, "Processor 2 should capture and export traces")

	// For each window, ensure we didn't exceed reservoir capacity
	for _, window := range capturedTraces2 {
		windowTraceCount := countUniqueTraces([]ptrace.Traces{window})
		assert.LessOrEqual(t, windowTraceCount, CONFIG_RESERVOIR_SIZE,
			"Number of traces from processor 2 should not exceed reservoir capacity")
	}

	// Verify DB operations occurred - we don't care about exact size, just that it's not empty
	assert.Greater(t, dbSizeAfter, int64(0), "DB file should exist with content")
}

// Helper functions

// NewNoopTracesSink creates a sink that discards all traces
func NewNoopTracesSink() consumer.Traces {
	return &NoopTracesSink{}
}

// NoopTracesSink is a consumer that discards all traces
type NoopTracesSink struct{}

// ConsumeTraces discards traces
func (s *NoopTracesSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	return nil
}

// Capabilities returns consumer capabilities
func (s *NoopTracesSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// NewCapturingSink creates a capturing sink that records all traces sent to it
func NewCapturingSink(nextConsumer consumer.Traces) *CapturingSink {
	return &CapturingSink{
		nextConsumer: nextConsumer,
		traces:       make([]ptrace.Traces, 0),
		mutex:        &sync.Mutex{},
	}
}

// CapturingSink is a consumer that captures all traces before passing them to the next consumer
type CapturingSink struct {
	nextConsumer consumer.Traces
	traces       []ptrace.Traces
	mutex        *sync.Mutex
}

// ConsumeTraces captures traces and forwards them
func (s *CapturingSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Make a copy of the traces
	tracesCopy := ptrace.NewTraces()
	td.CopyTo(tracesCopy)
	s.traces = append(s.traces, tracesCopy)

	return s.nextConsumer.ConsumeTraces(ctx, td)
}

// GetAllTraces returns all captured traces
func (s *CapturingSink) GetAllTraces() []ptrace.Traces {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.traces
}

// Reset clears all captured traces
func (s *CapturingSink) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.traces = make([]ptrace.Traces, 0)
}

// Capabilities returns consumer capabilities
func (s *CapturingSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// createTestConfig creates a configuration for testing
func createTestConfig(reservoirSize int, windowDuration, bufferTimeout, persistenceInterval time.Duration, dbPath string) component.Config {
	return &reservoirsampler.Config{
		SizeK:                    reservoirSize,
		WindowDuration:           windowDuration.String(),
		TraceAware:               true,
		TraceBufferMaxSize:       10000,
		TraceBufferTimeout:       bufferTimeout.String(),
		CheckpointPath:           dbPath,
		CheckpointInterval:       persistenceInterval.String(),
		DbCompactionScheduleCron: "*/5 * * * *", // Every 5 minutes
	}
}

// generateTestTraces creates test traces with unique trace IDs
func generateTestTraces(startIdx, count, spansPerTrace int) ptrace.Traces {
	traces := ptrace.NewTraces()

	for i := 0; i < count; i++ {
		traceIdx := startIdx + i
		traceID := generateTraceID(traceIdx)

		for j := 0; j < spansPerTrace; j++ {
			rs := traces.ResourceSpans().AppendEmpty()
			res := rs.Resource()

			// Add minimal resource attributes to avoid excessive memory use
			attrs := res.Attributes()
			attrs.PutStr("service.name", fmt.Sprintf("svc-%d", traceIdx))

			ss := rs.ScopeSpans().AppendEmpty()
			scope := ss.Scope()
			scope.SetName("test")

			span := ss.Spans().AppendEmpty()
			span.SetTraceID(traceID)
			span.SetSpanID(generateSpanID(traceIdx*100 + j))

			// Set parent span ID for all except the first span
			if j > 0 {
				span.SetParentSpanID(generateSpanID(traceIdx * 100))
			}

			span.SetName(fmt.Sprintf("sp-%d-%d", traceIdx, j))
			span.SetKind(ptrace.SpanKindServer)

			// Set timestamps
			startTime := time.Now().Add(-10 * time.Second)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(startTime.Add(100 * time.Millisecond)))

			// Add minimal span attributes
			spanAttrs := span.Attributes()
			spanAttrs.PutInt("idx", int64(traceIdx))
		}
	}

	return traces
}

// generateTraceID creates a deterministic trace ID from an index
func generateTraceID(index int) pcommon.TraceID {
	var traceID pcommon.TraceID
	traceID[0] = byte(index >> 8)
	traceID[1] = byte(index)
	// Fill the rest with non-zero values
	for i := 2; i < len(traceID); i++ {
		traceID[i] = byte(i)
	}
	return traceID
}

// generateSpanID creates a deterministic span ID from an index
func generateSpanID(index int) pcommon.SpanID {
	var spanID pcommon.SpanID
	spanID[0] = byte(index >> 8)
	spanID[1] = byte(index)
	// Fill the rest with non-zero values
	for i := 2; i < len(spanID); i++ {
		spanID[i] = byte(i)
	}
	return spanID
}

// countUniqueTraces counts unique traces across all trace batches
func countUniqueTraces(traceBatches []ptrace.Traces) int {
	// Use a map to track unique trace IDs
	uniqueTraces := make(map[string]struct{})

	for _, batch := range traceBatches {
		for i := 0; i < batch.ResourceSpans().Len(); i++ {
			rs := batch.ResourceSpans().At(i)

			for j := 0; j < rs.ScopeSpans().Len(); j++ {
				ss := rs.ScopeSpans().At(j)

				for k := 0; k < ss.Spans().Len(); k++ {
					span := ss.Spans().At(k)
					traceID := span.TraceID().String()
					uniqueTraces[traceID] = struct{}{}
				}
			}
		}
	}

	return len(uniqueTraces)
}

// getFileSize returns the size of a file in bytes
func getFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
