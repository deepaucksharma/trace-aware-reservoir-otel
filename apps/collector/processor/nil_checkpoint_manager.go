package processor

import (
	"time"

	"github.com/deepaucksharma/reservoir"
)

// NilCheckpointManager implements a no-op checkpoint manager
type NilCheckpointManager struct{}

// NewNilCheckpointManager creates a new no-op checkpoint manager
func NewNilCheckpointManager() *NilCheckpointManager {
	return &NilCheckpointManager{}
}

// Checkpoint is a no-op
func (m *NilCheckpointManager) Checkpoint(
	windowID int64,
	startTime time.Time,
	endTime time.Time,
	windowCount int64,
	spans map[string]reservoir.SpanWithResource,
) error {
	// No-op
	return nil
}

// LoadCheckpoint returns an empty result
func (m *NilCheckpointManager) LoadCheckpoint() (
	windowID int64,
	startTime time.Time,
	endTime time.Time,
	windowCount int64,
	spans map[string]reservoir.SpanWithResource,
	err error,
) {
	// Return empty data
	return 0, time.Now(), time.Now().Add(time.Hour), 0, make(map[string]reservoir.SpanWithResource), nil
}

// Close is a no-op
func (m *NilCheckpointManager) Close() error {
	return nil
}

// Compact is a no-op
func (m *NilCheckpointManager) Compact() error {
	return nil
}

// UpdateMetrics is a no-op
func (m *NilCheckpointManager) UpdateMetrics() {
	// No-op
}
