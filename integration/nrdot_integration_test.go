// Integration tests for NR-DOT integration
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/integration/nrdot"
	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

// mockNext is a mock consumer that records received traces
type mockNext struct {
	traces []ptrace.Traces
}

func (m *mockNext) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	m.traces = append(m.traces, td)
	return nil
}

func (m *mockNext) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// TestEndToEndReservoirWithNRDOT tests the end-to-end integration with NR-DOT
func TestEndToEndReservoirWithNRDOT(t *testing.T) {
	// Create a logger for testing
	logger, _ := zap.NewDevelopment()
	
	// Create a temporary checkpoint directory
	tempDir, err := os.MkdirTemp("", "reservoir-checkpoint")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)
	
	// Create the checkpoint file path
	checkpointPath := filepath.Join(tempDir, "reservoir.db")
	
	// Create the NR-DOT integration
	integration := nrdot.NewIntegration(logger)
	
	// Create a configuration with a small reservoir for testing
	cfg := integration.CreateDefaultConfig()
	cfg.SizeK = 10 // Small reservoir for testing
	cfg.WindowDuration = "1s" // Short window for testing
	cfg.CheckpointPath = checkpointPath
	cfg.CheckpointInterval = "1s"
	cfg.TraceAware = true
	cfg.TraceBufferMaxSize = 1000
	cfg.TraceBufferTimeout = "1s"
	
	// Create a mock trace consumer
	nextConsumer := &mockNext{traces: make([]ptrace.Traces, 0)}
	
	// Create the processor settings
	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: logger,
		},
	}
	
	// Create the reservoir processor
	proc, err := reservoirsampler.CreateTracesProcessorForTesting(
		context.Background(),
		settings,
		cfg,
		nextConsumer,
	)
	require.NoError(t, err, "Failed to create processor")
	
	// Start the processor
	err = proc.Start(context.Background(), nil)
	require.NoError(t, err, "Failed to start processor")
	
	// Create test traces with different trace IDs
	numTraces := 20 // More traces than reservoir size
	
	for i := 0; i < numTraces; i++ {
		traces := createTestTrace(i)
		err = proc.ConsumeTraces(context.Background(), traces)
		require.NoError(t, err, "Failed to consume traces")
		
		// Small delay to ensure traces are processed
		time.Sleep(10 * time.Millisecond)
	}
	
	// Force trace buffer flush and window rollover
	time.Sleep(1500 * time.Millisecond)
	
	// Verify the processor is working
	// We should have some traces in the next consumer, but fewer than we sent
	// due to sampling
	assert.Greater(t, len(nextConsumer.traces), 0, "No traces were output")
	assert.LessOrEqual(t, len(nextConsumer.traces), numTraces, "Too many traces were output")
	
	// Shutdown the processor
	err = proc.Shutdown(context.Background())
	require.NoError(t, err, "Failed to shutdown processor")
	
	// Verify the checkpoint file exists
	_, err = os.Stat(checkpointPath)
	assert.NoError(t, err, "Checkpoint file doesn't exist")
	
	// Create a new processor to verify checkpoint restoration
	nextConsumer2 := &mockNext{traces: make([]ptrace.Traces, 0)}
	proc2, err := reservoirsampler.CreateTracesProcessorForTesting(
		context.Background(),
		settings,
		cfg,
		nextConsumer2,
	)
	require.NoError(t, err, "Failed to create second processor")
	
	// Start the new processor
	err = proc2.Start(context.Background(), nil)
	require.NoError(t, err, "Failed to start second processor")
	
	// Force export
	reservoirsampler.ForceReservoirExport(proc2)
	
	// Allow time for export
	time.Sleep(100 * time.Millisecond)
	
	// Verify traces were restored from checkpoint
	assert.Greater(t, len(nextConsumer2.traces), 0, "No traces were restored from checkpoint")
	
	// Shutdown the second processor
	err = proc2.Shutdown(context.Background())
	require.NoError(t, err, "Failed to shutdown second processor")
}

// createTestTrace creates a test trace with a unique ID
func createTestTrace(index int) ptrace.Traces {
	traces := ptrace.NewTraces()
	
	// Create a resource span
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	
	// Add a resource attribute
	resourceSpans.Resource().Attributes().PutStr("service.name", "test-service")
	
	// Add a scope span
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scopeSpans.Scope().SetName("test-scope")
	
	// Create root span
	span := scopeSpans.Spans().AppendEmpty()
	
	// Create a unique trace ID based on the index
	traceID := [16]byte{}
	traceID[15] = byte(index)
	traceID[14] = byte(index >> 8)
	span.SetTraceID(pcommon.TraceID(traceID))
	
	// Create a unique span ID
	spanID := [8]byte{}
	spanID[7] = byte(index)
	span.SetSpanID(pcommon.SpanID(spanID))
	
	// Set span name and attributes
	span.SetName("test-span")
	span.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	span.SetEndTimestamp(pcommon.Timestamp(time.Now().Add(100 * time.Millisecond).UnixNano()))
	
	// Add a child span for completeness
	childSpan := scopeSpans.Spans().AppendEmpty()
	childSpanID := [8]byte{}
	childSpanID[7] = byte(index + 100)
	childSpan.SetTraceID(pcommon.TraceID(traceID))
	childSpan.SetSpanID(pcommon.SpanID(childSpanID))
	childSpan.SetParentSpanID(pcommon.SpanID(spanID))
	childSpan.SetName("child-span")
	childSpan.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	childSpan.SetEndTimestamp(pcommon.Timestamp(time.Now().Add(50 * time.Millisecond).UnixNano()))
	
	return traces
}