package reservoirsampler

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

// NewFactory returns a new factory for the reservoir sampler processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType("reservoir_sampler"),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelBeta),
	)
}

// createTracesProcessor creates a trace processor based on this config.
func createTracesProcessor(
	ctx context.Context,
	params processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	return newReservoirProcessor(ctx, params.TelemetrySettings, cfg.(*Config), nextConsumer)
}

// CreateTracesProcessorForTesting exposes the createTracesProcessor function for testing purposes.
// This allows integration tests to create processors directly without going through the factory.
//
// This is exported for use in integration tests.
func CreateTracesProcessorForTesting(
	ctx context.Context,
	params processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	return createTracesProcessor(ctx, params, cfg, nextConsumer)
}

// ForceReservoirExport is a test helper that triggers export of the current reservoir.
// This is used in integration tests to force the processor to export spans.
//
// This is exported for use in integration tests.
func ForceReservoirExport(p processor.Traces) {
	if rp, ok := p.(*reservoirProcessor); ok {
		rp.lock.Lock()
		defer rp.lock.Unlock()

		// Process any complete traces from the trace buffer
		if rp.traceBuffer != nil {
			completedTraces := rp.traceBuffer.GetCompletedTraces()
			if len(completedTraces) > 0 {
				rp.logger.Debug("Processing completed traces for test",
					zap.Int("count", len(completedTraces)))
			}

			for _, traces := range completedTraces {
				// Process each trace with consumeTracesSimple
				if err := rp.consumeTracesSimple(rp.ctx, traces); err != nil {
					rp.logger.Error("Failed to process completed trace", zap.Error(err))
				}
			}
		}

		// Log the window rollover
		rp.logger.Info("Forced window rollover for testing")

		// Initialize a new window
		rp.initializeWindowLocked()

		// Process the trace buffer again after window rollover
		if rp.traceBuffer != nil {
			completedTraces := rp.traceBuffer.GetCompletedTraces()
			if len(completedTraces) > 0 {
				rp.logger.Debug("Processing completed traces after window rollover",
					zap.Int("count", len(completedTraces)))
			}

			for _, traces := range completedTraces {
				if err := rp.consumeTracesSimple(rp.ctx, traces); err != nil {
					rp.logger.Error("Failed to process completed trace after rollover", zap.Error(err))
				}
			}
		}
	}
}
