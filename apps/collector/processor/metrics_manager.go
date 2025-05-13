package processor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.uber.org/atomic"
)

// MetricsManager handles metrics reporting for the reservoir sampler
type MetricsManager struct {
	ctx           context.Context
	meter         component.Meter
	
	// Gauges
	reservoirSize    *atomic.Float64
	checkpointAge    *atomic.Float64
	reservoirDbSize  *atomic.Float64
	traceBufferSize  *atomic.Float64
	
	// Counters
	sampledSpans     *atomic.Float64
	lruEvictions     *atomic.Float64
	compactionCount  *atomic.Float64
}

// NewMetricsManager creates a new metrics manager
func NewMetricsManager(ctx context.Context, meter component.Meter) *MetricsManager {
	return &MetricsManager{
		ctx:              ctx,
		meter:            meter,
		reservoirSize:    atomic.NewFloat64(0),
		checkpointAge:    atomic.NewFloat64(0),
		reservoirDbSize:  atomic.NewFloat64(0),
		traceBufferSize:  atomic.NewFloat64(0),
		sampledSpans:     atomic.NewFloat64(0),
		lruEvictions:     atomic.NewFloat64(0),
		compactionCount:  atomic.NewFloat64(0),
	}
}

// RegisterMetrics registers all metrics with the meter
func (m *MetricsManager) RegisterMetrics() error {
	// Register reservoir size gauge
	if err := m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Gauge("reservoir_size", 
				instrument.WithDescription("Current number of spans in the reservoir"),
				instrument.WithUnit("spans"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Gauge("reservoir_size"), m.reservoirSize.Load())
			return nil
		},
	); err != nil {
		return err
	}
	
	// Register checkpoint age gauge
	if err := m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Gauge("checkpoint_age_seconds", 
				instrument.WithDescription("Age of the last checkpoint in seconds"),
				instrument.WithUnit("s"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Gauge("checkpoint_age_seconds"), m.checkpointAge.Load())
			return nil
		},
	); err != nil {
		return err
	}
	
	// Register reservoir DB size gauge
	if err := m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Gauge("reservoir_db_size_bytes", 
				instrument.WithDescription("Size of the checkpoint database in bytes"),
				instrument.WithUnit("By"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Gauge("reservoir_db_size_bytes"), m.reservoirDbSize.Load())
			return nil
		},
	); err != nil {
		return err
	}
	
	// Register trace buffer size gauge
	if err := m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Gauge("trace_buffer_size", 
				instrument.WithDescription("Current number of traces in the buffer"),
				instrument.WithUnit("traces"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Gauge("trace_buffer_size"), m.traceBufferSize.Load())
			return nil
		},
	); err != nil {
		return err
	}
	
	// Register sampled spans counter
	_ = m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Counter("sampled_spans_total", 
				instrument.WithDescription("Total number of spans sampled"),
				instrument.WithUnit("spans"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Counter("sampled_spans_total"), m.sampledSpans.Load())
			return nil
		},
	)
	
	// Register LRU evictions counter
	_ = m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Counter("lru_evictions_total", 
				instrument.WithDescription("Total number of traces evicted from the buffer"),
				instrument.WithUnit("traces"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Counter("lru_evictions_total"), m.lruEvictions.Load())
			return nil
		},
	)
	
	// Register compaction count counter
	_ = m.meter.RegisterCallback(
		[]instrument.Asynchronous{
			m.meter.AsyncFloat64().Counter("compaction_count_total", 
				instrument.WithDescription("Total number of DB compactions performed"),
				instrument.WithUnit("compactions"),
			),
		},
		func(ctx context.Context, observer metric.Observer) error {
			observer.ObserveFloat64(m.meter.AsyncFloat64().Counter("compaction_count_total"), m.compactionCount.Load())
			return nil
		},
	)
	
	return nil
}

// Implement reservoir.MetricsReporter interface

// ReportReservoirSize reports the current size of the reservoir
func (m *MetricsManager) ReportReservoirSize(size int) {
	m.reservoirSize.Store(float64(size))
}

// ReportSampledSpans reports the number of spans that were sampled
func (m *MetricsManager) ReportSampledSpans(count int) {
	m.sampledSpans.Add(float64(count))
}

// ReportTraceBufferSize reports the current size of the trace buffer
func (m *MetricsManager) ReportTraceBufferSize(size int) {
	m.traceBufferSize.Store(float64(size))
}

// ReportEvictions reports the number of trace evictions from the buffer
func (m *MetricsManager) ReportEvictions(count int) {
	m.lruEvictions.Add(float64(count))
}

// ReportCheckpointAge reports the age of the last checkpoint
func (m *MetricsManager) ReportCheckpointAge(age time.Duration) {
	m.checkpointAge.Store(age.Seconds())
}

// ReportDBSize reports the size of the checkpoint storage
func (m *MetricsManager) ReportDBSize(sizeBytes int64) {
	m.reservoirDbSize.Store(float64(sizeBytes))
}

// ReportCompactions reports the number of storage compactions
func (m *MetricsManager) ReportCompactions(count int) {
	m.compactionCount.Add(float64(count))
}

// Getter methods for use by other components

// GetReservoirSizeGauge returns the gauge func for reservoir size
func (m *MetricsManager) GetReservoirSizeGauge() func(float64) {
	return func(v float64) {
		m.reservoirSize.Store(v)
	}
}

// GetCheckpointAgeGauge returns the gauge func for checkpoint age
func (m *MetricsManager) GetCheckpointAgeGauge() func(float64) {
	return func(v float64) {
		m.checkpointAge.Store(v)
	}
}

// GetReservoirDbSizeGauge returns the gauge func for DB size
func (m *MetricsManager) GetReservoirDbSizeGauge() func(float64) {
	return func(v float64) {
		m.reservoirDbSize.Store(v)
	}
}

// GetSampledSpansCounter returns the counter func for sampled spans
func (m *MetricsManager) GetSampledSpansCounter() func(float64) {
	return func(v float64) {
		m.sampledSpans.Add(v)
	}
}

// GetLruEvictionsCounter returns the counter func for LRU evictions
func (m *MetricsManager) GetLruEvictionsCounter() func(float64) {
	return func(v float64) {
		m.lruEvictions.Add(v)
	}
}

// GetCompactionCountCounter returns the counter func for DB compactions
func (m *MetricsManager) GetCompactionCountCounter() func(float64) {
	return func(v float64) {
		m.compactionCount.Add(v)
	}
}
