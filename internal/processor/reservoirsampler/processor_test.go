package reservoirsampler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.uber.org/zap"
)

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg, "failed to create default configuration")
	assert.NoError(t, component.ValidateConfig(cfg))
}

func TestCreateProcessor(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	
	// Need to set checkpoint path for validation to pass
	rcfg := cfg.(*Config)
	rcfg.CheckpointPath = t.TempDir() + "/test_checkpoint.db"
	
	ctx := context.Background()
	proc, err := factory.CreateTracesProcessor(
		ctx,
		processortest.NewNopCreateSettings(),
		cfg,
		consumertest.NewNop(),
	)
	require.NoError(t, err)
	require.NotNil(t, proc)
}

func TestConfigValidation(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	
	// Valid config
	cfg.SizeK = 100
	cfg.WindowDuration = "30s"
	cfg.CheckpointPath = "/tmp/checkpoint.db"
	cfg.CheckpointInterval = "5s"
	cfg.TraceAware = true
	cfg.TraceBufferMaxSize = 1000
	cfg.TraceBufferTimeout = "5s"
	
	assert.NoError(t, cfg.Validate())
	
	// Invalid config: negative size
	cfg.SizeK = -1
	assert.Error(t, cfg.Validate())
	cfg.SizeK = 100 // Reset
	
	// Invalid config: empty window duration
	cfg.WindowDuration = ""
	assert.Error(t, cfg.Validate())
	cfg.WindowDuration = "30s" // Reset
	
	// Invalid config: empty checkpoint path
	cfg.CheckpointPath = ""
	assert.Error(t, cfg.Validate())
	cfg.CheckpointPath = "/tmp/checkpoint.db" // Reset
	
	// Invalid config: empty checkpoint interval
	cfg.CheckpointInterval = ""
	assert.Error(t, cfg.Validate())
	cfg.CheckpointInterval = "5s" // Reset
	
	// Invalid config: trace-aware enabled but missing buffer configs
	cfg.TraceAware = true
	cfg.TraceBufferMaxSize = 0
	assert.Error(t, cfg.Validate())
	cfg.TraceBufferMaxSize = 1000 // Reset
	
	cfg.TraceBufferTimeout = ""
	assert.Error(t, cfg.Validate())
	cfg.TraceBufferTimeout = "5s" // Reset
}

func TestReservoirSampling(t *testing.T) {
	// Create processor with in-memory storage (no checkpoints)
	cfg := &Config{
		SizeK:               10,
		WindowDuration:      "10s",
		CheckpointPath:      "",
		CheckpointInterval:  "1s",
		TraceAware:          false,
	}
	
	sink := new(consumertest.TracesSink)
	set := processortest.NewNopCreateSettings()
	set.Logger = zap.NewNop()
	
	ctx := context.Background()
	proc, err := newReservoirProcessor(ctx, set, cfg, sink)
	require.NoError(t, err)
	
	// Start the processor
	err = proc.Start(ctx, nil)
	require.NoError(t, err)
	defer proc.Shutdown(ctx)
	
	// Create more spans than the reservoir size
	numSpans := 100
	traces := generateTraces(numSpans)
	
	// Process the traces
	err = proc.ConsumeTraces(ctx, traces)
	require.NoError(t, err)
	
	// Check if the reservoir size is limited to the configured size
	p, ok := proc.(*reservoirProcessor)
	require.True(t, ok)
	
	p.lock.RLock()
	reservoirSize := len(p.reservoir)
	windowCount := p.windowCount.Load()
	p.lock.RUnlock()
	
	assert.Equal(t, int64(numSpans), windowCount, "Window count should match the number of spans processed")
	assert.LessOrEqual(t, reservoirSize, cfg.SizeK, "Reservoir size should not exceed the configured limit")
}

func TestTraceAwareSampling(t *testing.T) {
	// Create processor with trace-aware sampling
	cfg := &Config{
		SizeK:               10,
		WindowDuration:      "10s",
		CheckpointPath:      "",
		CheckpointInterval:  "1s",
		TraceAware:          true,
		TraceBufferMaxSize:  100,
		TraceBufferTimeout:  "50ms", // Short timeout for testing
	}
	
	sink := new(consumertest.TracesSink)
	set := processortest.NewNopCreateSettings()
	set.Logger = zap.NewNop()
	
	ctx := context.Background()
	proc, err := newReservoirProcessor(ctx, set, cfg, sink)
	require.NoError(t, err)
	
	// Start the processor
	err = proc.Start(ctx, nil)
	require.NoError(t, err)
	defer proc.Shutdown(ctx)
	
	// Create some traces with shared trace IDs
	traces := generateTracesWithSharedIDs(20, 5) // 20 spans across 5 trace IDs
	
	// Process the traces
	err = proc.ConsumeTraces(ctx, traces)
	require.NoError(t, err)
	
	// Wait for the trace buffer to process
	time.Sleep(200 * time.Millisecond)
	
	// Check if traces were buffered
	p, ok := proc.(*reservoirProcessor)
	require.True(t, ok)
	
	// The trace buffer should now be empty as traces should have been processed
	count := p.traceBuffer.Size()
	assert.Equal(t, 0, count, "Trace buffer should be empty after processing")
}

// generateTraces creates test trace data with the specified number of spans
func generateTraces(numSpans int) ptrace.Traces {
	traces := ptrace.NewTraces()
	
	for i := 0; i < numSpans; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "test-service")
		
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("test-scope")
		
		span := ss.Spans().AppendEmpty()
		span.SetName("test-span")
		span.SetTraceID(generateTraceID(i))
		span.SetSpanID(generateSpanID(i))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-10 * time.Millisecond)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	}
	
	return traces
}

// generateTracesWithSharedIDs creates test trace data with shared trace IDs
func generateTracesWithSharedIDs(numSpans, numTraces int) ptrace.Traces {
	traces := ptrace.NewTraces()
	
	for i := 0; i < numSpans; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "test-service")
		
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("test-scope")
		
		span := ss.Spans().AppendEmpty()
		span.SetName("test-span")
		
		// Use modulo to create shared trace IDs
		traceIDIndex := i % numTraces
		span.SetTraceID(generateTraceID(traceIDIndex))
		span.SetSpanID(generateSpanID(i))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-10 * time.Millisecond)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	}
	
	return traces
}

// generateTraceID creates a trace ID from an integer
func generateTraceID(id int) pcommon.TraceID {
	var traceID [16]byte
	for i := 0; i < 16; i++ {
		traceID[i] = byte((id + i) % 256)
	}
	return pcommon.TraceID(traceID)
}

// generateSpanID creates a span ID from an integer
func generateSpanID(id int) pcommon.SpanID {
	var spanID [8]byte
	for i := 0; i < 8; i++ {
		spanID[i] = byte((id + i) % 256)
	}
	return pcommon.SpanID(spanID)
}