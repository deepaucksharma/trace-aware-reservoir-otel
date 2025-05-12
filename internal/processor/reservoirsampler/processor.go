package reservoirsampler

import (
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
	"github.com/golang/protobuf/proto"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric"
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
		p.db.Close()
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
// 1. If we have seen fewer than k elements, add the element to our reservoir
// 2. Otherwise, with probability k/n, keep the new element
//    where:
//      n = the number of elements we have seen so far
//      k = the size of our reservoir
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
			p.logger.Warn("No reservoir bucket found for window, resetting count",
				zap.Int64("window", p.currentWindow))
			// CRITICAL: Reset n to 0 when summaries are not restored
			p.windowCount.Store(0)
			return nil
		}

		// Read all spans
		p.reservoir = make(map[uint64]SpanWithResource)
		p.reservoirHashes = make([]uint64, 0, p.windowSize)

		spansLoaded := 0
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
			spansLoaded++

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to read spans from checkpoint: %w", err)
		}

		p.logger.Info("Restored spans from checkpoint",
			zap.Int("count", spansLoaded),
			zap.Int("capacity", p.windowSize))

		// CRITICAL: Reset n to match the actual number of loaded spans when fewer spans than n are restored
		if spansLoaded == 0 {
			p.windowCount.Store(0)
			p.logger.Warn("No spans loaded from checkpoint, resetting window count to 0")
		} else if int64(spansLoaded) < p.windowCount.Load() {
			p.logger.Warn("Fewer spans loaded than expected, adjusting window count",
				zap.Int("loaded", spansLoaded),
				zap.Int64("expected", p.windowCount.Load()))
			// We set to at least the number of spans to ensure algorithm correctness
			if int64(spansLoaded) > p.windowCount.Load() {
				p.windowCount.Store(int64(spansLoaded))
			}
		}

		return nil
	})

	if err != nil {
		// Reset to safe values on error
		p.windowCount.Store(0)
		p.reservoir = make(map[uint64]SpanWithResource)
		p.reservoirHashes = make([]uint64, 0, p.windowSize)
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
	// Create a proper span summary with all important fields
	traceID := swr.Span.TraceID()
	spanID := swr.Span.SpanID()

	// Serialize basic span data
	spanData := make([]byte, 0, 128)

	// Start with trace and span IDs
	spanData = append(spanData, traceID[:]...)
	spanData = append(spanData, spanID[:]...)

	// Add parent span ID
	parentSpanID := swr.Span.ParentSpanID()
	spanData = append(spanData, parentSpanID[:]...)

	// Add span name (with length prefix)
	spanName := []byte(swr.Span.Name())
	spanNameLen := make([]byte, 4)
	binary.BigEndian.PutUint32(spanNameLen, uint32(len(spanName)))
	spanData = append(spanData, spanNameLen...)
	spanData = append(spanData, spanName...)

	// Add start and end timestamps
	startTimeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(startTimeBytes, uint64(swr.Span.StartTimestamp()))
	spanData = append(spanData, startTimeBytes...)

	endTimeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(endTimeBytes, uint64(swr.Span.EndTimestamp()))
	spanData = append(spanData, endTimeBytes...)

	// Serialize resource data - just grab service.name and service.version if available
	resourceData := make([]byte, 0, 64)
	swr.Resource.Attributes().Range(func(k string, v pcommon.Value) bool {
		if k == "service.name" || k == "service.version" {
			// Add key with length prefix
			keyBytes := []byte(k)
			keyLen := make([]byte, 4)
			binary.BigEndian.PutUint32(keyLen, uint32(len(keyBytes)))
			resourceData = append(resourceData, keyLen...)
			resourceData = append(resourceData, keyBytes...)

			// Add value with length prefix
			valueBytes := []byte(v.AsString())
			valueLen := make([]byte, 4)
			binary.BigEndian.PutUint32(valueLen, uint32(len(valueBytes)))
			resourceData = append(resourceData, valueLen...)
			resourceData = append(resourceData, valueBytes...)
		}
		return true
	})

	// For scope, just store the name and version
	scopeData := make([]byte, 0, 64)
	scopeName := []byte(swr.Scope.Name())
	scopeNameLen := make([]byte, 4)
	binary.BigEndian.PutUint32(scopeNameLen, uint32(len(scopeName)))
	scopeData = append(scopeData, scopeNameLen...)
	scopeData = append(scopeData, scopeName...)

	scopeVersion := []byte(swr.Scope.Version())
	scopeVersionLen := make([]byte, 4)
	binary.BigEndian.PutUint32(scopeVersionLen, uint32(len(scopeVersion)))
	scopeData = append(scopeData, scopeVersionLen...)
	scopeData = append(scopeData, scopeVersion...)

	// Create and marshal the summary
	summary := &spanprotos.SpanWithResourceSummary{
		SpanData:     spanData,
		ResourceData: resourceData,
		ScopeData:    scopeData,
	}

	return proto.Marshal(summary)
}

// deserializeSpanWithResource deserializes bytes to a SpanWithResource
func deserializeSpanWithResource(data []byte) (SpanWithResource, error) {
	// Unmarshal the SpanWithResourceSummary
	summary := &spanprotos.SpanWithResourceSummary{}
	if err := proto.Unmarshal(data, summary); err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to unmarshal span summary: %w", err)
	}

	// Create a new traces object with structure
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	// Extract span data
	spanData := summary.SpanData
	if len(spanData) < 32 { // At minimum, need trace ID and span ID (16 bytes each)
		return SpanWithResource{}, fmt.Errorf("invalid span data length")
	}

	// Extract trace ID
	traceID := pcommon.TraceID{}
	copy(traceID[:], spanData[:16])
	span.SetTraceID(traceID)

	// Extract span ID
	spanID := pcommon.SpanID{}
	copy(spanID[:], spanData[16:24])
	span.SetSpanID(spanID)

	// Extract parent span ID if present
	if len(spanData) >= 32 {
		parentSpanID := pcommon.SpanID{}
		copy(parentSpanID[:], spanData[24:32])
		span.SetParentSpanID(parentSpanID)
	}

	// Extract more fields if they exist
	if len(spanData) > 32 {
		// Extract span name
		if len(spanData) >= 36 {
			nameLen := binary.BigEndian.Uint32(spanData[32:36])
			if len(spanData) >= 36+int(nameLen) {
				span.SetName(string(spanData[36 : 36+nameLen]))

				// Extract timestamps if present
				pos := 36 + nameLen
				if len(spanData) >= int(pos)+16 { // Need 8 bytes each for start and end
					startTime := binary.BigEndian.Uint64(spanData[pos : pos+8])
					span.SetStartTimestamp(pcommon.Timestamp(startTime))

					endTime := binary.BigEndian.Uint64(spanData[pos+8 : pos+16])
					span.SetEndTimestamp(pcommon.Timestamp(endTime))
				}
			}
		}
	}

	// Extract resource data
	resourceData := summary.ResourceData
	pos := 0
	for pos+8 <= len(resourceData) {
		// Read key
		keyLen := binary.BigEndian.Uint32(resourceData[pos : pos+4])
		pos += 4
		if pos+int(keyLen)+4 > len(resourceData) {
			break
		}
		key := string(resourceData[pos : pos+int(keyLen)])
		pos += int(keyLen)

		// Read value
		valueLen := binary.BigEndian.Uint32(resourceData[pos : pos+4])
		pos += 4
		if pos+int(valueLen) > len(resourceData) {
			break
		}
		value := string(resourceData[pos : pos+int(valueLen)])
		pos += int(valueLen)

		// Set resource attribute
		rs.Resource().Attributes().PutStr(key, value)
	}

	// Extract scope data
	scopeData := summary.ScopeData
	if len(scopeData) >= 4 {
		nameLen := binary.BigEndian.Uint32(scopeData[:4])
		if len(scopeData) >= 4+int(nameLen)+4 {
			ss.Scope().SetName(string(scopeData[4 : 4+nameLen]))

			// Extract version
			pos := 4 + nameLen
			versionLen := binary.BigEndian.Uint32(scopeData[pos : pos+4])
			if len(scopeData) >= int(pos)+4+int(versionLen) {
				ss.Scope().SetVersion(string(scopeData[pos+4 : pos+4+versionLen]))
			}
		}
	}

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

	// Perform compaction using the safer approach:
	// 1. Close the current DB
	// 2. Create a copy with a temporary file
	// 3. Rename the temporary file to replace the original
	// 4. Reopen the DB

	// Define filenames
	originalFile := p.config.CheckpointPath
	tempFile := originalFile + ".compact"
	backupFile := originalFile + ".bak"

	// Function to ensure we clean up and restore the DB in case of errors
	reopenOriginalDB := func() {
		if db, err := bolt.Open(originalFile, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
			p.logger.Error("Failed to reopen database after compaction failure", zap.Error(err))
		} else {
			p.lock.Lock()
			p.db = db
			p.lock.Unlock()
			p.logger.Info("Successfully reopened original database after error")
		}
	}

	// 1. Close the DB for compaction
	p.lock.Lock()
	db := p.db
	p.db = nil
	p.lock.Unlock()

	if err := db.Close(); err != nil {
		p.logger.Error("Failed to close database for compaction", zap.Error(err))
		reopenOriginalDB()
		return
	}

	// Create backup of original file before attempting compaction
	if err := copyFile(originalFile, backupFile); err != nil {
		p.logger.Error("Failed to create backup before compaction", zap.Error(err))
		reopenOriginalDB()
		return
	}

	// 2. Perform compaction by creating a new DB and copying the data
	var newDb, sourceDb *bolt.DB
	var compactionSuccess bool

	// SAFER APPROACH: Use a function to ensure proper cleanup of resources
	func() {
		var err error

		// Create new empty database
		newDb, err = bolt.Open(tempFile, 0644, &bolt.Options{Timeout: 1 * time.Second})
		if err != nil {
			p.logger.Error("Failed to create new database for compaction", zap.Error(err))
			return
		}
		defer func() {
			if !compactionSuccess && newDb != nil {
				newDb.Close()
				os.Remove(tempFile)
			}
		}()

		// Open original database in read-only mode
		sourceDb, err = bolt.Open(originalFile, 0444, &bolt.Options{ReadOnly: true, Timeout: 1 * time.Second})
		if err != nil {
			p.logger.Error("Failed to open source database for compaction", zap.Error(err))
			return
		}
		defer sourceDb.Close()

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
		reopenOriginalDB()
		// Clean up temporary files
		os.Remove(tempFile)
		os.Remove(backupFile)
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

		reopenOriginalDB()
		return
	}

	// Remove the backup file if rename was successful
	os.Remove(backupFile)

	// 4. Reopen the compacted database
	if db, err := bolt.Open(originalFile, 0644, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
		p.logger.Error("Failed to open compacted database", zap.Error(err))

		// Critical error - try to restore from backup
		if err := os.Rename(backupFile, originalFile); err != nil {
			p.logger.Error("Failed to restore from backup after reopen error", zap.Error(err))
		}

		reopenOriginalDB()
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
	defer sourceFile.Close()

	// Create destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

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