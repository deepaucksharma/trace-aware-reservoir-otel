package reservoirsampler

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
	"go.uber.org/atomic"
)

// MetricsManager handles registration and updates of metrics
type MetricsManager struct {
	// Metric instruments
	reservoirSizeGauge     *atomic.Int64
	windowCountGauge       *atomic.Int64
	checkpointAgeGauge     *atomic.Int64
	reservoirDbSizeGauge   *atomic.Int64
	compactionCountCounter *atomic.Int64
	lruEvictionsCounter    *atomic.Int64
	sampledSpansCounter    *atomic.Int64
	
	// Context and meter
	metricCtx context.Context
	meter     metric.Meter
}

// NewMetricsManager creates a new metrics manager
func NewMetricsManager(ctx context.Context, meter metric.Meter) *MetricsManager {
	return &MetricsManager{
		reservoirSizeGauge:     atomic.NewInt64(0),
		windowCountGauge:       atomic.NewInt64(0),
		checkpointAgeGauge:     atomic.NewInt64(0),
		reservoirDbSizeGauge:   atomic.NewInt64(0),
		compactionCountCounter: atomic.NewInt64(0),
		lruEvictionsCounter:    atomic.NewInt64(0),
		sampledSpansCounter:    atomic.NewInt64(0),
		metricCtx:              ctx,
		meter:                  meter,
	}
}

// RegisterMetrics registers all metrics with the meter
func (m *MetricsManager) RegisterMetrics() error {
	var err error
	
	// Register the reservoir size gauge
	_, err = m.meter.Int64ObservableGauge(
		"reservoir_sampler.reservoir_size",
		metric.WithDescription("Number of spans currently in the reservoir"),
		metric.WithUnit("{spans}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.reservoirSizeGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register reservoir size gauge: %w", err)
	}
	
	// Register the window count gauge
	_, err = m.meter.Int64ObservableGauge(
		"reservoir_sampler.window_count",
		metric.WithDescription("Total number of spans seen in the current window"),
		metric.WithUnit("{spans}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.windowCountGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register window count gauge: %w", err)
	}
	
	// Register the checkpoint age gauge
	_, err = m.meter.Int64ObservableGauge(
		"reservoir_sampler.checkpoint_age",
		metric.WithDescription("Age of the last checkpoint in seconds"),
		metric.WithUnit("s"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.checkpointAgeGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register checkpoint age gauge: %w", err)
	}
	
	// Register the DB size gauge
	_, err = m.meter.Int64ObservableGauge(
		"reservoir_sampler.db_size",
		metric.WithDescription("Size of the reservoir checkpoint database in bytes"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.reservoirDbSizeGauge.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register db size gauge: %w", err)
	}
	
	// Register the compaction counter
	_, err = m.meter.Int64ObservableCounter(
		"reservoir_sampler.db_compactions",
		metric.WithDescription("Number of database compactions performed"),
		metric.WithUnit("{compactions}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.compactionCountCounter.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register compaction counter: %w", err)
	}
	
	// Register the LRU evictions counter
	_, err = m.meter.Int64ObservableCounter(
		"reservoir_sampler.lru_evictions",
		metric.WithDescription("Number of trace evictions from the LRU cache"),
		metric.WithUnit("{evictions}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.lruEvictionsCounter.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register LRU evictions counter: %w", err)
	}
	
	// Register the sampled spans counter
	_, err = m.meter.Int64ObservableCounter(
		"reservoir_sampler.sampled_spans",
		metric.WithDescription("Number of spans sampled (added to reservoir)"),
		metric.WithUnit("{spans}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(m.sampledSpansCounter.Load())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register sampled spans counter: %w", err)
	}
	
	return nil
}

// GetReservoirSizeGauge returns the reservoir size gauge
func (m *MetricsManager) GetReservoirSizeGauge() *atomic.Int64 {
	return m.reservoirSizeGauge
}

// GetWindowCountGauge returns the window count gauge
func (m *MetricsManager) GetWindowCountGauge() *atomic.Int64 {
	return m.windowCountGauge
}

// GetCheckpointAgeGauge returns the checkpoint age gauge
func (m *MetricsManager) GetCheckpointAgeGauge() *atomic.Int64 {
	return m.checkpointAgeGauge
}

// GetReservoirDbSizeGauge returns the reservoir db size gauge
func (m *MetricsManager) GetReservoirDbSizeGauge() *atomic.Int64 {
	return m.reservoirDbSizeGauge
}

// GetCompactionCountCounter returns the compaction count counter
func (m *MetricsManager) GetCompactionCountCounter() *atomic.Int64 {
	return m.compactionCountCounter
}

// GetLruEvictionsCounter returns the LRU evictions counter
func (m *MetricsManager) GetLruEvictionsCounter() *atomic.Int64 {
	return m.lruEvictionsCounter
}

// GetSampledSpansCounter returns the sampled spans counter
func (m *MetricsManager) GetSampledSpansCounter() *atomic.Int64 {
	return m.sampledSpansCounter
}