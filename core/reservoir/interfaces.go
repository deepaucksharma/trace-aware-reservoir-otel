package reservoir

import (
	"time"
)

// Reservoir defines the core sampling algorithm interface
type Reservoir interface {
	// AddSpan adds a span to the reservoir
	// Returns true if the span was added, false otherwise
	AddSpan(span SpanData) bool
	
	// GetSample returns a snapshot of the current reservoir contents
	GetSample() []SpanData
	
	// Reset clears the reservoir contents
	Reset()
	
	// Size returns the current number of spans in the reservoir
	Size() int
	
	// MaxSize returns the maximum number of spans the reservoir can hold
	MaxSize() int
	
	// SetMaxSize updates the maximum number of spans the reservoir can hold
	SetMaxSize(size int)
}

// Window defines the time window management interface
type Window interface {
	// Current returns the current window ID, start time, and end time
	Current() (id int64, start, end time.Time)
	
	// CheckRollover checks if a window rollover should occur and performs it if needed
	// Returns true if a rollover occurred
	CheckRollover() bool
	
	// SetRolloverCallback sets the function to call when a window rolls over
	SetRolloverCallback(callback func())
}

// CheckpointManager defines the interface for checkpoint persistence
type CheckpointManager interface {
	// Checkpoint saves the current state of the reservoir
	Checkpoint(windowID int64, startTime time.Time, endTime time.Time, windowCount int64, spans map[string]SpanWithResource) error
	
	// LoadCheckpoint loads a previously saved state of the reservoir
	LoadCheckpoint() (windowID int64, startTime time.Time, endTime time.Time, windowCount int64, spans map[string]SpanWithResource, err error)
	
	// Close releases resources used by the checkpoint manager
	Close() error
	
	// Compact performs compaction of the underlying storage
	Compact() error
	
	// UpdateMetrics updates metrics about the checkpoint state
	UpdateMetrics()
}

// TraceAggregator defines the trace buffering interface
type TraceAggregator interface {
	// AddSpan adds a span to the trace buffer
	AddSpan(span SpanData)
	
	// GetCompletedTraces returns all traces that are considered complete
	GetCompletedTraces() [][]SpanData
	
	// SetEvictionCallback sets the function to call when a trace is evicted
	SetEvictionCallback(callback func(traceID string))
}

// MetricsReporter defines the interface for reporting metrics
type MetricsReporter interface {
	// ReportReservoirSize reports the current size of the reservoir
	ReportReservoirSize(size int)
	
	// ReportSampledSpans reports the number of spans that were sampled
	ReportSampledSpans(count int)
	
	// ReportTraceBufferSize reports the current size of the trace buffer
	ReportTraceBufferSize(size int)
	
	// ReportEvictions reports the number of trace evictions from the buffer
	ReportEvictions(count int)
	
	// ReportCheckpointAge reports the age of the last checkpoint
	ReportCheckpointAge(age time.Duration)
	
	// ReportDBSize reports the size of the checkpoint storage
	ReportDBSize(sizeBytes int64)
	
	// ReportCompactions reports the number of storage compactions
	ReportCompactions(count int)
}
