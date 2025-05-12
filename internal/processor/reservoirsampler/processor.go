package reservoirsampler

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

const (
	// Bolt DB bucket names
	bucketReservoir    = "reservoir"
	bucketCheckpoint   = "checkpoint"
	keyReservoirState  = "state"
	keyCurrentWindow   = "current_window"
	keyWindowStartTime = "window_start_time"
)

// reservoirProcessor implements a reservoir sampler processor that provides
// statistically sound sampling of spans while preserving complete traces.
//
// Key features:
// - Reservoir sampling using Algorithm R for statistically representative sampling
// - Trace-aware mode to preserve complete traces
// - Persistent storage of reservoir state for durability across restarts
// - Metrics for monitoring performance and behavior
// - Configurable window sizes and sampling rates
//
// The implementation follows the technical specification, prioritizing:
// - Memory efficiency and bounds
// - Correctness of sampling algorithm
// - Durability of sampled data
// - Thread safety and concurrent access
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
	sampledSpansCounter    atomic.Int64

	// Metrics export
	meter     metric.Meter
	metricCtx context.Context

	// Compaction
	compactionCron *cron.Cron
}

// Ensure the processor implements the required interfaces
var _ processor.Traces = (*reservoirProcessor)(nil)
var _ component.Component = (*reservoirProcessor)(nil)

// newReservoirProcessor creates a new reservoir sampler processor.
func newReservoirProcessor(ctx context.Context, set component.TelemetrySettings, cfg *Config, nextConsumer consumer.Traces) (processor.Traces, error) {
	logger := set.Logger

	// Parse window duration to validate it
	_, err := time.ParseDuration(cfg.WindowDuration)
	if err != nil {
		return nil, fmt.Errorf("invalid window duration: %w", err)
	}

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

	// Create the processor with a secure source for random number generation
	source := rand.NewSource(time.Now().UnixNano())
	p := &reservoirProcessor{
		ctx:             processorCtx,
		ctxCancel:       processorCancel,
		config:          cfg,
		nextConsumer:    nextConsumer,
		logger:          logger,
		reservoir:       make(map[uint64]SpanWithResource),
		reservoirHashes: make([]uint64, 0, cfg.SizeK),
		windowSize:      cfg.SizeK,
		random:          rand.New(source),
		stopChan:        make(chan struct{}),
		meter:           set.MeterProvider.Meter("reservoirsampler"),
		metricCtx:       processorCtx,
	}

	// Create trace buffer if trace-aware mode is enabled
	if cfg.TraceAware {
		p.traceBuffer = NewTraceBuffer(cfg.TraceBufferMaxSize, bufferTimeout, logger)
		// Connect the trace buffer to the processor's eviction counter
		p.traceBuffer.SetEvictionCounter(&p.lruEvictionsCounter)
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
			if closeErr := db.Close(); closeErr != nil {
				logger.Error("Failed to close database after initialization error", zap.Error(closeErr))
			}
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

// registerMetrics sets up the metrics for the processor
func (p *reservoirProcessor) registerMetrics() error {
	// First, set up synchronous gauges that directly read atomic values
	var err error

	// Reservoir size gauge
	_, err = p.meter.Int64ObservableGauge(
		"reservoir_sampler.reservoir_size",
		metric.WithDescription("Number of spans currently in the reservoir"),
		metric.WithUnit("{spans}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.reservoirSizeGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create reservoir size gauge: %w", err)
	}

	// Window count gauge
	_, err = p.meter.Int64ObservableGauge(
		"reservoir_sampler.window_count",
		metric.WithDescription("Total number of spans seen in the current window"),
		metric.WithUnit("{spans}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.windowCountGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create window count gauge: %w", err)
	}

	// Checkpoint age gauge
	_, err = p.meter.Int64ObservableGauge(
		"reservoir_sampler.checkpoint_age",
		metric.WithDescription("Age of the last checkpoint in seconds"),
		metric.WithUnit("s"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.checkpointAgeGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create checkpoint age gauge: %w", err)
	}

	// DB size gauge
	_, err = p.meter.Int64ObservableGauge(
		"reservoir_sampler.db_size",
		metric.WithDescription("Size of the reservoir checkpoint database in bytes"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.reservoirDbSizeGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create db size gauge: %w", err)
	}

	// Compaction counter - this is monotonic
	_, err = p.meter.Int64ObservableCounter(
		"reservoir_sampler.db_compactions",
		metric.WithDescription("Number of database compactions performed"),
		metric.WithUnit("{compactions}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.compactionCountCounter.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create compaction counter: %w", err)
	}

	// LRU evictions counter - this is monotonic
	_, err = p.meter.Int64ObservableCounter(
		"reservoir_sampler.lru_evictions",
		metric.WithDescription("Number of trace evictions from the LRU cache"),
		metric.WithUnit("{evictions}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.lruEvictionsCounter.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create LRU evictions counter: %w", err)
	}

	// Sampled spans counter - this is monotonic
	_, err = p.meter.Int64ObservableCounter(
		"reservoir_sampler.sampled_spans",
		metric.WithDescription("Number of spans sampled (added to reservoir)"),
		metric.WithUnit("{spans}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(p.sampledSpansCounter.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create sampled spans counter: %w", err)
	}

	// For trace-aware mode, add trace buffer metrics
	if p.config.TraceAware && p.traceBuffer != nil {
		// Trace buffer size
		_, err = p.meter.Int64ObservableGauge(
			"reservoir_sampler.trace_buffer_size",
			metric.WithDescription("Number of traces currently in the buffer"),
			metric.WithUnit("{traces}"),
			metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
				o.Observe(int64(p.traceBuffer.Size()))
				return nil
			}),
		)
		if err != nil {
			return fmt.Errorf("failed to create trace buffer size gauge: %w", err)
		}

		// Trace buffer span count
		_, err = p.meter.Int64ObservableGauge(
			"reservoir_sampler.trace_buffer_span_count",
			metric.WithDescription("Total number of spans in the trace buffer"),
			metric.WithUnit("{spans}"),
			metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
				o.Observe(int64(p.traceBuffer.SpanCount()))
				return nil
			}),
		)
		if err != nil {
			return fmt.Errorf("failed to create trace buffer span count gauge: %w", err)
		}
	}

	return nil
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

	// Register metrics
	if err := p.registerMetrics(); err != nil {
		p.logger.Error("Failed to register metrics", zap.Error(err))
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
		if closeErr := p.db.Close(); closeErr != nil {
			p.logger.Error("Failed to close database during shutdown", zap.Error(closeErr))
		}
	}

	return nil
}

// ConsumeTraces implements the processor.Traces interface.
func (p *reservoirProcessor) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	startTime := time.Now()
	var err error

	// Process through the appropriate mode
	if p.config.TraceAware {
		err = p.consumeTracesAware(ctx, traces)
	} else {
		err = p.consumeTracesSimple(ctx, traces)
	}

	// Capture metrics for the operation
	latency := time.Since(startTime)

	// Log processing time for large trace batches
	if traces.SpanCount() > 1000 {
		p.logger.Debug("Processed large trace batch",
			zap.Int("span_count", traces.SpanCount()),
			zap.Duration("latency", latency))
	}

	return err
}

// consumeTracesSimple implements standard reservoir sampling for traces.
func (p *reservoirProcessor) consumeTracesSimple(ctx context.Context, traces ptrace.Traces) error {
	// Check if we need to roll over to a new window
	p.checkWindowRollover()

	// Use a sync.Pool to get a slice for storing span data
	spanDataList := make([]struct {
		span     ptrace.Span
		resource pcommon.Resource
		scope    pcommon.InstrumentationScope
	}, 0, 100) // Pre-allocate some capacity for better performance

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

				// Collect the span data
				spanDataList = append(spanDataList, struct {
					span     ptrace.Span
					resource pcommon.Resource
					scope    pcommon.InstrumentationScope
				}{
					span:     span,
					resource: resource,
					scope:    scope,
				})
			}
		}
	}

	// Now add all spans to the reservoir in a single lock section
	for _, data := range spanDataList {
		p.addSpanToReservoir(data.span, data.resource, data.scope)
	}

	return nil
}

// consumeTracesAware implements trace-aware reservoir sampling.
func (p *reservoirProcessor) consumeTracesAware(ctx context.Context, traces ptrace.Traces) error {
	// Check if we need to roll over to a new window
	p.checkWindowRollover()

	// Prepare a list of spans to add to the trace buffer
	spanDataList := make([]struct {
		span     ptrace.Span
		resource pcommon.Resource
		scope    pcommon.InstrumentationScope
	}, 0, 100) // Pre-allocate some capacity for better performance

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
				spanDataList = append(spanDataList, struct {
					span     ptrace.Span
					resource pcommon.Resource
					scope    pcommon.InstrumentationScope
				}{
					span:     span,
					resource: resource,
					scope:    scope,
				})
			}
		}
	}

	// Add all spans to the trace buffer in a batch
	for _, data := range spanDataList {
		p.traceBuffer.AddSpan(data.span, data.resource, data.scope)
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
//
// This implements Algorithm R (Jeffrey Vitter):
//  1. If we have seen fewer than k elements, add the element to our reservoir
//  2. Otherwise, with probability k/n, keep the new element
//     where:
//     n = the number of elements we have seen so far
//     k = the size of our reservoir
//
// In this implementation, when an item is selected to be kept with probability k/n,
// we replace a random existing element in the reservoir with the new one.
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
		// Increment the sampled span counter
		p.sampledSpansCounter.Inc()
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
			// Increment the sampled span counter
			p.sampledSpansCounter.Inc()
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
			if len(completedTraces) > 0 {
				p.logger.Debug("Processing completed traces",
					zap.Int("count", len(completedTraces)))
			}

			for _, traces := range completedTraces {
				// Forward each trace to consumeTracesSimple for normal reservoir sampling
				if err := p.consumeTracesSimple(p.ctx, traces); err != nil {
					p.logger.Error("Failed to process completed trace", zap.Error(err))
				}
			}

		case <-p.ctx.Done():
			p.logger.Info("Stopping trace buffer processor due to context cancellation")
			return

		case <-p.stopChan:
			p.logger.Info("Stopping trace buffer processor due to stopChan signal")
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
	// Use manual serialization for the state instead of protobuf to avoid stack issues
	stateBuffer := &bytes.Buffer{}
	if err := binary.Write(stateBuffer, binary.BigEndian, p.currentWindow); err != nil {
		return fmt.Errorf("failed to write current window: %w", err)
	}
	if err := binary.Write(stateBuffer, binary.BigEndian, p.windowStartTime.Unix()); err != nil {
		return fmt.Errorf("failed to write window start time: %w", err)
	}
	if err := binary.Write(stateBuffer, binary.BigEndian, p.windowEndTime.Unix()); err != nil {
		return fmt.Errorf("failed to write window end time: %w", err)
	}
	if err := binary.Write(stateBuffer, binary.BigEndian, p.windowCount.Load()); err != nil {
		return fmt.Errorf("failed to write window count: %w", err)
	}
	stateBytes := stateBuffer.Bytes()

	// Save the state first in its own transaction
	err := p.db.Update(func(tx *bolt.Tx) error {
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
		_, err := reservoirBucket.CreateBucketIfNotExists([]byte(fmt.Sprintf("window_%d", p.currentWindow)))
		if err != nil {
			return fmt.Errorf("failed to create reservoir window bucket: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint: %w", err)
	}

	// Count of spans we will save
	spanCount := 0
	totalSpans := len(p.reservoir)

	// Process spans in batches to avoid memory pressure
	const batchSize = 10
	spanKeys := make([]uint64, 0, len(p.reservoir))

	// Collect all keys first
	for hash := range p.reservoir {
		spanKeys = append(spanKeys, hash)
	}

	// Process in fixed size batches
	for i := 0; i < len(spanKeys); i += batchSize {
		end := i + batchSize
		if end > len(spanKeys) {
			end = len(spanKeys)
		}

		currentBatch := spanKeys[i:end]

		// Process this batch in its own transaction
		err = p.db.Update(func(tx *bolt.Tx) error {
			reservoirBucket := tx.Bucket([]byte(bucketReservoir))
			windowBucket := reservoirBucket.Bucket([]byte(fmt.Sprintf("window_%d", p.currentWindow)))
			if windowBucket == nil {
				return fmt.Errorf("window bucket not found for %d", p.currentWindow)
			}

			for _, hash := range currentBatch {
				spanWithRes, exists := p.reservoir[hash]
				if !exists {
					continue // Skip if somehow the span was removed
				}

				// Serialize the span with our direct binary serialization
				spanBytes, err := serializeSpanWithResource(spanWithRes)
				if err != nil {
					p.logger.Error("Failed to serialize span for checkpoint",
						zap.Error(err),
						zap.Uint64("hash", hash))
					continue
				}

				// Save the span
				hashBytes := make([]byte, 8)
				binary.BigEndian.PutUint64(hashBytes, hash)
				if err := windowBucket.Put(hashBytes, spanBytes); err != nil {
					return fmt.Errorf("failed to write span to checkpoint: %w", err)
				}
				spanCount++
			}

			return nil
		})

		if err != nil {
			p.logger.Error("Failed to checkpoint span batch",
				zap.Error(err),
				zap.Int("batch_start", i),
				zap.Int("batch_end", end))
			// Continue with next batch despite error
		}

		// Log progress for large reservoirs
		if totalSpans > 100 && (i+batchSize)%100 == 0 {
			p.logger.Debug("Checkpointing progress",
				zap.Int("spans_processed", i+batchSize),
				zap.Int("total_spans", totalSpans))
		}
	}

	// Finalize the checkpoint
	if err == nil {
		p.lastCheckpoint = time.Now()

		// Update metrics
		p.checkpointAgeGauge.Store(0)
		if fi, err := os.Stat(p.config.CheckpointPath); err == nil {
			p.reservoirDbSizeGauge.Store(fi.Size())
		}

		p.logger.Debug("Checkpoint completed",
			zap.Int("spans_saved", spanCount),
			zap.Int("total_spans", totalSpans),
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

	// Variables to hold state
	var currentWindow int64
	var windowStartTime, windowEndTime time.Time
	var windowCount int64

	// Read the state
	err := p.db.View(func(tx *bolt.Tx) error {
		checkpointBucket := tx.Bucket([]byte(bucketCheckpoint))
		stateBytes := checkpointBucket.Get([]byte(keyReservoirState))
		if stateBytes == nil {
			return fmt.Errorf("no checkpoint state found")
		}

		// Manual deserialization to match our checkpoint format
		if len(stateBytes) < 32 { // 4 int64s = 32 bytes
			return fmt.Errorf("invalid checkpoint state data: too short")
		}

		buffer := bytes.NewReader(stateBytes)

		// Read state fields in the same order they were written
		if err := binary.Read(buffer, binary.BigEndian, &currentWindow); err != nil {
			return fmt.Errorf("failed to read current window: %w", err)
		}

		var windowStartUnix, windowEndUnix int64
		if err := binary.Read(buffer, binary.BigEndian, &windowStartUnix); err != nil {
			return fmt.Errorf("failed to read window start time: %w", err)
		}
		if err := binary.Read(buffer, binary.BigEndian, &windowEndUnix); err != nil {
			return fmt.Errorf("failed to read window end time: %w", err)
		}

		// Convert unix timestamps to time.Time
		windowStartTime = time.Unix(windowStartUnix, 0)
		windowEndTime = time.Unix(windowEndUnix, 0)

		if err := binary.Read(buffer, binary.BigEndian, &windowCount); err != nil {
			return fmt.Errorf("failed to read window count: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Check if the previous window is still valid
	now := time.Now()

	if now.After(windowEndTime) {
		// Previous window has expired, start a new one
		p.initializeWindowLocked()
		p.logger.Info("Previous sampling window expired, starting a new one")
		return nil
	}

	// Previous window still valid, load it
	p.currentWindow = currentWindow
	p.windowStartTime = windowStartTime
	p.windowEndTime = windowEndTime
	p.windowCount.Store(windowCount)

	// Load the reservoir spans in batches to avoid stack overflow
	p.reservoir = make(map[uint64]SpanWithResource)
	p.reservoirHashes = make([]uint64, 0, p.windowSize)

	// Process spans in manageable chunks to avoid stack issues
	spansLoaded := 0

	err = p.db.View(func(tx *bolt.Tx) error {
		reservoirBucket := tx.Bucket([]byte(bucketReservoir))
		windowBucket := reservoirBucket.Bucket([]byte(fmt.Sprintf("window_%d", p.currentWindow)))
		if windowBucket == nil {
			p.logger.Warn("No reservoir bucket found for window, resetting count",
				zap.Int64("window", p.currentWindow))
			// Reset count when no bucket is found
			p.windowCount.Store(0)
			return nil
		}

		// Iterate through keys in batches
		return windowBucket.ForEach(func(k, v []byte) error {
			if len(k) != 8 {
				return nil // Skip invalid keys
			}

			hash := binary.BigEndian.Uint64(k)

			// Deserialize the span
			spanWithRes, err := deserializeSpanWithResource(v)
			if err != nil {
				p.logger.Error("Failed to deserialize span from checkpoint",
					zap.Error(err),
					zap.Uint64("hash", hash))
				return nil // Continue with other spans
			}

			// Add to reservoir
			p.reservoir[hash] = spanWithRes
			p.reservoirHashes = append(p.reservoirHashes, hash)
			spansLoaded++

			// Log progress for large reservoirs
			if spansLoaded%100 == 0 {
				p.logger.Debug("Loading spans from checkpoint",
					zap.Int("loaded_so_far", spansLoaded))
			}

			return nil
		})
	})

	if err != nil {
		// Reset to safe values on error
		p.windowCount.Store(0)
		p.reservoir = make(map[uint64]SpanWithResource)
		p.reservoirHashes = make([]uint64, 0, p.windowSize)
		return err
	}

	// Validate and adjust window count if needed
	if spansLoaded == 0 {
		p.windowCount.Store(0)
		p.logger.Warn("No spans loaded from checkpoint, resetting window count to 0")
	} else if int64(spansLoaded) < p.windowCount.Load() {
		p.logger.Warn("Fewer spans loaded than expected, adjusting window count",
			zap.Int("loaded", spansLoaded),
			zap.Int64("expected", p.windowCount.Load()))

		// Ensure window count is at least the number of spans loaded
		if int64(spansLoaded) > p.windowCount.Load() {
			p.windowCount.Store(int64(spansLoaded))
		}
	}

	p.lastCheckpoint = now

	// Update metrics
	p.reservoirSizeGauge.Store(int64(len(p.reservoir)))
	p.windowCountGauge.Store(windowCount)

	p.logger.Info("Loaded previous state from checkpoint",
		zap.Int64("window", p.currentWindow),
		zap.Time("start", p.windowStartTime),
		zap.Time("end", p.windowEndTime),
		zap.Int("spans", len(p.reservoir)))

	return nil
}

// serializeSpanWithResource serializes a SpanWithResource to bytes
// This implementation avoids deep recursive copying to prevent stack overflow
// This implementation creates a minimal SpanWithResource to avoid excessive allocations

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

	// Iterate over all keys in the source nested bucket
	return sourceNestedBucket.ForEach(func(k, v []byte) error {
		// If the value is nil, it's a nested bucket
		if v == nil {
			// Handle nested buckets recursively
			return copyNestedBucket(sourceNestedBucket, destNestedBucket, k)
		}

		// Otherwise, it's a key-value pair
		return destNestedBucket.Put(k, v)
	})
}

// compactDatabase performs database compaction to reduce file size and improve performance
// It creates a temporary compacted copy of the database, then replaces the original with it
func (p *reservoirProcessor) compactDatabase() {
	// Skip if database is not initialized
	if p.db == nil {
		p.logger.Warn("Skipping database compaction - database not initialized")
		return
	}

	// Check if compaction is needed based on file size
	originalFile := p.config.CheckpointPath
	fi, err := os.Stat(originalFile)
	if err != nil {
		p.logger.Error("Failed to get database file info for compaction", zap.Error(err))
		return
	}

	currentSize := fi.Size()
	if currentSize < p.config.DbCompactionTargetSize {
		p.logger.Debug("Skipping database compaction - current size below target",
			zap.Int64("current_size", currentSize),
			zap.Int64("target_size", p.config.DbCompactionTargetSize))
		return
	}

	startTime := time.Now()
	p.logger.Info("Starting database compaction", zap.Int64("current_size", currentSize))

	// Create backup and temporary file paths
	backupFile := originalFile + ".bak"
	tempFile := originalFile + ".tmp"

	// Create a function to reopen the original DB in case of errors
	reopenOriginalDB := func() error {
		db, err := bolt.Open(originalFile, 0644, &bolt.Options{Timeout: 1 * time.Second})
		if err != nil {
			p.logger.Error("Failed to reopen original database after error", zap.Error(err))
			return fmt.Errorf("failed to reopen original database: %w", err)
		}

		p.lock.Lock()
		p.db = db
		p.lock.Unlock()
		p.logger.Info("Successfully reopened original database after error")
		return nil
	}

	// 1. Create a backup of the original file
	if err := copyFile(originalFile, backupFile); err != nil {
		p.logger.Error("Failed to create backup before compaction", zap.Error(err))
		return
	}

	// 2. Perform compaction to a temporary file
	compactionSuccess := false
	var sourceDb, newDb *bolt.DB

	func() {
		// Automatically handle cleanup in case of panics or other errors
		defer func() {
			// Close both databases safely if they're still open
			if sourceDb != nil {
				if err := sourceDb.Close(); err != nil {
					p.logger.Error("Failed to close source database", zap.Error(err))
				}
			}
			if newDb != nil {
				if err := newDb.Close(); err != nil {
					p.logger.Error("Failed to close new database", zap.Error(err))
				}
			}

			if !compactionSuccess {
				// Clean up temporary file if compaction failed
				if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
					p.logger.Error("Failed to remove temporary file", zap.Error(err), zap.String("file", tempFile))
				}
			}
		}()

		// Open the source database in read-only mode
		var err error
		sourceDb, err = bolt.Open(originalFile, 0644, &bolt.Options{ReadOnly: true, Timeout: 1 * time.Second})
		if err != nil {
			p.logger.Error("Failed to open source database for compaction", zap.Error(err))
			return
		}

		// Create a new database for compaction
		newDb, err = bolt.Open(tempFile, 0644, &bolt.Options{Timeout: 1 * time.Second})
		if err != nil {
			p.logger.Error("Failed to create new database for compaction", zap.Error(err))
			return
		}

		// Copy all data from source to new database
		err = sourceDb.View(func(sourceTx *bolt.Tx) error {
			return newDb.Update(func(destTx *bolt.Tx) error {
				return sourceTx.ForEach(func(name []byte, sourceBucket *bolt.Bucket) error {
					// Create the bucket in the destination
					destBucket, err := destTx.CreateBucketIfNotExists(name)
					if err != nil {
						return fmt.Errorf("error creating bucket %s: %w", string(name), err)
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

		if err != nil {
			p.logger.Error("Failed to copy data during compaction", zap.Error(err))
			return
		}

		// Close both databases safely
		if err = sourceDb.Close(); err != nil {
			p.logger.Error("Failed to close source database", zap.Error(err))
			return
		}
		sourceDb = nil

		if err = newDb.Close(); err != nil {
			p.logger.Error("Failed to close new database", zap.Error(err))
			return
		}
		newDb = nil

		// Mark compaction as successful
		compactionSuccess = true
	}()

	// If compaction failed, reopen the original DB and return
	if !compactionSuccess {
		p.logger.Error("Compaction failed, reopening original database")
		if err := reopenOriginalDB(); err != nil {
			p.logger.Error("Failed to reopen original database", zap.Error(err))
		}
		// Clean up temporary files
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			p.logger.Error("Failed to remove temporary file", zap.Error(err), zap.String("file", tempFile))
		}
		if err := os.Remove(backupFile); err != nil && !os.IsNotExist(err) {
			p.logger.Error("Failed to remove backup file", zap.Error(err), zap.String("file", backupFile))
		}
		return
	}

	// 3. Replace the original file with the compacted one
	if err := os.Rename(tempFile, originalFile); err != nil {
		p.logger.Error("Failed to replace database with compacted version", zap.Error(err))

		// Try to restore from backup
		if err := os.Rename(backupFile, originalFile); err != nil {
			p.logger.Error("Failed to restore from backup after compaction error", zap.Error(err))
		} else {
			p.logger.Info("Successfully restored database from backup")
		}

		if err := reopenOriginalDB(); err != nil {
			p.logger.Error("Failed to reopen original database", zap.Error(err))
		}
		return
	}

	// Remove the backup file if rename was successful
	if err := os.Remove(backupFile); err != nil && !os.IsNotExist(err) {
		p.logger.Error("Failed to remove backup file after successful compaction", zap.Error(err), zap.String("file", backupFile))
	}

	// 4. Reopen the compacted database
	if db, err := bolt.Open(originalFile, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
		p.logger.Error("Failed to open compacted database", zap.Error(err))

		// Critical error - try to restore from backup
		if err := os.Rename(backupFile, originalFile); err != nil {
			p.logger.Error("Failed to restore from backup after reopen error", zap.Error(err))
		}

		if err := reopenOriginalDB(); err != nil {
			p.logger.Error("Failed to reopen original database", zap.Error(err))
		}
	} else {
		p.lock.Lock()
		p.db = db
		p.lock.Unlock()
		p.logger.Info("Successfully reopened compacted database")

		// Get new size and update metrics
		if fi, err := os.Stat(originalFile); err == nil {
			newSize := fi.Size()
			p.reservoirDbSizeGauge.Store(newSize)

			p.logger.Info("Database compaction completed",
				zap.Int64("original_size", currentSize),
				zap.Int64("new_size", newSize),
				zap.Float64("reduction_pct", float64(currentSize-newSize)*100/float64(currentSize)),
				zap.Duration("duration", time.Since(startTime)))

			p.compactionCountCounter.Inc()
		}
	}
}

// copyFile is a helper function to copy a file
func copyFile(src, dst string) error {
	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close source file: %w", closeErr)
		}
	}()

	// Create destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() {
		if closeErr := destFile.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close destination file: %w", closeErr)
		}
	}()

	// Copy the contents
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Sync to ensure data is written to disk
	if err = destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}
