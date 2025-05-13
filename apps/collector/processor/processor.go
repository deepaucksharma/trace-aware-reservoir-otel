package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/deepaucksharma/reservoir"
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
	config    *reservoir.Config

	// Next consumer in the pipeline
	nextConsumer consumer.Traces

	// Components
	metricsManager    *MetricsManager
	window            reservoir.Window
	reservoir         reservoir.Reservoir
	checkpointManager reservoir.CheckpointManager
	traceBuffer       reservoir.TraceAggregator
	otelAdapter       adapter.OTelAdapter
	
	// Background tasks
	checkpointTicker *time.Ticker
	compactionCron   *cron.Cron
	stopChan         chan struct{}
}

// Ensure the processor implements required interfaces
var _ processor.Traces = (*reservoirProcessor)(nil)
var _ component.Component = (*reservoirProcessor)(nil)

// NewReservoirProcessor creates a new reservoir sampler processor
func NewReservoirProcessor(
	ctx context.Context,
	set component.TelemetrySettings,
	cfg *reservoir.Config,
	nextConsumer consumer.Traces,
	checkpointManager reservoir.CheckpointManager,
	metricsManager *MetricsManager,
	otelAdapter adapter.OTelAdapter,
) (processor.Traces, error) {
	processorCtx, processorCancel := context.WithCancel(ctx)
	logger := set.Logger

	// Create a processor instance
	p := &reservoirProcessor{
		ctx:              processorCtx,
		ctxCancel:        processorCancel,
		logger:           logger,
		config:           cfg,
		nextConsumer:     nextConsumer,
		metricsManager:   metricsManager,
		checkpointManager: checkpointManager,
		otelAdapter:      otelAdapter,
		stopChan:         make(chan struct{}),
	}

	// Create window manager with rollover callback
	p.window = reservoir.NewTimeWindow(cfg.WindowDuration)
	p.window.SetRolloverCallback(p.onWindowRollover)

	// Create reservoir
	p.reservoir = reservoir.NewAlgorithmR(
		cfg.SizeK,
		p.metricsManager,
	)

	// Create trace buffer if trace-aware mode is enabled
	if cfg.TraceAware {
		p.traceBuffer = reservoir.NewTraceBuffer(
			cfg.TraceBufferMaxSize,
			cfg.TraceBufferTimeout,
			p.metricsManager,
		)
		
		logger.Info("Trace-aware sampling enabled",
			zap.Int("buffer_size", cfg.TraceBufferMaxSize),
			zap.Duration("buffer_timeout", cfg.TraceBufferTimeout))
	}

	// Set up checkpoint ticker if checkpoint manager is provided
	if p.checkpointManager != nil {
		p.checkpointTicker = time.NewTicker(cfg.CheckpointInterval)
		
		logger.Info("Checkpoint storage initialized",
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
				p.window.SetState(windowID, startTime, endTime, windowCount)

				// Restore reservoir spans
				for hash, span := range spans {
					p.reservoir.AddSpan(span.Span)
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
		windowID, startTime, endTime := p.window.Current()
		count := 0 // TODO: Get proper count

		// Checkpoint the current state
		allSpans := p.reservoir.GetSample()
		spanMap := make(map[string]reservoir.SpanWithResource, len(allSpans))
		
		for _, span := range allSpans {
			key := span.ID + "-" + span.TraceID
			spanMap[key] = reservoir.SpanWithResource{
				Span:     span,
				Resource: span.ResourceInfo,
				Scope:    span.ScopeInfo,
			}
		}
		
		if err := p.checkpointManager.Checkpoint(
			windowID,
			startTime,
			endTime,
			int64(count),
			spanMap,
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
	p.window.CheckRollover()

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
	// Convert OTEL traces to our domain model
	spans := p.otelAdapter.ConvertTraces(traces)
	
	// Add each span to the reservoir
	for _, span := range spans {
		p.reservoir.AddSpan(span)
	}

	return nil
}

// consumeTracesAware implements trace-aware sampling
func (p *reservoirProcessor) consumeTracesAware(ctx context.Context, traces ptrace.Traces) error {
	// Convert OTEL traces to our domain model
	spans := p.otelAdapter.ConvertTraces(traces)
	
	// Add each span to the trace buffer
	for _, span := range spans {
		p.traceBuffer.AddSpan(span)
	}

	return nil
}

// onWindowRollover is called when a new window starts
func (p *reservoirProcessor) onWindowRollover() {
	// Get the current reservoir sample
	sampleSpans := p.reservoir.GetSample()
	
	// Only export if there are spans to export
	if len(sampleSpans) > 0 {
		p.logger.Info("Exporting reservoir", zap.Int("span_count", len(sampleSpans)))
		
		// Convert back to OTEL format
		traces := p.otelAdapter.ConvertToOTEL(sampleSpans).(ptrace.Traces)
		
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
		for _, traceBatch := range completedTraces {
			// Add each span in the trace to the reservoir
			for _, span := range traceBatch {
				p.reservoir.AddSpan(span)
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

			for _, traceBatch := range completedTraces {
				// Add each span in the trace to the reservoir
				for _, span := range traceBatch {
					p.reservoir.AddSpan(span)
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
			windowID, startTime, endTime := p.window.Current()
			count := 0 // TODO: Get proper count

			// Get all spans from the reservoir with their keys
			allSpans := p.reservoir.GetSample()
			spanMap := make(map[string]reservoir.SpanWithResource, len(allSpans))
			
			for _, span := range allSpans {
				key := span.ID + "-" + span.TraceID
				spanMap[key] = reservoir.SpanWithResource{
					Span:     span,
					Resource: span.ResourceInfo,
					Scope:    span.ScopeInfo,
				}
			}
			
			// Checkpoint the current state
			if err := p.checkpointManager.Checkpoint(
				windowID,
				startTime,
				endTime,
				int64(count),
				spanMap,
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
	sampleSpans := p.reservoir.GetSample()
	if len(sampleSpans) > 0 {
		traces := p.otelAdapter.ConvertToOTEL(sampleSpans).(ptrace.Traces)
		if err := p.nextConsumer.ConsumeTraces(p.ctx, traces); err != nil {
			return err
		}
	}
	return nil
}
