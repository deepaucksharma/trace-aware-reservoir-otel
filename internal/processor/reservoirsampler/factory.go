package reservoirsampler

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
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

// ForceReservoirExport is a test helper that triggers export of the current reservoir.
// This is used in integration tests to force the processor to export spans.
func ForceReservoirExport(p processor.Traces) error {
	if rp, ok := p.(*reservoirProcessor); ok {
		return rp.ForceExport()
	}
	return nil
}