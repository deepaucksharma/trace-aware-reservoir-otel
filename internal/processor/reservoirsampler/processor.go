package reservoirsampler

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/golang/protobuf/proto"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler/spanprotos"
)

const (
	// Bolt DB bucket names
	bucketReservoir    = "reservoir"
	bucketCheckpoint   = "checkpoint"
	keyReservoirState  = "state"
	keyCurrentWindow   = "current_window"
	keyWindowStartTime = "window_start_time"
)

// reservoirProcessor implements a reservoir sampler processor.
type reservoirProcessor struct {
	// Required interfaces for the processor
	component.StartFunc
	component.ShutdownFunc

	// Context for cancellation
	ctx       context.Context
	ctxCancel context.CancelFunc

	// Configuration
	config *Config

	// Next consumer in the pipeline
	nextConsumer consumer.Traces

	// Logger for debugging and error reporting
	logger *zap.Logger

	// Reservoir storage
	lock            sync.RWMutex
	reservoir       map[uint64]SpanWithResource
	reservoirHashes []uint64
	windowStartTime time.Time
	windowEndTime   time.Time
	windowCount     atomic.Int64
	windowSize      int
	currentWindow   int64
	random          *rand.Rand
	randomLock      sync.Mutex

	// Checkpoint and persistence
	db               *bolt.DB
	checkpointTicker *time.Ticker
	lastCheckpoint   time.Time
	stopChan         chan struct{}

	// Trace buffer for trace-aware sampling
	traceBuffer *TraceBuffer

	// Metrics
	reservoirSizeGauge     atomic.Int64
	windowCountGauge       atomic.Int64
	checkpointAgeGauge     atomic.Int64
	lruEvictionsCounter    atomic.Int64
	reservoirDbSizeGauge   atomic.Int64
	compactionCountCounter atomic.Int64

	// Compaction
	compactionCron *cron.Cron
}

// Ensure the processor implements the required interfaces
var _ processor.Traces = (*reservoirProcessor)(nil)
var _ component.Component = (*reservoirProcessor)(nil)

// newReservoirProcessor creates a new reservoir sampler processor.
func newReservoirProcessor(ctx context.Context, set component.TelemetrySettings, cfg *Config, nextConsumer consumer.Traces) (processor.Traces, error) {
	logger := set.Logger

	// Parse window duration
	windowDuration, err := time.ParseDuration(cfg.WindowDuration)
	if err != nil {
		return nil, fmt.Errorf("invalid window duration: %w", err)
	}
	_ = windowDuration // use this to avoid compiler error

	// Parse checkpoint interval
	checkpointInterval, err := time.ParseDuration(cfg.CheckpointInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid checkpoint interval: %w", err)
	}

	// Parse trace buffer timeout if trace-aware mode is enabled
	var bufferTimeout time.Duration
	if cfg.TraceAware {
		bufferTimeout, err = time.ParseDuration(cfg.TraceBufferTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid trace buffer timeout: %w", err)
		}
	}

	// Create context with cancellation
	processorCtx, processorCancel := context.WithCancel(ctx)

	// Create the processor
	p := &reservoirProcessor{
		ctx:             processorCtx,
		ctxCancel:       processorCancel,
		config:          cfg,
		nextConsumer:    nextConsumer,
		logger:          logger,
		reservoir:       make(map[uint64]SpanWithResource),
		reservoirHashes: make([]uint64, 0, cfg.SizeK),
		windowSize:      cfg.SizeK,
		random:          rand.New(rand.NewSource(time.Now().UnixNano())),
		stopChan:        make(chan struct{}),
	}

	// Create trace buffer if trace-aware mode is enabled
	if cfg.TraceAware {
		p.traceBuffer = NewTraceBuffer(cfg.TraceBufferMaxSize, bufferTimeout, logger)
		logger.Info("Trace-aware sampling enabled", 
			zap.Int("buffer_size", cfg.TraceBufferMaxSize),
			zap.String("buffer_timeout", cfg.TraceBufferTimeout))
	}

	// Create checkpoint directory if it doesn't exist
	if cfg.CheckpointPath != "" {
		// Ensure the directory exists
		checkpointDir := filepath.Dir(cfg.CheckpointPath)
		if err := os.MkdirAll(checkpointDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
		}

		// Open or create the bolt DB
		db, err := bolt.Open(cfg.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("failed to open checkpoint database: %w", err)
		}

		// Initialize the database buckets
		if err := db.Update(func(tx *bolt.Tx) error {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucketReservoir)); err != nil {
				return fmt.Errorf("failed to create reservoir bucket: %w", err)
			}
			if _, err := tx.CreateBucketIfNotExists([]byte(bucketCheckpoint)); err != nil {
				return fmt.Errorf("failed to create checkpoint bucket: %w", err)
			}
			return nil
		}); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to initialize checkpoint database: %w", err)
		}

		p.db = db
		p.checkpointTicker = time.NewTicker(checkpointInterval)

		logger.Info("Checkpoint storage initialized",
			zap.String("path", cfg.CheckpointPath),
			zap.String("interval", cfg.CheckpointInterval))
	}

	// Set up compaction if configured
	if cfg.DbCompactionScheduleCron != "" && cfg.DbCompactionTargetSize > 0 {
		p.compactionCron = cron.New()
		_, err := p.compactionCron.AddFunc(cfg.DbCompactionScheduleCron, func() {
			p.compactDatabase()
		})
		if err != nil {
			logger.Error("Failed to set up database compaction", zap.Error(err))
		} else {
			logger.Info("Database compaction scheduled", 
				zap.String("schedule", cfg.DbCompactionScheduleCron),
				zap.Int64("target_size_bytes", cfg.DbCompactionTargetSize))
		}
	}

	logger.Info("Reservoir sampler processor created",
		zap.Int("size", cfg.SizeK),
		zap.String("window", cfg.WindowDuration),
		zap.Bool("trace_aware", cfg.TraceAware))

	return p, nil
}

// Start implements the Component interface.
func (p *reservoirProcessor) Start(ctx context.Context, host component.Host) error {
	p.logger.Info("Starting reservoir sampler processor")

	// Initialize a new sampling window
	p.initializeWindow()

	// Try to load the previous state
	if p.db != nil {
		if err := p.loadState(); err != nil {
			p.logger.Error("Failed to load previous state, starting with empty reservoir", zap.Error(err))
		}
	}

	// Start background goroutines
	if p.checkpointTicker != nil {
		go p.checkpointLoop()
	}

	// Start compaction cron if configured
	if p.compactionCron != nil {
		p.compactionCron.Start()
	}

	// Start trace buffer processing if in trace-aware mode
	if p.config.TraceAware {
		go p.processTraceBuffer()
	}

	return nil
}

// Shutdown implements the Component interface.
func (p *reservoirProcessor) Shutdown(ctx context.Context) error {
	p.logger.Info("Shutting down reservoir sampler processor")

	// Signal all goroutines to stop
	p.ctxCancel()
	close(p.stopChan)

	// Stop checkpoint ticker
	if p.checkpointTicker != nil {
		p.checkpointTicker.Stop()
	}

	// Stop compaction cron
	if p.compactionCron != nil {
		p.compactionCron.Stop()
	}

	// Perform a final checkpoint
	if p.db != nil {
		if err := p.checkpoint(); err != nil {
			p.logger.Error("Failed to perform final checkpoint", zap.Error(err))
		}
		p.db.Close()
	}

	return nil
}

// ConsumeTraces implements the processor.Traces interface.
func (p *reservoirProcessor) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	if p.config.TraceAware {
		return p.consumeTracesAware(ctx, traces)
	}
	return p.consumeTracesSimple(ctx, traces)
}

// consumeTracesSimple implements standard reservoir sampling for traces.
func (p *reservoirProcessor) consumeTracesSimple(ctx context.Context, traces ptrace.Traces) error {
	// Check if we need to roll over to a new window
	p.checkWindowRollover()

	// Process each resource spans
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		resource := rs.Resource()
		
		// Process each instrumentation scope spans
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			ils := ilss.At(j)
			scope := ils.Scope()
			
			// Process each span
			spans := ils.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				
				// Add the span to the reservoir
				p.addSpanToReservoir(span, resource, scope)
			}
		}
	}

	return nil
}

// consumeTracesAware implements trace-aware reservoir sampling.
func (p *reservoirProcessor) consumeTracesAware(ctx context.Context, traces ptrace.Traces) error {
	// Check if we need to roll over to a new window
	p.checkWindowRollover()

	// Add all spans to the trace buffer
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		resource := rs.Resource()
		
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			ils := ilss.At(j)
			scope := ils.Scope()
			
			spans := ils.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				p.traceBuffer.AddSpan(span, resource, scope)
			}
		}
	}

	return nil
}

// Capabilities implements the processor.Traces interface.
func (p *reservoirProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// checkWindowRollover checks if it's time to roll over to a new sampling window
func (p *reservoirProcessor) checkWindowRollover() {
	now := time.Now()
	
	// Check if we're past the window end time
	if now.After(p.windowEndTime) {
		p.lock.Lock()
		defer p.lock.Unlock()
		
		// Double check after acquiring the lock
		if now.After(p.windowEndTime) {
			// Do a checkpoint before rolling over
			if p.db != nil {
				if err := p.checkpointLocked(); err != nil {
					p.logger.Error("Failed to checkpoint before window rollover", zap.Error(err))
				}
			}
			
			// Start a new window
			p.initializeWindowLocked()
			
			p.logger.Info("Started new sampling window",
				zap.Int64("window", p.currentWindow),
				zap.Time("start", p.windowStartTime),
				zap.Time("end", p.windowEndTime))
		}
	}
}

// initializeWindow initializes a new sampling window
func (p *reservoirProcessor) initializeWindow() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.initializeWindowLocked()
}

// initializeWindowLocked initializes a new sampling window (must be called with lock held)
func (p *reservoirProcessor) initializeWindowLocked() {
	now := time.Now()
	windowDuration, _ := time.ParseDuration(p.config.WindowDuration)
	
	p.windowStartTime = now
	p.windowEndTime = now.Add(windowDuration)
	p.currentWindow++
	p.windowCount.Store(0)
	
	// Clear the reservoir
	p.reservoir = make(map[uint64]SpanWithResource)
	p.reservoirHashes = make([]uint64, 0, p.windowSize)
}

// addSpanToReservoir adds a span to the reservoir using reservoir sampling algorithm
func (p *reservoirProcessor) addSpanToReservoir(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) {
	// Increment the total count for this window
	count := p.windowCount.Inc()
	
	// Create span key and hash
	key := createSpanKey(span)
	hash := hashSpanKey(key)
	
	p.lock.Lock()
	defer p.lock.Unlock()
	
	if int(count) <= p.windowSize {
		// Reservoir not full yet, add span directly
		p.reservoir[hash] = cloneSpanWithContext(span, resource, scope)
		p.reservoirHashes = append(p.reservoirHashes, hash)
	} else {
		// Reservoir is full, use reservoir sampling algorithm
		// Generate a random index in [0, count)
		p.randomLock.Lock()
		j := p.random.Int63n(count)
		p.randomLock.Unlock()
		
		if j < int64(p.windowSize) {
			// Replace the span at index j
			oldHash := p.reservoirHashes[j]
			p.reservoir[hash] = cloneSpanWithContext(span, resource, scope)
			delete(p.reservoir, oldHash)
			p.reservoirHashes[j] = hash
		}
		// If j >= size, just skip this span
	}
	
	// Update metrics
	p.reservoirSizeGauge.Store(int64(len(p.reservoir)))
	p.windowCountGauge.Store(count)
}

// processTraceBuffer periodically processes the trace buffer to add complete traces to the reservoir
func (p *reservoirProcessor) processTraceBuffer() {
	// Create a ticker with 1/10th of the trace timeout interval to check for completed traces
	bufferTimeout, _ := time.ParseDuration(p.config.TraceBufferTimeout)
	checkInterval := bufferTimeout / 10
	if checkInterval < time.Second {
		checkInterval = time.Second
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Get all completed traces from the buffer
			completedTraces := p.traceBuffer.GetCompletedTraces()
			for _, traces := range completedTraces {
				// Forward each trace to consumeTracesSimple for normal reservoir sampling
				p.consumeTracesSimple(p.ctx, traces)
			}
			
		case <-p.stopChan:
			return
		}
	}
}

// checkpoint performs a checkpoint of the current state to durable storage
func (p *reservoirProcessor) checkpoint() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.checkpointLocked()
}

// checkpointLocked performs a checkpoint while holding the lock
func (p *reservoirProcessor) checkpointLocked() error {
	if p.db == nil {
		return fmt.Errorf("checkpoint database not initialized")
	}
	
	startTime := time.Now()
	
	// Create state objects to serialize
	state := &spanprotos.ReservoirState{
		CurrentWindow:   p.currentWindow,
		WindowStartTime: p.windowStartTime.Unix(),
		WindowEndTime:   p.windowEndTime.Unix(),
		WindowCount:     p.windowCount.Load(),
	}
	
	stateBytes, err := proto.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint state: %w", err)
	}
	
	// Start a transaction
	err = p.db.Update(func(tx *bolt.Tx) error {
		// Save state
		checkpointBucket := tx.Bucket([]byte(bucketCheckpoint))
		if err := checkpointBucket.Put([]byte(keyReservoirState), stateBytes); err != nil {
			return fmt.Errorf("failed to write checkpoint state: %w", err)
		}
		
		// Clear the previous reservoir
		reservoirBucket := tx.Bucket([]byte(bucketReservoir))
		if err := reservoirBucket.DeleteBucket([]byte(fmt.Sprintf("window_%d", p.currentWindow))); err != nil && err != bolt.ErrBucketNotFound {
			return fmt.Errorf("failed to clear previous reservoir bucket: %w", err)
		}
		
		// Create a new bucket for this window
		windowBucket, err := reservoirBucket.CreateBucketIfNotExists([]byte(fmt.Sprintf("window_%d", p.currentWindow)))
		if err != nil {
			return fmt.Errorf("failed to create reservoir window bucket: %w", err)
		}
		
		// Save all spans in the reservoir
		for hash, spanWithRes := range p.reservoir {
			// Serialize the span
			spanBytes, err := serializeSpanWithResource(spanWithRes)
			if err != nil {
				p.logger.Error("Failed to serialize span for checkpoint", zap.Error(err))
				continue
			}
			
			// Save the span
			hashBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(hashBytes, hash)
			if err := windowBucket.Put(hashBytes, spanBytes); err != nil {
				return fmt.Errorf("failed to write span to checkpoint: %w", err)
			}
		}
		
		return nil
	})
	
	if err == nil {
		p.lastCheckpoint = time.Now()
		
		// Update metrics
		p.checkpointAgeGauge.Store(0)
		if fi, err := os.Stat(p.config.CheckpointPath); err == nil {
			p.reservoirDbSizeGauge.Store(fi.Size())
		}
		
		p.logger.Debug("Checkpoint completed",
			zap.Int("spans", len(p.reservoir)),
			zap.Duration("duration", time.Since(startTime)))
	}
	
	return err
}

// checkpointLoop runs a background goroutine to periodically checkpoint
func (p *reservoirProcessor) checkpointLoop() {
	for {
		select {
		case <-p.checkpointTicker.C:
			if err := p.checkpoint(); err != nil {
				p.logger.Error("Failed to checkpoint reservoir", zap.Error(err))
			} else {
				// Update checkpoint age metric
				elapsed := time.Since(p.lastCheckpoint)
				p.checkpointAgeGauge.Store(int64(elapsed.Seconds()))
			}
			
		case <-p.stopChan:
			return
		}
	}
}

// loadState loads the previous state from checkpoint storage
func (p *reservoirProcessor) loadState() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	
	if p.db == nil {
		return fmt.Errorf("checkpoint database not initialized")
	}
	
	var state *spanprotos.ReservoirState
	
	// Read the state
	err := p.db.View(func(tx *bolt.Tx) error {
		checkpointBucket := tx.Bucket([]byte(bucketCheckpoint))
		stateBytes := checkpointBucket.Get([]byte(keyReservoirState))
		if stateBytes == nil {
			return fmt.Errorf("no checkpoint state found")
		}
		
		state = &spanprotos.ReservoirState{}
		if err := proto.Unmarshal(stateBytes, state); err != nil {
			return fmt.Errorf("failed to unmarshal checkpoint state: %w", err)
		}
		
		return nil
	})
	
	if err != nil {
		return err
	}
	
	// Check if the previous window is still valid
	now := time.Now()
	windowEndTime := time.Unix(state.WindowEndTime, 0)
	
	if now.After(windowEndTime) {
		// Previous window has expired, start a new one
		p.initializeWindowLocked()
		p.logger.Info("Previous sampling window expired, starting a new one")
		return nil
	}
	
	// Previous window still valid, load it
	p.currentWindow = state.CurrentWindow
	p.windowStartTime = time.Unix(state.WindowStartTime, 0)
	p.windowEndTime = windowEndTime
	p.windowCount.Store(state.WindowCount)
	
	// Load the reservoir spans
	err = p.db.View(func(tx *bolt.Tx) error {
		reservoirBucket := tx.Bucket([]byte(bucketReservoir))
		windowBucket := reservoirBucket.Bucket([]byte(fmt.Sprintf("window_%d", p.currentWindow)))
		if windowBucket == nil {
			return fmt.Errorf("no reservoir bucket found for window %d", p.currentWindow)
		}
		
		// Read all spans
		p.reservoir = make(map[uint64]SpanWithResource)
		p.reservoirHashes = make([]uint64, 0, p.windowSize)
		
		err := windowBucket.ForEach(func(k, v []byte) error {
			if len(k) != 8 {
				return nil
			}
			
			hash := binary.BigEndian.Uint64(k)
			
			spanWithRes, err := deserializeSpanWithResource(v)
			if err != nil {
				p.logger.Error("Failed to deserialize span from checkpoint", zap.Error(err))
				return nil
			}
			
			p.reservoir[hash] = spanWithRes
			p.reservoirHashes = append(p.reservoirHashes, hash)
			
			return nil
		})
		
		if err != nil {
			return fmt.Errorf("failed to read spans from checkpoint: %w", err)
		}
		
		return nil
	})
	
	if err != nil {
		return err
	}
	
	p.lastCheckpoint = now
	
	// Update metrics
	p.reservoirSizeGauge.Store(int64(len(p.reservoir)))
	p.windowCountGauge.Store(state.WindowCount)
	
	p.logger.Info("Loaded previous state from checkpoint",
		zap.Int64("window", p.currentWindow),
		zap.Time("start", p.windowStartTime),
		zap.Time("end", p.windowEndTime),
		zap.Int("spans", len(p.reservoir)))
	
	return nil
}

// serializeSpanWithResource serializes a SpanWithResource to bytes
func serializeSpanWithResource(swr SpanWithResource) ([]byte, error) {
	// For a real implementation, we would use a proper serialization strategy
	// For now, we'll create a stub that stores only span ID and trace ID
	// This is a placeholder - in production, you'd implement proper serialization
	
	// Extract span ID and trace ID as examples of data to store
	traceID := swr.Span.TraceID()
	spanID := swr.Span.SpanID()
	
	// For demonstration, we'll just serialize these IDs
	summary := &spanprotos.SpanWithResourceSummary{
		SpanData:     append(traceID[:], spanID[:]...),
		ResourceData: []byte("resource-placeholder"),
		ScopeData:    []byte("scope-placeholder"),
	}
	
	// Marshal the summary
	return proto.Marshal(summary)
}

// deserializeSpanWithResource deserializes bytes to a SpanWithResource
func deserializeSpanWithResource(data []byte) (SpanWithResource, error) {
	// Unmarshal the SpanWithResourceSummary
	summary := &spanprotos.SpanWithResourceSummary{}
	if err := proto.Unmarshal(data, summary); err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to unmarshal span summary: %w", err)
	}
	
	// Create a new traces object with minimal structure
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	
	// In a real implementation, we would deserialize the actual span data
	// For now, we'll just return a minimal valid structure
	
	return SpanWithResource{
		Span:     span,
		Resource: rs.Resource(),
		Scope:    ss.Scope(),
	}, nil
}

// copyNestedBucket recursively copies a nested bucket and its contents
func copyNestedBucket(sourceBucket, destBucket *bolt.Bucket, bucketName []byte) error {
	// Get the source nested bucket
	sourceNestedBucket := sourceBucket.Bucket(bucketName)
	if sourceNestedBucket == nil {
		return fmt.Errorf("source nested bucket %s not found", string(bucketName))
	}

	// Create the destination nested bucket
	destNestedBucket, err := destBucket.CreateBucketIfNotExists(bucketName)
	if err != nil {
		return fmt.Errorf("error creating nested bucket %s: %w", string(bucketName), err)
	}

	// Copy all key-value pairs from source to destination
	return sourceNestedBucket.ForEach(func(k, v []byte) error {
		// If value is nil, it's another nested bucket
		if v == nil {
			return copyNestedBucket(sourceNestedBucket, destNestedBucket, k)
		}
		// Otherwise, it's a key-value pair
		return destNestedBucket.Put(k, v)
	})
}

// compactDatabase performs database compaction if the size exceeds the target
func (p *reservoirProcessor) compactDatabase() {
	if p.db == nil || p.config.DbCompactionTargetSize <= 0 {
		return
	}
	
	// Check current DB size
	fi, err := os.Stat(p.config.CheckpointPath)
	if err != nil {
		p.logger.Error("Failed to get database file info", zap.Error(err))
		return
	}
	
	// If size is below target, nothing to do
	currentSize := fi.Size()
	if currentSize <= p.config.DbCompactionTargetSize {
		p.logger.Debug("Database size below target, no compaction needed",
			zap.Int64("current_size", currentSize),
			zap.Int64("target_size", p.config.DbCompactionTargetSize))
		return
	}
	
	p.logger.Info("Database compaction started",
		zap.Int64("current_size", currentSize),
		zap.Int64("target_size", p.config.DbCompactionTargetSize))
	
	startTime := time.Now()
	
	// Perform compaction manually - BoltDB doesn't have a built-in Compact function in newer versions
	tempFile := p.config.CheckpointPath + ".compact"

	// Close the DB for compaction
	p.lock.Lock()
	db := p.db
	p.db = nil
	p.lock.Unlock()

	if err := db.Close(); err != nil {
		p.logger.Error("Failed to close database for compaction", zap.Error(err))

		// Reopen the database
		if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
		}

		return
	}

	// Manual compaction by creating a new DB and copying the data
	newDb, err := bolt.Open(tempFile, 0644, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		p.logger.Error("Database compaction failed", zap.Error(err))
		
		// Reopen the original database
		if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
		}
		
		return
	}

	// Reopen the source database in read-only mode
	sourceDb, err := bolt.Open(p.config.CheckpointPath, 0444, &bolt.Options{ReadOnly: true, Timeout: 1 * time.Second})
	if err != nil {
		p.logger.Error("Failed to open source database for compaction", zap.Error(err))
		newDb.Close()
		os.Remove(tempFile)
		
		// Reopen the original database
		if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
		}
		
		return
	}

	// Copy all data from the source to the destination database
	err = sourceDb.View(func(sourceTx *bolt.Tx) error {
		return newDb.Update(func(destTx *bolt.Tx) error {
			// Iterate over all buckets in the source database
			return sourceTx.ForEach(func(bucketName []byte, sourceBucket *bolt.Bucket) error {
				// Create the bucket in the destination
				destBucket, err := destTx.CreateBucketIfNotExists(bucketName)
				if err != nil {
					return fmt.Errorf("error creating bucket %s: %w", string(bucketName), err)
				}

				// Iterate over all keys in the source bucket
				return sourceBucket.ForEach(func(k, v []byte) error {
					// If the value is nil, it's a nested bucket
					if v == nil {
						// Handle nested buckets recursively
						return copyNestedBucket(sourceBucket, destBucket, k)
					}
					
					// Otherwise, it's a key-value pair
					return destBucket.Put(k, v)
				})
			})
		})
	})

	// Close the source database
	sourceDb.Close()

	if err != nil {
		p.logger.Error("Failed to copy data during compaction", zap.Error(err))
		newDb.Close()
		os.Remove(tempFile)
		
		// Reopen the original database
		if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
		}
		
		return
	}

	// Close the new database
	if err := newDb.Close(); err != nil {
		p.logger.Error("Failed to close new database after compaction", zap.Error(err))
		os.Remove(tempFile)
		
		// Reopen the original database
		if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
		}
		
		return
	}
	
	// Replace the original file with the compacted one
	if err := os.Rename(tempFile, p.config.CheckpointPath); err != nil {
		p.logger.Error("Failed to replace database with compacted version", zap.Error(err))
		
		// Cleanup temporary file
		os.Remove(tempFile)
		
		// Reopen the original database
		if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
		}
		
		return
	}
	
	// Reopen the compacted database
	if db, err := bolt.Open(p.config.CheckpointPath, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
		p.logger.Error("Failed to open compacted database", zap.Error(err))
	} else {
		p.lock.Lock()
		p.db = db
		p.lock.Unlock()
	}
	
	// Get new size
	if fi, err := os.Stat(p.config.CheckpointPath); err == nil {
		newSize := fi.Size()
		p.reservoirDbSizeGauge.Store(newSize)
		
		p.logger.Info("Database compaction completed",
			zap.Int64("original_size", currentSize),
			zap.Int64("new_size", newSize),
			zap.Float64("reduction_pct", float64(currentSize-newSize)*100/float64(currentSize)),
			zap.Duration("duration", time.Since(startTime)))
		
		// Update metrics
		p.compactionCountCounter.Inc()
	} else {
		p.logger.Error("Failed to get compacted database size", zap.Error(err))
	}
}