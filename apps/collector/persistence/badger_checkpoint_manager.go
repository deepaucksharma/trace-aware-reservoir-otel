package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/deepaucksharma/reservoir"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"
)

const (
	// Key prefixes for different types of data
	keyPrefixWindow  = "window:"
	keyPrefixSpan    = "span:"
	keyPrefixMeta    = "meta:"
	keyMetaLastCheck = "meta:last_checkpoint"
)

// BadgerCheckpointManager implements persistence using BadgerDB
type BadgerCheckpointManager struct {
	db                 *badger.DB
	path               string
	logger             *zap.Logger
	compactionTarget   int64
	checkpointAgeGauge func(float64)
	dbSizeGauge        func(float64)
	compactionCounter  func(float64)
	lastCheckpoint     time.Time
}

// NewBadgerCheckpointManager creates a new BadgerDB-based checkpoint manager
func NewBadgerCheckpointManager(
	path string,
	compactionTarget int64,
	checkpointAgeGauge func(float64),
	dbSizeGauge func(float64),
	compactionCounter func(float64),
	logger *zap.Logger,
) (*BadgerCheckpointManager, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	// Open BadgerDB
	opts := badger.DefaultOptions(path).
		WithLogger(nil).        // Disable Badger's default logger
		WithLoggingLevel(3).    // Only log errors and worse
		WithSyncWrites(true).   // Ensure durability
		WithValueDir(path)      // Store values in the same directory

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	manager := &BadgerCheckpointManager{
		db:                 db,
		path:               path,
		logger:             logger,
		compactionTarget:   compactionTarget,
		checkpointAgeGauge: checkpointAgeGauge,
		dbSizeGauge:        dbSizeGauge,
		compactionCounter:  compactionCounter,
	}

	// Update metrics on startup
	manager.UpdateMetrics()

	return manager, nil
}

// Checkpoint saves the current state of the reservoir
func (m *BadgerCheckpointManager) Checkpoint(
	windowID int64,
	startTime time.Time,
	endTime time.Time,
	windowCount int64,
	spans map[string]reservoir.SpanWithResource,
) error {
	// Start a BadgerDB transaction
	txn := m.db.NewTransaction(true)
	defer txn.Discard()

	// Save window metadata
	windowKey := fmt.Sprintf("%s%d", keyPrefixWindow, windowID)
	windowData := struct {
		ID         int64     `json:"id"`
		StartTime  time.Time `json:"start_time"`
		EndTime    time.Time `json:"end_time"`
		CountTotal int64     `json:"count_total"`
	}{
		ID:         windowID,
		StartTime:  startTime,
		EndTime:    endTime,
		CountTotal: windowCount,
	}

	windowJSON, err := json.Marshal(windowData)
	if err != nil {
		return fmt.Errorf("failed to marshal window data: %w", err)
	}

	if err := txn.Set([]byte(windowKey), windowJSON); err != nil {
		return fmt.Errorf("failed to save window data: %w", err)
	}

	// Save each span
	for key, span := range spans {
		spanKey := fmt.Sprintf("%s%s", keyPrefixSpan, key)
		spanJSON, err := json.Marshal(span)
		if err != nil {
			return fmt.Errorf("failed to marshal span data: %w", err)
		}

		if err := txn.Set([]byte(spanKey), spanJSON); err != nil {
			return fmt.Errorf("failed to save span data: %w", err)
		}
	}

	// Save metadata about this checkpoint
	now := time.Now()
	m.lastCheckpoint = now
	metaData := struct {
		Timestamp  time.Time `json:"timestamp"`
		WindowID   int64     `json:"window_id"`
		SpanCount  int       `json:"span_count"`
		WindowData windowData `json:"window_data"`
	}{
		Timestamp:  now,
		WindowID:   windowID,
		SpanCount:  len(spans),
		WindowData: windowData,
	}

	metaJSON, err := json.Marshal(metaData)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := txn.Set([]byte(keyMetaLastCheck), metaJSON); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Commit the transaction
	if err := txn.Commit(); err != nil {
		return fmt.Errorf("failed to commit checkpoint transaction: %w", err)
	}

	m.logger.Info("Checkpoint created",
		zap.Int64("window_id", windowID),
		zap.Time("start", startTime),
		zap.Time("end", endTime),
		zap.Int("spans", len(spans)))

	// Update metrics
	m.UpdateMetrics()

	return nil
}

// LoadCheckpoint loads a previously saved state of the reservoir
func (m *BadgerCheckpointManager) LoadCheckpoint() (
	windowID int64,
	startTime time.Time,
	endTime time.Time,
	windowCount int64,
	spans map[string]reservoir.SpanWithResource,
	err error,
) {
	// Initialize an empty span map
	spans = make(map[string]reservoir.SpanWithResource)

	// Get the last checkpoint metadata
	var meta struct {
		Timestamp  time.Time `json:"timestamp"`
		WindowID   int64     `json:"window_id"`
		SpanCount  int       `json:"span_count"`
		WindowData struct {
			ID         int64     `json:"id"`
			StartTime  time.Time `json:"start_time"`
			EndTime    time.Time `json:"end_time"`
			CountTotal int64     `json:"count_total"`
		} `json:"window_data"`
	}

	err = m.db.View(func(txn *badger.Txn) error {
		// Get the metadata
		item, err := txn.Get([]byte(keyMetaLastCheck))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return fmt.Errorf("no checkpoint found")
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &meta)
		})
	})

	if err != nil {
		return 0, time.Time{}, time.Time{}, 0, nil, fmt.Errorf("failed to read checkpoint metadata: %w", err)
	}

	// Set the window data from metadata
	windowID = meta.WindowData.ID
	startTime = meta.WindowData.StartTime
	endTime = meta.WindowData.EndTime
	windowCount = meta.WindowData.CountTotal

	// Get all spans
	err = m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(keyPrefixSpan)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			spanKey := key[len(keyPrefixSpan):]

			err := item.Value(func(val []byte) error {
				var span reservoir.SpanWithResource
				if err := json.Unmarshal(val, &span); err != nil {
					return err
				}

				spans[spanKey] = span
				return nil
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return 0, time.Time{}, time.Time{}, 0, nil, fmt.Errorf("failed to read checkpoint spans: %w", err)
	}

	m.logger.Info("Checkpoint loaded",
		zap.Int64("window_id", windowID),
		zap.Time("start", startTime),
		zap.Time("end", endTime),
		zap.Int("spans", len(spans)))

	return windowID, startTime, endTime, windowCount, spans, nil
}

// Close releases resources used by the checkpoint manager
func (m *BadgerCheckpointManager) Close() error {
	return m.db.Close()
}

// Compact performs compaction of the underlying storage
func (m *BadgerCheckpointManager) Compact() error {
	m.logger.Info("Starting database compaction")

	if err := m.db.Flatten(2); err != nil {
		m.logger.Error("Error during database flattening", zap.Error(err))
		return err
	}

	if err := m.db.RunValueLogGC(0.5); err != nil {
		if err == badger.ErrNoRewrite {
			m.logger.Info("No garbage collection needed")
		} else {
			m.logger.Error("Error during value log garbage collection", zap.Error(err))
			return err
		}
	}

	m.logger.Info("Database compaction completed")

	// Update metrics
	if m.compactionCounter != nil {
		m.compactionCounter(1)
	}
	m.UpdateMetrics()

	return nil
}

// UpdateMetrics updates metrics about the checkpoint state
func (m *BadgerCheckpointManager) UpdateMetrics() {
	// Update checkpoint age gauge
	if m.checkpointAgeGauge != nil {
		age := time.Since(m.lastCheckpoint).Seconds()
		m.checkpointAgeGauge(age)
	}

	// Update DB size gauge
	if m.dbSizeGauge != nil {
		// Calculate DB size by summing all files in the directory
		var totalSize int64
		err := filepath.Walk(m.path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				totalSize += info.Size()
			}
			return nil
		})

		if err == nil {
			m.dbSizeGauge(float64(totalSize))
		} else {
			m.logger.Warn("Failed to calculate DB size", zap.Error(err))
		}
	}
}
