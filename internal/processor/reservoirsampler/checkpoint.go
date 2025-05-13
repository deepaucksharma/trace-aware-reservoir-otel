package reservoirsampler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v3"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

const (
	// Badger DB key prefixes
	keyPrefixState      = "state:"
	keyPrefixReservoir  = "reservoir:"
	keyPrefixWindow     = "window:"
	keyPrefixCheckpoint = "checkpoint:"
)

// BadgerCheckpointManager implements checkpoint management using BadgerDB
type BadgerCheckpointManager struct {
	// Badger database
	db *badger.DB
	
	// Configuration
	checkpointPath       string
	compactionTargetSize int64
	
	// State
	lastCheckpoint time.Time
	
	// Metrics
	checkpointAgeGauge     *atomic.Int64
	dbSizeGauge            *atomic.Int64
	compactionCountCounter *atomic.Int64
	
	// Logging
	logger *zap.Logger
}

// NewBadgerCheckpointManager creates a new BadgerCheckpointManager
func NewBadgerCheckpointManager(
	checkpointPath string, 
	compactionTargetSize int64,
	checkpointAgeGauge, dbSizeGauge, compactionCountCounter *atomic.Int64,
	logger *zap.Logger,
) (*BadgerCheckpointManager, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(checkpointPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}
	
	// Open BadgerDB with sensible defaults
	badgerOptions := badger.DefaultOptions(checkpointPath).
		WithLogger(zapToBadgerLogger{logger.Named("badger")}).
		WithSyncWrites(true).    // Ensure durability
		WithCompression(badger.ZSTD)  // Better compression
	
	db, err := badger.Open(badgerOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to open checkpoint database: %w", err)
	}
	
	return &BadgerCheckpointManager{
		db:                     db,
		checkpointPath:         checkpointPath,
		compactionTargetSize:   compactionTargetSize,
		lastCheckpoint:         time.Time{},
		checkpointAgeGauge:     checkpointAgeGauge,
		dbSizeGauge:            dbSizeGauge,
		compactionCountCounter: compactionCountCounter,
		logger:                 logger,
	}, nil
}

// Checkpoint saves the current state to persistent storage
func (c *BadgerCheckpointManager) Checkpoint(
	windowID int64, 
	startTime time.Time, 
	endTime time.Time, 
	windowCount int64, 
	spans map[uint64]SpanWithResource,
) error {
	startTimer := time.Now()
	
	// Create state buffer
	stateBuffer := &bytes.Buffer{}
	if err := binary.Write(stateBuffer, binary.BigEndian, windowID); err != nil {
		return fmt.Errorf("failed to write window ID: %w", err)
	}
	if err := binary.Write(stateBuffer, binary.BigEndian, startTime.Unix()); err != nil {
		return fmt.Errorf("failed to write start time: %w", err)
	}
	if err := binary.Write(stateBuffer, binary.BigEndian, endTime.Unix()); err != nil {
		return fmt.Errorf("failed to write end time: %w", err)
	}
	if err := binary.Write(stateBuffer, binary.BigEndian, windowCount); err != nil {
		return fmt.Errorf("failed to write window count: %w", err)
	}
	stateBytes := stateBuffer.Bytes()
	
	// Write state in transaction
	err := c.db.Update(func(txn *badger.Txn) error {
		// Write window state
		stateKey := []byte(fmt.Sprintf("%s%d", keyPrefixState, windowID))
		if err := txn.Set(stateKey, stateBytes); err != nil {
			return fmt.Errorf("failed to write state: %w", err)
		}
		
		// Write current window marker
		currentWindowKey := []byte(keyPrefixCheckpoint + "current_window")
		if err := txn.Set(currentWindowKey, []byte(fmt.Sprintf("%d", windowID))); err != nil {
			return fmt.Errorf("failed to write current window: %w", err)
		}
		
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}
	
	// Process spans in batches
	const batchSize = 100
	spanKeys := make([]uint64, 0, len(spans))
	
	// Collect all keys
	for hash := range spans {
		spanKeys = append(spanKeys, hash)
	}
	
	// Process in batches
	spanCount := 0
	totalSpans := len(spans)
	
	for i := 0; i < len(spanKeys); i += batchSize {
		end := i + batchSize
		if end > len(spanKeys) {
			end = len(spanKeys)
		}
		
		currentBatch := spanKeys[i:end]
		
		// Write spans in a transaction
		err = c.db.Update(func(txn *badger.Txn) error {
			for _, hash := range currentBatch {
				spanWithRes, exists := spans[hash]
				if !exists {
					continue
				}
				
				// Serialize span
				spanBytes, err := serializeSpanWithResource(spanWithRes)
				if err != nil {
					c.logger.Error("Failed to serialize span",
						zap.Uint64("hash", hash),
						zap.Error(err))
					continue
				}
				
				// Create key for this span
				key := []byte(fmt.Sprintf("%s%d:%d", keyPrefixReservoir, windowID, hash))
				
				// Write span
				if err := txn.Set(key, spanBytes); err != nil {
					return fmt.Errorf("failed to write span: %w", err)
				}
				
				spanCount++
			}
			
			return nil
		})
		
		if err != nil {
			c.logger.Error("Failed to write span batch",
				zap.Int("batch_start", i),
				zap.Int("batch_end", end),
				zap.Error(err))
			// Continue with next batch despite error
		}
		
		// Log progress for large reservoirs
		if totalSpans > 1000 && (i+batchSize)%1000 == 0 {
			c.logger.Debug("Checkpointing progress",
				zap.Int("spans_processed", i+batchSize),
				zap.Int("total_spans", totalSpans))
		}
	}
	
	// Update checkpoint time and metrics
	c.lastCheckpoint = time.Now()
	c.checkpointAgeGauge.Store(0)
	
	// Update database size metric
	if fi, err := os.Stat(c.checkpointPath); err == nil {
		c.dbSizeGauge.Store(fi.Size())
	}
	
	c.logger.Debug("Checkpoint completed",
		zap.Int("spans_saved", spanCount),
		zap.Int("total_spans", totalSpans),
		zap.Duration("duration", time.Since(startTimer)))
	
	return nil
}

// LoadCheckpoint loads the most recent state from persistent storage
func (c *BadgerCheckpointManager) LoadCheckpoint() (
	windowID int64, 
	startTime time.Time, 
	endTime time.Time, 
	windowCount int64, 
	spans map[uint64]SpanWithResource, 
	err error) {
	
	// Initialize return values
	windowID = 0
	startTime = time.Now()
	endTime = startTime.Add(1 * time.Minute) // Default 1 minute window
	windowCount = 0
	spans = make(map[uint64]SpanWithResource)
	
	// Find the current window ID
	var currentWindowID int64
	err = c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(keyPrefixCheckpoint + "current_window"))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return fmt.Errorf("no checkpoint found")
			}
			return fmt.Errorf("failed to get current window: %w", err)
		}
		
		return item.Value(func(val []byte) error {
			_, err := fmt.Sscanf(string(val), "%d", &currentWindowID)
			return err
		})
	})
	
	if err != nil {
		return windowID, startTime, endTime, windowCount, spans, err
	}
	
	// Read the window state
	err = c.db.View(func(txn *badger.Txn) error {
		stateKey := []byte(fmt.Sprintf("%s%d", keyPrefixState, currentWindowID))
		item, err := txn.Get(stateKey)
		if err != nil {
			return fmt.Errorf("failed to get window state: %w", err)
		}
		
		return item.Value(func(stateBytes []byte) error {
			if len(stateBytes) < 32 { // 4 int64s = 32 bytes
				return fmt.Errorf("invalid state data: too short")
			}
			
			buffer := bytes.NewReader(stateBytes)
			
			// Read state fields
			if err := binary.Read(buffer, binary.BigEndian, &windowID); err != nil {
				return fmt.Errorf("failed to read window ID: %w", err)
			}
			
			var startTimeUnix, endTimeUnix int64
			if err := binary.Read(buffer, binary.BigEndian, &startTimeUnix); err != nil {
				return fmt.Errorf("failed to read start time: %w", err)
			}
			if err := binary.Read(buffer, binary.BigEndian, &endTimeUnix); err != nil {
				return fmt.Errorf("failed to read end time: %w", err)
			}
			
			// Convert Unix timestamps to time.Time
			startTime = time.Unix(startTimeUnix, 0)
			endTime = time.Unix(endTimeUnix, 0)
			
			if err := binary.Read(buffer, binary.BigEndian, &windowCount); err != nil {
				return fmt.Errorf("failed to read window count: %w", err)
			}
			
			return nil
		})
	})
	
	if err != nil {
		return windowID, startTime, endTime, windowCount, spans, err
	}
	
	// Check if the window is still valid
	now := time.Now()
	if now.After(endTime) {
		// Window has expired, return empty state
		return 0, now, now.Add(1 * time.Minute), 0, spans, nil
	}
	
	// Read all spans for this window
	// Use a prefix scan for efficient retrieval
	prefix := []byte(fmt.Sprintf("%s%d:", keyPrefixReservoir, windowID))
	
	err = c.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 100 // Tune for performance
		
		it := txn.NewIterator(opts)
		defer it.Close()
		
		// Count for logging
		loadedCount := 0
		
		// Iterate through all keys with the prefix
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			
			// Extract the hash from the key
			keyStr := string(item.Key())
			var hash uint64
			_, err := fmt.Sscanf(keyStr, "%s%d:%d", &keyPrefixReservoir, &windowID, &hash)
			if err != nil {
				c.logger.Warn("Failed to parse span key", zap.String("key", keyStr), zap.Error(err))
				continue
			}
			
			// Read the span value
			err = item.Value(func(spanBytes []byte) error {
				// Deserialize the span
				spanWithRes, err := deserializeSpanWithResource(spanBytes)
				if err != nil {
					return fmt.Errorf("failed to deserialize span: %w", err)
				}
				
				// Add to spans map
				spans[hash] = spanWithRes
				loadedCount++
				
				return nil
			})
			
			if err != nil {
				c.logger.Warn("Failed to load span",
					zap.Uint64("hash", hash),
					zap.Error(err))
				continue
			}
			
			// Log progress for large reservoirs
			if loadedCount > 0 && loadedCount%1000 == 0 {
				c.logger.Debug("Loading checkpoint progress",
					zap.Int("loaded_spans", loadedCount))
			}
		}
		
		c.logger.Info("Loaded spans from checkpoint",
			zap.Int("spans_loaded", loadedCount))
		
		return nil
	})
	
	// If there was an error loading spans, still return the window state
	// but with an empty spans map
	if err != nil {
		c.logger.Error("Error loading spans from checkpoint", zap.Error(err))
		return windowID, startTime, endTime, windowCount, make(map[uint64]SpanWithResource), err
	}
	
	// Update metrics
	c.lastCheckpoint = time.Now()
	c.checkpointAgeGauge.Store(0)
	
	return windowID, startTime, endTime, windowCount, spans, nil
}

// Compact performs database compaction
func (c *BadgerCheckpointManager) Compact() error {
	// Check if compaction is needed based on file size
	fi, err := os.Stat(c.checkpointPath)
	if err != nil {
		return fmt.Errorf("failed to get database size: %w", err)
	}
	
	currentSize := fi.Size()
	if currentSize < c.compactionTargetSize {
		c.logger.Debug("Skipping compaction - current size below target",
			zap.Int64("current_size", currentSize),
			zap.Int64("target_size", c.compactionTargetSize))
		return nil
	}
	
	startTime := time.Now()
	c.logger.Info("Starting database compaction", zap.Int64("current_size", currentSize))
	
	// Run BadgerDB's built-in garbage collection
	err = c.db.RunValueLogGC(0.5) // GC if we can reclaim at least 50% of a file
	if err != nil && err != badger.ErrNoRewrite {
		return fmt.Errorf("failed to run value log GC: %w", err)
	}
	
	// Update metrics
	if fi, err := os.Stat(c.checkpointPath); err == nil {
		newSize := fi.Size()
		c.dbSizeGauge.Store(newSize)
		
		c.logger.Info("Database compaction completed",
			zap.Int64("original_size", currentSize),
			zap.Int64("new_size", newSize),
			zap.Float64("reduction_pct", float64(currentSize-newSize)*100/float64(currentSize)),
			zap.Duration("duration", time.Since(startTime)))
		
		c.compactionCountCounter.Inc()
	}
	
	return nil
}

// Close releases any resources used by the checkpoint manager
func (c *BadgerCheckpointManager) Close() error {
	return c.db.Close()
}

// UpdateMetrics updates the checkpoint age metric
func (c *BadgerCheckpointManager) UpdateMetrics() {
	if !c.lastCheckpoint.IsZero() {
		elapsed := time.Since(c.lastCheckpoint)
		c.checkpointAgeGauge.Store(int64(elapsed.Seconds()))
	}
}

// zapToBadgerLogger adapts zap.Logger to badger.Logger
type zapToBadgerLogger struct {
	*zap.Logger
}

func (l zapToBadgerLogger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

func (l zapToBadgerLogger) Warningf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

func (l zapToBadgerLogger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

func (l zapToBadgerLogger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// copyFile is a helper function to copy a file
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	
	return destFile.Sync()
}