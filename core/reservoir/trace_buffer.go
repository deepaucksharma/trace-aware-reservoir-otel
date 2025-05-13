package reservoir

import (
	"container/list"
	"sync"
	"time"
)

// TraceBuffer implements trace-aware buffering for the reservoir sampler
type TraceBuffer struct {
	maxSize       int
	timeout       time.Duration
	traces        map[string]*traceData
	traceQueue    *list.List
	mu            sync.RWMutex
	metrics       MetricsReporter
	evictCallback func(string)
}

type traceData struct {
	spans       []SpanData
	lastUpdated time.Time
	element     *list.Element
}

// NewTraceBuffer creates a new trace buffer
func NewTraceBuffer(maxSize int, timeout time.Duration, metrics MetricsReporter) *TraceBuffer {
	return &TraceBuffer{
		maxSize:    maxSize,
		timeout:    timeout,
		traces:     make(map[string]*traceData),
		traceQueue: list.New(),
		metrics:    metrics,
	}
}

// SetEvictionCallback sets the function to call when a trace is evicted
func (tb *TraceBuffer) SetEvictionCallback(callback func(traceID string)) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	tb.evictCallback = callback
}

// AddSpan adds a span to the trace buffer
func (tb *TraceBuffer) AddSpan(span SpanData) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	// Get or create the trace entry
	trace, exists := tb.traces[span.TraceID]
	if !exists {
		// If we've reached capacity, evict the oldest trace
		if len(tb.traces) >= tb.maxSize {
			tb.evictOldestLocked()
		}
		
		// Create a new trace entry
		trace = &traceData{
			spans:       make([]SpanData, 0, 8), // Start with a small capacity
			lastUpdated: time.Now(),
		}
		
		// Add to the queue and map
		trace.element = tb.traceQueue.PushBack(span.TraceID)
		tb.traces[span.TraceID] = trace
	} else {
		// Update the timestamp and move to the back of the queue
		trace.lastUpdated = time.Now()
		tb.traceQueue.MoveToBack(trace.element)
	}
	
	// Add the span to the trace
	trace.spans = append(trace.spans, span)
	
	// Update metrics
	if tb.metrics != nil {
		tb.metrics.ReportTraceBufferSize(len(tb.traces))
	}
}

// GetCompletedTraces returns traces that are considered complete based on timeout
func (tb *TraceBuffer) GetCompletedTraces() [][]SpanData {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	now := time.Now()
	var completed [][]SpanData
	var toRemove []string
	
	// Find all traces that have timed out
	for traceID, trace := range tb.traces {
		if now.Sub(trace.lastUpdated) > tb.timeout {
			// Add to the completed list
			completed = append(completed, trace.spans)
			
			// Mark for removal
			toRemove = append(toRemove, traceID)
			
			// Remove from the queue
			tb.traceQueue.Remove(trace.element)
		}
	}
	
	// Remove all completed traces from the map
	for _, traceID := range toRemove {
		delete(tb.traces, traceID)
	}
	
	// Update metrics
	if tb.metrics != nil && len(toRemove) > 0 {
		tb.metrics.ReportTraceBufferSize(len(tb.traces))
	}
	
	return completed
}

// evictOldestLocked evicts the oldest trace from the buffer
// Must be called with the lock held
func (tb *TraceBuffer) evictOldestLocked() {
	// Get the oldest trace from the queue
	element := tb.traceQueue.Front()
	if element == nil {
		return // Empty queue
	}
	
	// Remove it from the queue
	traceID := element.Value.(string)
	tb.traceQueue.Remove(element)
	
	// Remove it from the map
	delete(tb.traces, traceID)
	
	// Call the eviction callback if set
	if tb.evictCallback != nil {
		tb.evictCallback(traceID)
	}
	
	// Update metrics
	if tb.metrics != nil {
		tb.metrics.ReportEvictions(1)
	}
}

// Size returns the current number of traces in the buffer
func (tb *TraceBuffer) Size() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	
	return len(tb.traces)
}

// SpanCount returns the total number of spans in the buffer
func (tb *TraceBuffer) SpanCount() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	
	var count int
	for _, trace := range tb.traces {
		count += len(trace.spans)
	}
	
	return count
}
