package reservoirsampler

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// ReservoirStore defines the interface for a reservoir storage system
type ReservoirStore interface {
	// AddSpan adds a span to the reservoir
	AddSpan(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope)
	
	// Reset clears the reservoir and initializes a new window
	Reset(windowID int64, startTime time.Time, endTime time.Time)
	
	// Export returns all spans in the reservoir as traces
	Export(ctx context.Context) (ptrace.Traces, error)
	
	// Size returns the number of spans in the reservoir
	Size() int
}

// TraceManager defines the interface for trace aware sampling
type TraceManager interface {
	// AddSpan adds a span to the trace buffer
	AddSpan(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope)
	
	// GetCompletedTraces returns all completed traces
	GetCompletedTraces() []ptrace.Traces
	
	// Size returns the number of traces in the buffer
	Size() int
	
	// SpanCount returns the total number of spans in the buffer
	SpanCount() int
}

// CheckpointManager defines the interface for checkpoint persistence
type CheckpointManager interface {
	// Checkpoint saves the current state to persistent storage
	Checkpoint(windowID int64, startTime time.Time, endTime time.Time, windowCount int64, spans map[uint64]SpanWithResource) error
	
	// LoadCheckpoint loads the most recent state from persistent storage
	LoadCheckpoint() (windowID int64, startTime time.Time, endTime time.Time, windowCount int64, spans map[uint64]SpanWithResource, err error)
	
	// Close releases any resources used by the checkpoint manager
	Close() error
	
	// Compact performs database compaction
	Compact() error
}