package reservoirsampler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func TestTraceBufferAddAndGet(t *testing.T) {
	logger := zap.NewNop()
	tb := NewTraceBuffer(10, 100*time.Millisecond, logger)
	
	// Create a trace
	traceID := generateTraceID(1)
	spanID1 := generateSpanID(1)
	spanID2 := generateSpanID(2)
	
	// Create resources and scopes
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "test-service")
	
	scope := pcommon.NewInstrumentationScope()
	scope.SetName("test-scope")
	
	// Create span 1
	span1 := ptrace.NewSpan()
	span1.SetName("span-1")
	span1.SetTraceID(traceID)
	span1.SetSpanID(spanID1)
	
	// Create span 2 (same trace)
	span2 := ptrace.NewSpan()
	span2.SetName("span-2")
	span2.SetTraceID(traceID)
	span2.SetSpanID(spanID2)
	span2.SetParentSpanID(spanID1) // Child of span 1
	
	// Add spans to the buffer
	tb.AddSpan(span1, resource, scope)
	tb.AddSpan(span2, resource, scope)
	
	// Verify they're in the buffer
	assert.Equal(t, 1, tb.Size(), "Buffer should contain 1 trace")
	assert.Equal(t, 2, tb.SpanCount(), "Buffer should contain 2 spans")
	
	// Get the trace
	trace := tb.GetTrace(traceID)
	
	// Verify trace structure
	assert.Equal(t, 2, trace.SpanCount(), "Trace should contain 2 spans")
	
	// Remove the trace
	tb.RemoveTrace(traceID)
	
	// Verify trace was removed
	assert.Equal(t, 0, tb.Size(), "Buffer should be empty after removal")
}

func TestTraceBufferCompletedTraces(t *testing.T) {
	logger := zap.NewNop()
	tb := NewTraceBuffer(10, 100*time.Millisecond, logger)
	
	// Create two traces
	traceID1 := generateTraceID(1)
	traceID2 := generateTraceID(2)
	
	// Create resources and scopes
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "test-service")
	
	scope := pcommon.NewInstrumentationScope()
	scope.SetName("test-scope")
	
	// Create spans for trace 1
	span1 := ptrace.NewSpan()
	span1.SetName("trace1-span-1")
	span1.SetTraceID(traceID1)
	span1.SetSpanID(generateSpanID(1))
	
	// Create spans for trace 2
	span2 := ptrace.NewSpan()
	span2.SetName("trace2-span-1")
	span2.SetTraceID(traceID2)
	span2.SetSpanID(generateSpanID(2))
	
	// Add spans to the buffer
	tb.AddSpan(span1, resource, scope)
	tb.AddSpan(span2, resource, scope)
	
	// Verify they're in the buffer
	assert.Equal(t, 2, tb.Size(), "Buffer should contain 2 traces")
	
	// Wait for traces to complete
	time.Sleep(150 * time.Millisecond)
	
	// Get completed traces
	completedTraces := tb.GetCompletedTraces()
	assert.Equal(t, 2, len(completedTraces), "Should have 2 completed traces")
	
	// Verify traces were removed from buffer
	assert.Equal(t, 0, tb.Size(), "Buffer should be empty after getting completed traces")
	
	// Verify trace contents
	// Each trace should have 1 span
	assert.Equal(t, 1, completedTraces[0].SpanCount(), "First completed trace should have 1 span")
	assert.Equal(t, 1, completedTraces[1].SpanCount(), "Second completed trace should have 1 span")
}

func TestTraceBufferEviction(t *testing.T) {
	logger := zap.NewNop()
	maxTraces := 5
	tb := NewTraceBuffer(maxTraces, 100*time.Millisecond, logger)
	
	// Create resources and scopes
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "test-service")
	
	scope := pcommon.NewInstrumentationScope()
	scope.SetName("test-scope")
	
	// Add more traces than the buffer capacity
	for i := 0; i < maxTraces+3; i++ {
		traceID := generateTraceID(i)
		
		span := ptrace.NewSpan()
		span.SetName(fmt.Sprintf("span-%d", i))
		span.SetTraceID(traceID)
		span.SetSpanID(generateSpanID(i))
		
		tb.AddSpan(span, resource, scope)
		
		// Small delay to ensure different lastUpdated times
		time.Sleep(1 * time.Millisecond)
	}
	
	// Verify buffer size is limited
	assert.Equal(t, maxTraces, tb.Size(), "Buffer size should be limited to maxTraces")
	
	// Wait for traces to complete
	time.Sleep(150 * time.Millisecond)
	
	// Get completed traces
	completedTraces := tb.GetCompletedTraces()
	assert.Equal(t, maxTraces, len(completedTraces), "Should have maxTraces completed traces")
	
	// Verify traces were removed from buffer
	assert.Equal(t, 0, tb.Size(), "Buffer should be empty after getting completed traces")
}

func TestTraceBufferConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	tb := NewTraceBuffer(100, 100*time.Millisecond, logger)
	
	// Create resources and scopes
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "test-service")
	
	scope := pcommon.NewInstrumentationScope()
	scope.SetName("test-scope")
	
	// Add spans concurrently
	numGoroutines := 10
	numSpansPerGoroutine := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			
			for i := 0; i < numSpansPerGoroutine; i++ {
				traceID := generateTraceID(goroutineID)
				
				span := ptrace.NewSpan()
				span.SetName(fmt.Sprintf("span-%d-%d", goroutineID, i))
				span.SetTraceID(traceID)
				span.SetSpanID(generateSpanID(goroutineID*100 + i))
				
				tb.AddSpan(span, resource, scope)
			}
		}(g)
	}
	
	// Wait for all goroutines to complete
	wg.Wait()
	
	// Verify trace buffer state
	assert.Equal(t, numGoroutines, tb.Size(), "Buffer should contain the correct number of traces")
	assert.Equal(t, numGoroutines*numSpansPerGoroutine, tb.SpanCount(), "Buffer should contain the correct number of spans")
	
	// Wait for traces to complete
	time.Sleep(150 * time.Millisecond)
	
	// Get completed traces
	completedTraces := tb.GetCompletedTraces()
	assert.Equal(t, numGoroutines, len(completedTraces), "Should have correct number of completed traces")
	
	// Verify buffer is empty
	assert.Equal(t, 0, tb.Size(), "Buffer should be empty after getting completed traces")
}