package nrdot

import (
	"fmt"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/processor"
)

// Optimization levels for NR-DOT environment
const (
	OptimizationLevelLow    = "low"    // Minimal resource usage, reduced sampling
	OptimizationLevelMedium = "medium" // Balanced resource usage and sampling quality
	OptimizationLevelHigh   = "high"   // High sampling quality, more resource usage
)

// OptimizeConfig optimizes the reservoir sampler configuration for NR-DOT
// based on the given optimization level.
func (i *Integration) OptimizeConfig(cfg *reservoirsampler.Config, level string) *reservoirsampler.Config {
	// Use default config if nil
	if cfg == nil {
		cfg = i.CreateDefaultConfig()
	}
	
	// Apply optimizations based on level
	switch level {
	case OptimizationLevelLow:
		// Low resource usage, suitable for resource-constrained environments
		cfg.SizeK = 1000                     // Smaller reservoir
		cfg.WindowDuration = "120s"          // Longer windows
		cfg.CheckpointInterval = "30s"       // Less frequent checkpoints
		cfg.TraceBufferMaxSize = 20000       // Smaller trace buffer
		cfg.TraceBufferTimeout = "45s"       // Longer timeout to reduce evictions
		cfg.DbCompactionScheduleCron = "0 2 * * 0" // Weekly compaction
		
	case OptimizationLevelMedium:
		// Balanced, suitable for most environments
		cfg.SizeK = 5000                     // Medium reservoir
		cfg.WindowDuration = "60s"           // Medium windows
		cfg.CheckpointInterval = "10s"       // Regular checkpoints
		cfg.TraceBufferMaxSize = 100000      // Medium trace buffer
		cfg.TraceBufferTimeout = "30s"       // Medium timeout
		cfg.DbCompactionScheduleCron = "0 2 * * 0" // Weekly compaction
		
	case OptimizationLevelHigh:
		// High quality, suitable for high-performance environments
		cfg.SizeK = 10000                    // Larger reservoir
		cfg.WindowDuration = "30s"           // Shorter windows
		cfg.CheckpointInterval = "5s"        // More frequent checkpoints
		cfg.TraceBufferMaxSize = 200000      // Larger trace buffer
		cfg.TraceBufferTimeout = "15s"       // Shorter timeout for quicker processing
		cfg.DbCompactionScheduleCron = "0 2 * * *" // Daily compaction
	}
	
	return cfg
}

// NRDOTProcessorSettings returns processor settings optimized for NR-DOT
func (i *Integration) NRDOTProcessorSettings() processor.Settings {
	return processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			// Set sensible telemetry defaults for NR-DOT
			Logger: i.logger,
		},
	}
}

// OptimizeForHighTrafficServices adjusts the configuration for high-traffic services
func (i *Integration) OptimizeForHighTrafficServices(cfg *reservoirsampler.Config) *reservoirsampler.Config {
	// Use default config if nil
	if cfg == nil {
		cfg = i.CreateDefaultConfig()
	}
	
	// High traffic optimizations
	cfg.SizeK = 15000                    // Very large reservoir
	cfg.WindowDuration = "15s"           // Very short windows
	cfg.CheckpointInterval = "15s"       // Balanced checkpoint interval
	cfg.TraceBufferMaxSize = 300000      // Very large trace buffer
	cfg.TraceBufferTimeout = "10s"       // Short timeout for rapid processing
	cfg.DbCompactionScheduleCron = "0 */6 * * *" // Every 6 hours
	
	return cfg
}

// OptimizeForNewRelicEntity adjusts the configuration for specific NR entity types
// Entities can be 'apm', 'browser', 'mobile', 'serverless', or any valid NR entity
func (i *Integration) OptimizeForNewRelicEntity(cfg *reservoirsampler.Config, entityType string) *reservoirsampler.Config {
	// Use default config if nil
	if cfg == nil {
		cfg = i.CreateDefaultConfig()
	}
	
	// Apply entity-specific optimizations
	switch entityType {
	case "apm":
		// APM applications typically have medium to large traces
		cfg.TraceBufferMaxSize = 150000
		cfg.TraceBufferTimeout = "20s"
		
	case "browser":
		// Browser monitoring typically has smaller traces
		cfg.SizeK = 7500 // More traces but smaller
		cfg.TraceBufferMaxSize = 50000
		cfg.TraceBufferTimeout = "5s"
		
	case "mobile":
		// Mobile apps may have connectivity issues, so longer timeouts
		cfg.TraceBufferTimeout = "45s"
		
	case "serverless":
		// Serverless functions typically have shorter durations
		cfg.WindowDuration = "45s"
		cfg.TraceBufferTimeout = "10s"
	}
	
	return cfg
}

// NewRelicExporterConfig generates the configuration for the New Relic exporter
// This ensures the sampled traces are properly sent to New Relic
func (i *Integration) NewRelicExporterConfig(licenseKey string, region string) string {
	// Determine endpoint based on region
	endpoint := "https://otlp.nr-data.net:4318"
	if region == "EU" {
		endpoint = "https://otlp.eu01.nr-data.net:4318"
	}
	
	return fmt.Sprintf(`exporters:
  otlphttp/newrelic:
    endpoint: "%s"
    headers:
      api-key: %s

service:
  pipelines:
    traces:
      exporters: [otlphttp/newrelic]
`, endpoint, licenseKey)
}