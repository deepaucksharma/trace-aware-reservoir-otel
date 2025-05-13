package reservoir

import (
	"math/rand"
	"sync"
	"time"
)

// AlgorithmR implements reservoir sampling using Algorithm R
type AlgorithmR struct {
	maxSize     int
	currentSize int
	rng         *rand.Rand
	spans       map[string]SpanData
	mu          sync.RWMutex
	metrics     MetricsReporter
}

// NewAlgorithmR creates a new reservoir sampler
func NewAlgorithmR(size int, metrics MetricsReporter) *AlgorithmR {
	return &AlgorithmR{
		maxSize: size,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		spans:   make(map[string]SpanData),
		metrics: metrics,
	}
}

// AddSpan adds a span to the reservoir
func (r *AlgorithmR) AddSpan(span SpanData) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Generate a unique key for the span
	key := span.ID + "-" + span.TraceID
	
	// If the reservoir isn't full yet, just add the span
	if r.currentSize < r.maxSize {
		r.spans[key] = span
		r.currentSize++
		
		// Update metrics
		if r.metrics != nil {
			r.metrics.ReportReservoirSize(r.currentSize)
			r.metrics.ReportSampledSpans(1)
		}
		
		return true
	}
	
	// Otherwise, randomly decide if this span should replace an existing one
	// Apply Algorithm R: with probability maxSize/(currentSize+1), keep the new item
	if j := r.rng.Intn(r.currentSize + 1); j < r.maxSize {
		// Choose a random element to replace
		toReplace := r.rng.Intn(r.maxSize)
		
		// Find the key of the element to replace
		var replaceKey string
		i := 0
		for k := range r.spans {
			if i == toReplace {
				replaceKey = k
				break
			}
			i++
		}
		
		// Replace the chosen element
		delete(r.spans, replaceKey)
		r.spans[key] = span
		
		// Update metrics
		if r.metrics != nil {
			r.metrics.ReportSampledSpans(1)
		}
		
		return true
	}
	
	r.currentSize++
	return false
}

// GetSample returns all spans in the reservoir
func (r *AlgorithmR) GetSample() []SpanData {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	result := make([]SpanData, 0, len(r.spans))
	for _, span := range r.spans {
		result = append(result, span)
	}
	
	return result
}

// Reset clears the reservoir
func (r *AlgorithmR) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.spans = make(map[string]SpanData)
	r.currentSize = 0
	
	// Update metrics
	if r.metrics != nil {
		r.metrics.ReportReservoirSize(0)
	}
}

// Size returns the current number of spans in the reservoir
func (r *AlgorithmR) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return len(r.spans)
}

// MaxSize returns the maximum capacity of the reservoir
func (r *AlgorithmR) MaxSize() int {
	return r.maxSize
}

// SetMaxSize updates the maximum capacity of the reservoir
func (r *AlgorithmR) SetMaxSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.maxSize = size
	
	// If the new size is smaller, we need to remove some spans
	if len(r.spans) > r.maxSize {
		// Create a slice of all keys
		keys := make([]string, 0, len(r.spans))
		for k := range r.spans {
			keys = append(keys, k)
		}
		
		// Shuffle the keys
		r.rng.Shuffle(len(keys), func(i, j int) {
			keys[i], keys[j] = keys[j], keys[i]
		})
		
		// Keep only maxSize elements
		for i := r.maxSize; i < len(keys); i++ {
			delete(r.spans, keys[i])
		}
		
		// Update metrics
		if r.metrics != nil {
			r.metrics.ReportReservoirSize(len(r.spans))
		}
	}
}

// GetAllSpansWithKeys returns all spans with their keys for serialization
func (r *AlgorithmR) GetAllSpansWithKeys() map[string]SpanData {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Create a copy of the map
	result := make(map[string]SpanData, len(r.spans))
	for k, v := range r.spans {
		result[k] = v
	}
	
	return result
}
