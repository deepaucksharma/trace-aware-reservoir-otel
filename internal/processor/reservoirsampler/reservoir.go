package reservoirsampler

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// Reservoir implements a thread-safe reservoir sampling algorithm
type Reservoir struct {
	// Store spans in a map keyed by hash for faster lookups
	spanMap  map[uint64]SpanWithResource
	spanKeys []uint64
	
	// Configuration
	size   int
	window *WindowManager
	
	// Thread safety
	mu        sync.RWMutex
	randomMu  sync.Mutex
	random    *rand.Rand
	
	// Metrics
	sizeGauge       *atomic.Int64
	sampledCounter  *atomic.Int64
	
	// Logging
	logger *zap.Logger
}

// spanWithResourcePool is an object pool for SpanWithResource to reduce GC pressure
var spanWithResourcePool = sync.Pool{
	New: func() interface{} {
		return &SpanWithResource{}
	},
}

// NewReservoir creates a new reservoir with the given size
func NewReservoir(
	size int, 
	window *WindowManager, 
	sizeGauge, sampledCounter *atomic.Int64, 
	logger *zap.Logger,
) *Reservoir {
	// Use a cryptographically secure seed for the random number generator
	seed := time.Now().UnixNano()
	source := rand.NewSource(seed)
	
	return &Reservoir{
		spanMap:        make(map[uint64]SpanWithResource, size),
		spanKeys:       make([]uint64, 0, size),
		size:           size,
		window:         window,
		random:         rand.New(source),
		sizeGauge:      sizeGauge,
		sampledCounter: sampledCounter,
		logger:         logger,
	}
}

// Reset clears the reservoir for a new window
func (r *Reservoir) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.spanMap = make(map[uint64]SpanWithResource, r.size)
	r.spanKeys = make([]uint64, 0, r.size)
	
	// Update metrics
	r.sizeGauge.Store(0)
}

// AddSpan adds a span to the reservoir using reservoir sampling algorithm
//
// This implements Algorithm R (Jeffrey Vitter):
//  1. If we have seen fewer than k elements, add the element to our reservoir
//  2. Otherwise, with probability k/n, keep the new element
//     where:
//     n = the number of elements we have seen so far
//     k = the size of our reservoir
func (r *Reservoir) AddSpan(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) {
	// Increment the total count for this window
	count := r.window.IncrementCount()
	
	// Create span key and hash
	key := createSpanKey(span)
	hash := hashSpanKey(key)
	
	// Acquire lock for modifying the reservoir
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if int(count) <= r.size {
		// Reservoir not full yet, add span directly
		r.addSpanToReservoirLocked(hash, span, resource, scope)
	} else {
		// Reservoir is full, use reservoir sampling algorithm
		// Generate a random index in [0, count)
		r.randomMu.Lock()
		j := r.random.Int63n(count)
		r.randomMu.Unlock()
		
		if j < int64(r.size) {
			// Replace the span at index j
			oldHash := r.spanKeys[j]
			delete(r.spanMap, oldHash)
			
			// Add the new span
			r.addSpanToReservoirLocked(hash, span, resource, scope)
			
			// Replace the key at index j
			r.spanKeys[j] = hash
		}
		// If j >= size, just skip this span
	}
	
	// Update metrics
	r.sizeGauge.Store(int64(len(r.spanMap)))
}

// addSpanToReservoirLocked adds a span to the reservoir (must be called with lock held)
func (r *Reservoir) addSpanToReservoirLocked(hash uint64, span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) {
	// Get a SpanWithResource from the pool
	spanWithRes := GetSpanWithResource()
	
	// Clone the span and its context
	FillSpanWithResource(spanWithRes, span, resource, scope)
	
	// Add to the reservoir
	r.spanMap[hash] = *spanWithRes
	r.spanKeys = append(r.spanKeys, hash)
	
	// Return the span to the pool (after copying to map)
	PutSpanWithResource(spanWithRes)
	
	// Increment the sampled span counter
	r.sampledCounter.Inc()
}

// Export returns all spans in the reservoir as traces
func (r *Reservoir) Export(ctx context.Context) (ptrace.Traces, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Create a new traces object to hold all spans
	exportTraces := ptrace.NewTraces()
	
	// Return empty traces if reservoir is empty
	if len(r.spanMap) == 0 {
		return exportTraces, nil
	}
	
	// Add all spans from the reservoir to the traces
	for _, hash := range r.spanKeys {
		if spanWithRes, ok := r.spanMap[hash]; ok {
			insertSpanIntoTraces(exportTraces, spanWithRes)
		}
	}
	
	return exportTraces, nil
}

// GetAllSpans returns a copy of all spans in the reservoir
func (r *Reservoir) GetAllSpans() map[uint64]SpanWithResource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Create a copy of the span map
	spansCopy := make(map[uint64]SpanWithResource, len(r.spanMap))
	for hash, spanWithRes := range r.spanMap {
		spansCopy[hash] = spanWithRes
	}
	
	return spansCopy
}

// Size returns the number of spans in the reservoir
func (r *Reservoir) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.spanMap)
}

// GetSpanWithResource gets a SpanWithResource from the pool
func GetSpanWithResource() *SpanWithResource {
	return spanWithResourcePool.Get().(*SpanWithResource)
}

// PutSpanWithResource returns a SpanWithResource to the pool
func PutSpanWithResource(s *SpanWithResource) {
	// Clear references to allow GC
	s.Span = ptrace.Span{}
	s.Resource = pcommon.Resource{}
	s.Scope = pcommon.InstrumentationScope{}
	spanWithResourcePool.Put(s)
}

// FillSpanWithResource fills a SpanWithResource with clones of the provided span, resource, and scope
func FillSpanWithResource(swr *SpanWithResource, span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) {
	// Create a new traces object to hold the cloned data
	traces := ptrace.NewTraces()
	
	// Add a resource span
	rs := traces.ResourceSpans().AppendEmpty()
	resource.CopyTo(rs.Resource())
	
	// Add a scope span
	ss := rs.ScopeSpans().AppendEmpty()
	scope.CopyTo(ss.Scope())
	
	// Add the span
	newSpan := ss.Spans().AppendEmpty()
	span.CopyTo(newSpan)
	
	// Fill the SpanWithResource
	swr.Span = newSpan
	swr.Resource = rs.Resource()
	swr.Scope = ss.Scope()
}