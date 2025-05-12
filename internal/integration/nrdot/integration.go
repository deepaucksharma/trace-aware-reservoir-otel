// Package nrdot provides integration between trace-aware reservoir sampling and NR-DOT.
package nrdot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

// Integration handles the integration between the reservoir sampler and NR-DOT.
type Integration struct {
	logger *zap.Logger
}

// NewIntegration creates a new NR-DOT integration.
func NewIntegration(logger *zap.Logger) *Integration {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Integration{
		logger: logger,
	}
}

// RegisterWithNRDOT registers the reservoir sampler with the NR-DOT distribution.
// It modifies the distribution.yaml file to include the reservoir sampler.
func (i *Integration) RegisterWithNRDOT(nrdotPath string) error {
	// Path to the distribution.yaml file
	distFile := filepath.Join(nrdotPath, "distributions/nrdot-collector/distribution.yaml")
	
	// Check if the file exists
	if _, err := os.Stat(distFile); os.IsNotExist(err) {
		return fmt.Errorf("distribution file not found: %s", distFile)
	}
	
	// Read the distribution file
	data, err := os.ReadFile(distFile)
	if err != nil {
		return fmt.Errorf("failed to read distribution file: %w", err)
	}
	
	// Make backup if not already done
	backupFile := distFile + ".bak"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		if err := os.WriteFile(backupFile, data, 0644); err != nil {
			return fmt.Errorf("failed to create backup file: %w", err)
		}
		i.logger.Info("Created backup of distribution file", zap.String("path", backupFile))
	}
	
	// Define paths for the components to add
	reservoirPath := "github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	fileStoragePath := "github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension"
	
	// Check if the components are already included
	dataStr := string(data)
	reservoirExists := strings.Contains(dataStr, reservoirPath)
	fileStorageExists := strings.Contains(dataStr, fileStoragePath)
	
	if reservoirExists {
		i.logger.Info("Reservoir sampler is already included in the distribution")
	}
	
	if fileStorageExists {
		i.logger.Info("File storage extension is already included in the distribution")
	}
	
	// Modify the distribution file to include our components
	if !reservoirExists || !fileStorageExists {
		// Add reservoir sampler to processors section
		if !reservoirExists {
			dataStr = addToSection(dataStr, "processors:", "exporters:", "  - "+reservoirPath)
			i.logger.Info("Added reservoir sampler to processors section")
		}
		
		// Add file storage extension to extensions section
		if !fileStorageExists {
			dataStr = addToSection(dataStr, "extensions:", "receivers:", "  - "+fileStoragePath)
			i.logger.Info("Added file storage extension to extensions section")
		}
		
		// Write the modified file
		if err := os.WriteFile(distFile, []byte(dataStr), 0644); err != nil {
			return fmt.Errorf("failed to write modified distribution file: %w", err)
		}
	}
	
	return nil
}

// CreateFactory creates and returns a processor factory for the reservoir sampler.
// This can be used directly with NR-DOT or any OpenTelemetry Collector.
func (i *Integration) CreateFactory() processor.Factory {
	return reservoirsampler.NewFactory()
}

// CreateDefaultConfig creates a default configuration for the reservoir sampler
// optimized for NR-DOT environments.
func (i *Integration) CreateDefaultConfig() *reservoirsampler.Config {
	// Create config with sensible defaults for NR-DOT
	return &reservoirsampler.Config{
		SizeK:                    5000,
		WindowDuration:           "60s",
		CheckpointPath:           "/var/otelpersist/reservoir.db",
		CheckpointInterval:       "10s",
		TraceAware:               true,
		TraceBufferMaxSize:       100000,
		TraceBufferTimeout:       "30s",
		DbCompactionScheduleCron: "0 2 * * 0",  // Weekly at 2 AM Sunday
		DbCompactionTargetSize:   104857600,    // 100 MB
	}
}

// LoadConfig loads reservoir sampler configuration from a file.
func (i *Integration) LoadConfig(configPath string) (*reservoirsampler.Config, error) {
	// Check if the file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}
	
	// Create config provider
	provider, err := confmap.NewFileProvider(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create config provider: %w", err)
	}
	
	// Load the config
	cfgMap, err := provider.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create the config
	cfg := &reservoirsampler.Config{}
	
	// Get the reservoir_sampler section
	samplerMap := cfgMap.Get(confmap.Key("processors.reservoir_sampler"))
	if samplerMap == nil {
		return nil, fmt.Errorf("processors.reservoir_sampler section not found in config")
	}
	
	// Convert to our config struct
	if err := samplerMap.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate the config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return cfg, nil
}

// GenerateConfigYAML generates a YAML configuration for the reservoir sampler
// based on the provided options.
func (i *Integration) GenerateConfigYAML(cfg *reservoirsampler.Config) string {
	if cfg == nil {
		cfg = i.CreateDefaultConfig()
	}
	
	return fmt.Sprintf(`processors:
  reservoir_sampler:
    size_k: %d
    window_duration: "%s"
    checkpoint_path: "%s"
    checkpoint_interval: "%s"
    trace_aware: %v
    trace_buffer_max_size: %d
    trace_buffer_timeout: "%s"
    db_compaction_schedule_cron: "%s"
    db_compaction_target_size: %d

service:
  extensions: [health_check, pprof, zpages, file_storage]
  
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, reservoir_sampler]
      exporters: [otlphttp/newrelic]
`,
		cfg.SizeK,
		cfg.WindowDuration,
		cfg.CheckpointPath,
		cfg.CheckpointInterval,
		cfg.TraceAware,
		cfg.TraceBufferMaxSize,
		cfg.TraceBufferTimeout,
		cfg.DbCompactionScheduleCron,
		cfg.DbCompactionTargetSize,
	)
}