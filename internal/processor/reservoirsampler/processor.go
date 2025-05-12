package reservoirsampler

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

// reservoirProcessor implements a trace-aware reservoir sampler processor
type reservoirProcessor struct {
	// Required interfaces for the processor
	component.StartFunc
	component.ShutdownFunc

	// Core dependencies
	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *zap.Logger
	config    *Config

	// Next consumer in the pipeline
	nextConsumer consumer.Traces

	// Components
	metricsManager    *MetricsManager
	windowManager     *WindowManager
	reservoir         *Reservoir
	checkpointManager CheckpointManager
	traceBuffer       *TraceBuffer
	
	// Background tasks
	checkpointTicker *time.Ticker
	compactionCron   *cron.Cron
	stopChan         chan struct{}
}

// Ensure the processor implements required interfaces
var _ processor.Traces = (*reservoirProcessor)(nil)
var _ component.Component = (*reservoirProcessor)(nil)

// newReservoirProcessor creates a new reservoir sampler processor
func newReservoirProcessor(
	ctx context.Context,
	set component.TelemetrySettings,
	cfg *Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	processorCtx, processorCancel := context.WithCancel(ctx)
	logger := set.Logger

	// Create a new metrics manager
	metricsManager := NewMetricsManager(processorCtx, set.MeterProvider.Meter("reservoirsampler"))

	// Create a processor instance
	p := &reservoirProcessor{
		ctx:            processorCtx,
		ctxCancel:      processorCancel,
		logger:         logger,
		config:         cfg,
		nextConsumer:   nextConsumer,
		metricsManager: metricsManager,
		stopChan:       make(chan struct{}),
	}

	// Create window manager with rollover callback
	p.windowManager = NewWindowManager(cfg.WindowDuration, p.onWindowRollover, logger)

	// Create reservoir
	p.reservoir = NewReservoir(
		cfg.SizeK,
		p.windowManager,
		metricsManager.GetReservoirSizeGauge(),
		metricsManager.GetSampledSpansCounter(),
		logger,
	)

	// Create trace buffer if trace-aware mode is enabled
	if cfg.TraceAware {
		p.traceBuffer = NewTraceBuffer(cfg.TraceBufferMaxSize, cfg.TraceBufferTimeout, logger)
		p.traceBuffer.SetEvictionCounter(metricsManager.GetLruEvictionsCounter())
		logger.Info("Trace-aware sampling enabled",
			zap.Int("buffer_size", cfg.TraceBufferMaxSize),
			zap.Duration("buffer_timeout", cfg.TraceBufferTimeout))
	}

	// Set up checkpoint manager if checkpoint path is specified
	if cfg.CheckpointPath != "" {
		var err error
		p.checkpointManager, err = NewBadgerCheckpointManager(
			cfg.CheckpointPath,
			cfg.DbCompactionTargetSize,
			metricsManager.GetCheckpointAgeGauge(),
			metricsManager.GetReservoirDbSizeGauge(),
			metricsManager.GetCompactionCountCounter(),
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create checkpoint manager: %w", err)
		}

		// Create checkpoint ticker
		p.checkpointTicker = time.NewTicker(cfg.CheckpointInterval)
		logger.Info("Checkpoint storage initialized",
			zap.String("path", cfg.CheckpointPath),
			zap.Duration("interval", cfg.CheckpointInterval))
	}

	// Set up compaction cron if configured
	if cfg.DbCompactionScheduleCron != "" && cfg.DbCompactionTargetSize > 0 && p.checkpointManager != nil {
		p.compactionCron = cron.New()
		_, err := p.compactionCron.AddFunc(cfg.DbCompactionScheduleCron, func() {
			if err := p.checkpointManager.Compact(); err != nil {
				logger.Error("DB compaction failed", zap.Error(err))
			}
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
		zap.Duration("window", cfg.WindowDuration),
		zap.Bool("trace_aware", cfg.TraceAware))

	return p, nil
}

// Start implements the Component interface
func (p *reservoirProcessor) Start(ctx context.Context, host component.Host) error {
	p.logger.Info("Starting reservoir sampler processor")

	// Register metrics
	if err := p.metricsManager.RegisterMetrics(); err != nil {
		p.logger.Error("Failed to register metrics", zap.Error(err))
	}

	// Try to load previous state from checkpoint
	if p.checkpointManager != nil {
		windowID, startTime, endTime, windowCount, spans, err := p.checkpointManager.LoadCheckpoint()
		if err != nil {
			p.logger.Error("Failed to load checkpoint, starting with empty reservoir", zap.Error(err))
		} else {
			// Check if the window is still valid (not expired)
			now := time.Now()
			if now.Before(endTime) {
				// Restore window state
				p.windowManager.SetState(windowID, startTime, endTime, windowCount)

				// Restore reservoir spans
				for hash, span := range spans {
					p.reservoir.AddSpan(span.Span, span.Resource, span.Scope)
				}

				p.logger.Info("Loaded previous state from checkpoint",
					zap.Int64("window", windowID),
					zap.Time("start", startTime),
					zap.Time("end", endTime),
					zap.Int("spans", len(spans)))
			} else {
				p.logger.Info("Previous window expired, starting with empty reservoir")
			}
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
	if p.traceBuffer != nil {
		go p.processTraceBuffer()
	}

	return nil
}

// Shutdown implements the Component interface
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

	// Final checkpoint
	if p.checkpointManager != nil {
		// Get the current window state
		windowID, startTime, endTime, count := p.windowManager.GetCurrentState()

		// Checkpoint the current state
		if err := p.checkpointManager.Checkpoint(
			windowID,
			startTime,
			endTime,
			count,
			p.reservoir.GetAllSpans(),
		); err != nil {
			p.logger.Error("Failed to perform final checkpoint", zap.Error(err))
		}

		// Close the checkpoint manager
		if err := p.checkpointManager.Close(); err != nil {
			p.logger.Error("Failed to close checkpoint manager", zap.Error(err))
		}
	}

	return nil
}

// ConsumeTraces implements the processor.Traces interface
func (p *reservoirProcessor) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	startTime := time.Now()
	var err error

	// Check if we need to roll over to a new window
	p.windowManager.CheckRollover()

	// Process through the appropriate mode
	if p.config.TraceAware {
		err = p.consumeTracesAware(ctx, traces)
	} else {
		err = p.consumeTracesSimple(ctx, traces)
	}

	// Log processing time for large trace batches
	latency := time.Since(startTime)
	if traces.SpanCount() > 1000 {
		p.logger.Debug("Processed large trace batch",
			zap.Int("span_count", traces.SpanCount()),
			zap.Duration("latency", latency))
	}

	return err
}

// consumeTracesSimple implements standard reservoir sampling
func (p *reservoirProcessor) consumeTracesSimple(ctx context.Context, traces ptrace.Traces) error {
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
				p.reservoir.AddSpan(span, resource, scope)
			}
		}
	}

	return nil
}

// consumeTracesAware implements trace-aware sampling
func (p *reservoirProcessor) consumeTracesAware(ctx context.Context, traces ptrace.Traces) error {
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
				p.traceBuffer.AddSpan(span, resource, scope)
			}
		}
	}

	return nil
}

// onWindowRollover is called when a new window starts
func (p *reservoirProcessor) onWindowRollover() {
	// Export the current reservoir
	traces, err := p.reservoir.Export(p.ctx)
	if err != nil {
		p.logger.Error("Failed to export reservoir", zap.Error(err))
		return
	}

	// Only export if there are spans to export
	if traces.SpanCount() > 0 {
		p.logger.Info("Exporting reservoir", zap.Int("span_count", traces.SpanCount()))
		
		// Export to the next consumer
		if err := p.nextConsumer.ConsumeTraces(p.ctx, traces); err != nil {
			p.logger.Error("Failed to export traces to next consumer", zap.Error(err))
		}
	}

	// Reset the reservoir for the new window
	p.reservoir.Reset()

	// Process any complete traces in the trace buffer
	if p.traceBuffer != nil {
		completedTraces := p.traceBuffer.GetCompletedTraces()
		for _, traces := range completedTraces {
			if err := p.consumeTracesSimple(p.ctx, traces); err != nil {
				p.logger.Error("Failed to process completed trace after window rollover", zap.Error(err))
			}
		}
	}
}

// processTraceBuffer periodically processes the trace buffer to add complete traces to the reservoir
func (p *reservoirProcessor) processTraceBuffer() {
	// Create a ticker with 1/10th of the trace timeout interval
	checkInterval := p.config.TraceBufferTimeout / 10
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

// checkpointLoop runs a background goroutine to periodically checkpoint
func (p *reservoirProcessor) checkpointLoop() {
	for {
		select {
		case <-p.checkpointTicker.C:
			// Get the current window state
			windowID, startTime, endTime, count := p.windowManager.GetCurrentState()

			// Checkpoint the current state
			if err := p.checkpointManager.Checkpoint(
				windowID,
				startTime,
				endTime,
				count,
				p.reservoir.GetAllSpans(),
			); err != nil {
				p.logger.Error("Failed to checkpoint", zap.Error(err))
			}

			// Update checkpoint metrics
			p.checkpointManager.UpdateMetrics()

		case <-p.stopChan:
			return
		}
	}
}

// Capabilities implements the processor.Traces interface
func (p *reservoirProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// ForceExport exports the current reservoir contents (for testing)
func (p *reservoirProcessor) ForceExport() error {
	traces, err := p.reservoir.Export(p.ctx)
	if err != nil {
		return err
	}

	if traces.SpanCount() > 0 {
		if err := p.nextConsumer.ConsumeTraces(p.ctx, traces); err != nil {
			return err
		}
	}

	return nil
}