package reservoirsampler

import (
	"container/list"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// traceElement represents a trace in the trace buffer
type traceElement struct {
	// Maps span IDs to spans
	spans map[pcommon.SpanID]SpanWithResource
	
	// Last time a span was added to this trace
	lastUpdated time.Time
	
	// List element for LRU eviction
	element *list.Element
	
	// Trace completion information
	rootSpanSeen bool
	spanCount    int
	expectedSpans int // Might be available from trace context
}

// TraceBuffer holds spans grouped by trace ID for trace-aware sampling.
// It maintains an in-memory buffer of spans organized by trace ID, with efficient
// LRU eviction when the buffer reaches its maximum size.
type TraceBuffer struct {
	// Maps trace IDs to all spans in that trace
	traces map[pcommon.TraceID]*traceElement
	
	// LRU list for eviction
	lruList *list.List
	
	// Maximum number of traces to buffer
	maxTraces int
	
	// How long to wait for a trace to complete
	timeout time.Duration
	
	// Logger for debug and error output
	logger *zap.Logger
	
	// Counter for trace evictions
	evictionCounter *atomic.Int64
	
	// Total span count for metrics
	spanCount atomic.Int64
	
	// Mutex for thread safety
	mu sync.RWMutex
}

// NewTraceBuffer creates a new trace buffer with the specified size and timeout
func NewTraceBuffer(maxTraces int, timeout time.Duration, logger *zap.Logger) *TraceBuffer {
	return &TraceBuffer{
		traces:         make(map[pcommon.TraceID]*traceElement, maxTraces),
		lruList:        list.New(),
		maxTraces:      maxTraces,
		timeout:        timeout,
		logger:         logger,
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
	
	now := time.Now()
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	// Get or create trace element
	traceElem, exists := tb.traces[traceID]
	if !exists {
		// Create new trace element if this is a new trace
		traceElem = &traceElement{
			spans:       make(map[pcommon.SpanID]SpanWithResource),
			lastUpdated: now,
			rootSpanSeen: span.ParentSpanID().IsEmpty(),
			spanCount:   0,
			expectedSpans: 0, // Will be updated if available in attributes
		}
		
		// Add to LRU list
		traceElem.element = tb.lruList.PushFront(traceID)
		
		// If buffer is full, evict the least recently used trace
		if len(tb.traces) >= tb.maxTraces {
			tb.evictLRUTrace()
		}
		
		// Add to traces map
		tb.traces[traceID] = traceElem
	} else {
		// Move to front of LRU list (most recently used)
		tb.lruList.MoveToFront(traceElem.element)
		
		// Update last updated time
		traceElem.lastUpdated = now
		
		// Update root span seen flag
		if !traceElem.rootSpanSeen && span.ParentSpanID().IsEmpty() {
			traceElem.rootSpanSeen = true
		}
	}
	
	// Get a SpanWithResource from the pool
	spanWithRes := GetSpanWithResource()
	
	// Fill the SpanWithResource
	FillSpanWithResource(spanWithRes, span, resource, scope)
	
	// Store the span
	traceElem.spans[spanID] = *spanWithRes
	
	// Return to the pool
	PutSpanWithResource(spanWithRes)
	
	// Update span counts
	traceElem.spanCount++
	tb.spanCount.Inc()
}

// GetCompletedTraces returns all traces that are considered complete and removes them from the buffer
func (tb *TraceBuffer) GetCompletedTraces() []ptrace.Traces {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	var completedTraces []ptrace.Traces
	now := time.Now()
	tracesToRemove := make([]pcommon.TraceID, 0)
	
	// Find traces that have timed out
	for traceID, traceElem := range tb.traces {
		if now.Sub(traceElem.lastUpdated) >= tb.timeout {
			// Create a new traces collection for this trace
			traces := ptrace.NewTraces()
			
			// Get span count for logging
			spanCount := len(traceElem.spans)
			
			// Add all spans from this trace
			for _, spanWithRes := range traceElem.spans {
				insertSpanIntoTraces(traces, spanWithRes)
			}
			
			completedTraces = append(completedTraces, traces)
			tracesToRemove = append(tracesToRemove, traceID)
			
			tb.logger.Debug("Trace completed by timeout",
				zap.Stringer("trace_id", traceID),
				zap.Int("span_count", spanCount),
				zap.Duration("age", now.Sub(traceElem.lastUpdated)))
			
			// Update span count
			tb.spanCount.Add(-int64(spanCount))
		}
	}
	
	// Remove completed traces from the buffer
	for _, traceID := range tracesToRemove {
		tb.removeTraceLocked(traceID)
	}
	
	return completedTraces
}

// removeTraceLocked removes a trace from the buffer (must be called with lock held)
func (tb *TraceBuffer) removeTraceLocked(traceID pcommon.TraceID) {
	if traceElem, exists := tb.traces[traceID]; exists {
		// Remove from LRU list
		if traceElem.element != nil {
			tb.lruList.Remove(traceElem.element)
		}
		
		// Remove from traces map
		delete(tb.traces, traceID)
	}
}

// evictLRUTrace evicts the least recently used trace from the buffer (must be called with lock held)
func (tb *TraceBuffer) evictLRUTrace() {
	// Get the last element from the LRU list
	if tb.lruList.Len() == 0 {
		return
	}
	
	// Get the trace ID from the list element
	elem := tb.lruList.Back()
	traceID, ok := elem.Value.(pcommon.TraceID)
	if !ok {
		tb.logger.Error("Invalid element type in LRU list")
		return
	}
	
	// Get the trace element
	traceElem, exists := tb.traces[traceID]
	if !exists {
		tb.logger.Error("Trace not found in buffer",
			zap.Stringer("trace_id", traceID))
		return
	}
	
	// Log the eviction
	tb.logger.Debug("Evicting trace from buffer due to capacity limit",
		zap.Stringer("trace_id", traceID),
		zap.Time("last_updated", traceElem.lastUpdated),
		zap.Int("spans", len(traceElem.spans)),
		zap.Duration("age", time.Since(traceElem.lastUpdated)))
	
	// Increment the eviction counter if set
	if tb.evictionCounter != nil {
		tb.evictionCounter.Inc()
	}
	
	// Update span count
	tb.spanCount.Add(-int64(len(traceElem.spans)))
	
	// Remove the trace from the buffer
	tb.removeTraceLocked(traceID)
}

// GetTrace returns a specific trace as a Traces object
func (tb *TraceBuffer) GetTrace(traceID pcommon.TraceID) ptrace.Traces {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	
	if traceElem, exists := tb.traces[traceID]; exists {
		traces := ptrace.NewTraces()
		for _, spanWithRes := range traceElem.spans {
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
	
	if traceElem, exists := tb.traces[traceID]; exists {
		// Update span count
		tb.spanCount.Add(-int64(len(traceElem.spans)))
		
		// Remove the trace
		tb.removeTraceLocked(traceID)
	}
}

// Size returns the number of traces in the buffer
func (tb *TraceBuffer) Size() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.traces)
}

// SpanCount returns the total number of spans across all traces
func (tb *TraceBuffer) SpanCount() int {
	return int(tb.spanCount.Load())
}