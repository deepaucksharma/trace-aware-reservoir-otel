package reservoirsampler

import (
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// TraceBuffer holds spans grouped by trace ID for trace-aware sampling.
// It maintains an in-memory buffer of spans organized by trace ID, with eviction
// policies based on trace age and a maximum buffer size.
type TraceBuffer struct {
	// Maps trace IDs to all spans in that trace
	traces map[pcommon.TraceID]map[pcommon.SpanID]SpanWithResource

	// Maps trace IDs to the last time a span from that trace was received
	lastUpdated map[pcommon.TraceID]time.Time

	// Maximum number of traces to buffer
	maxTraces int

	// How long to wait for a trace to complete
	timeout time.Duration

	// Logger for debug and error output
	logger *zap.Logger

	// Counter for trace evictions (optional)
	evictionCounter *atomic.Int64

	// Mutex for thread safety
	mu sync.RWMutex
}

// NewTraceBuffer creates a new trace buffer with the specified size and timeout
func NewTraceBuffer(maxTraces int, timeout time.Duration, logger *zap.Logger) *TraceBuffer {
	return &TraceBuffer{
		traces:          make(map[pcommon.TraceID]map[pcommon.SpanID]SpanWithResource),
		lastUpdated:     make(map[pcommon.TraceID]time.Time),
		maxTraces:       maxTraces,
		timeout:         timeout,
		logger:          logger,
		evictionCounter: nil, // Set externally if needed
	}
}

// SetEvictionCounter sets the counter for trace evictions
func (tb *TraceBuffer) SetEvictionCounter(counter *atomic.Int64) {
	tb.evictionCounter = counter
}

// AddSpan adds a span to the trace buffer
func (tb *TraceBuffer) AddSpan(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) {
	traceID := span.TraceID()
	spanID := span.SpanID()

	if traceID.IsEmpty() || spanID.IsEmpty() {
		// Skip spans with invalid IDs
		return
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Create trace entry if it doesn't exist
	if _, exists := tb.traces[traceID]; !exists {
		// If buffer is full, evict the oldest trace
		if len(tb.traces) >= tb.maxTraces {
			tb.evictOldestTrace()
		}
		tb.traces[traceID] = make(map[pcommon.SpanID]SpanWithResource)
	}

	// Store the span with its context
	tb.traces[traceID][spanID] = cloneSpanWithContext(span, resource, scope)

	// Update last update time
	tb.lastUpdated[traceID] = time.Now()
}

// GetCompletedTraces returns all traces that are considered complete and removes them from the buffer
func (tb *TraceBuffer) GetCompletedTraces() []ptrace.Traces {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	var completedTraces []ptrace.Traces
	now := time.Now()
	tracesToRemove := make([]pcommon.TraceID, 0)

	// Find traces that have timed out
	for traceID, lastUpdate := range tb.lastUpdated {
		if now.Sub(lastUpdate) >= tb.timeout {
			// Create a new traces collection for this trace
			traces := ptrace.NewTraces()

			// Add all spans from this trace
			for _, spanWithRes := range tb.traces[traceID] {
				insertSpanIntoTraces(traces, spanWithRes)
			}

			completedTraces = append(completedTraces, traces)
			tracesToRemove = append(tracesToRemove, traceID)
		}
	}

	// Remove completed traces from the buffer
	for _, traceID := range tracesToRemove {
		delete(tb.traces, traceID)
		delete(tb.lastUpdated, traceID)
	}

	return completedTraces
}

// GetTrace returns a specific trace as a Traces object
func (tb *TraceBuffer) GetTrace(traceID pcommon.TraceID) ptrace.Traces {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if spans, exists := tb.traces[traceID]; exists {
		traces := ptrace.NewTraces()
		for _, spanWithRes := range spans {
			insertSpanIntoTraces(traces, spanWithRes)
		}
		return traces
	}

	return ptrace.NewTraces()
}

// RemoveTrace removes a trace from the buffer
func (tb *TraceBuffer) RemoveTrace(traceID pcommon.TraceID) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	delete(tb.traces, traceID)
	delete(tb.lastUpdated, traceID)
}

// Size returns the number of traces in the buffer
func (tb *TraceBuffer) Size() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.traces)
}

// SpanCount returns the total number of spans across all traces
func (tb *TraceBuffer) SpanCount() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	count := 0
	for _, spans := range tb.traces {
		count += len(spans)
	}
	return count
}

// evictOldestTrace removes the trace with the oldest last update time
func (tb *TraceBuffer) evictOldestTrace() {
	var oldestTraceID pcommon.TraceID
	var oldestTime time.Time

	// Find the oldest trace
	first := true
	for traceID, updateTime := range tb.lastUpdated {
		if first || updateTime.Before(oldestTime) {
			oldestTraceID = traceID
			oldestTime = updateTime
			first = false
		}
	}

	// Remove the oldest trace
	if !first {
		// Count how many spans were in the trace
		evictedSpans := 0
		if spans, ok := tb.traces[oldestTraceID]; ok {
			evictedSpans = len(spans)
		}

		// Log the eviction
		tb.logger.Debug("Evicting trace from buffer due to capacity limit",
			zap.Stringer("trace_id", oldestTraceID),
			zap.Time("last_updated", oldestTime),
			zap.Int("spans", evictedSpans),
			zap.Duration("age", time.Since(oldestTime)))

		// Increment the eviction counter if set
		if tb.evictionCounter != nil {
			tb.evictionCounter.Inc()
		}

		// Remove the trace
		delete(tb.traces, oldestTraceID)
		delete(tb.lastUpdated, oldestTraceID)
	}
}
