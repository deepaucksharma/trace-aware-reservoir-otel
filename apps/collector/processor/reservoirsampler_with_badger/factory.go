package reservoirsampler_with_badger

import (
	"context"

	"github.com/deepaucksharma/reservoir"
	"github.com/deepaucksharma/trace-aware-reservoir-otel/apps/collector/adapter"
	"github.com/deepaucksharma/trace-aware-reservoir-otel/apps/collector/persistence"
	"github.com/deepaucksharma/trace-aware-reservoir-otel/apps/collector/processor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

// NewFactoryWithBadger creates a new processor factory for the reservoir sampler
// with BadgerDB checkpoint manager
func NewFactoryWithBadger() processor.Factory {
	return processor.NewFactory(
		component.MustNewType("reservoir_sampler"),
		reservoir.CreateDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelBeta),
	)
}

// createTracesProcessor creates a trace processor based on this config
func createTracesProcessor(
	ctx context.Context,
	ps processor.Settings,
	cfg component.Config,
	nc consumer.Traces,
) (processor.Traces, error) {
	// Cast to the core config
	coreCfg := cfg.(*reservoir.Config)
	logger := ps.TelemetrySettings.Logger

	// Create metrics manager
	metricsManager := processor.NewMetricsManager(
		ctx,
		ps.TelemetrySettings.MeterProvider.Meter("reservoirsampler"),
	)

	// Register metrics
	if err := metricsManager.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", zap.Error(err))
	}

	// Create OpenTelemetry adapter
	otelAdapter := adapter.NewOTelPDataAdapter()

	// Create checkpoint manager if checkpoint path is specified
	var checkpointManager reservoir.CheckpointManager
	if coreCfg.CheckpointPath != "" {
		var err error
		checkpointManager, err = persistence.NewBadgerCheckpointManager(
			coreCfg.CheckpointPath,
			coreCfg.DbCompactionTargetSize,
			metricsManager.GetCheckpointAgeGauge(),
			metricsManager.GetReservoirDbSizeGauge(),
			metricsManager.GetCompactionCountCounter(),
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create checkpoint manager: %w", err)
		}
	} else {
		// Create a no-op checkpoint manager
		checkpointManager = processor.NewNilCheckpointManager()
	}

	// Create the processor
	return processor.NewReservoirProcessor(
		ctx,
		ps.TelemetrySettings,
		coreCfg,
		nc,
		checkpointManager,
		metricsManager,
		otelAdapter,
	)
}
