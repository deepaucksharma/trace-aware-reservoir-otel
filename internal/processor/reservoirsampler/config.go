package reservoirsampler

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for the reservoir sampler processor.
// It includes settings for sampling window size, persistence, and trace-aware sampling.
type Config struct {
	// SizeK is the max number of spans to store in the reservoir
	SizeK int `mapstructure:"size_k"`

	// WindowDuration is the duration of each sampling window
	WindowDuration string `mapstructure:"window_duration"`

	// CheckpointPath is the file path to use for reservoir checkpoints
	CheckpointPath string `mapstructure:"checkpoint_path"`

	// CheckpointInterval is how often to checkpoint the reservoir state to disk
	CheckpointInterval string `mapstructure:"checkpoint_interval"`

	// TraceAware determines whether to use trace-aware sampling
	TraceAware bool `mapstructure:"trace_aware"`

	// TraceBufferMaxSize is the maximum number of traces to keep in memory at once
	TraceBufferMaxSize int `mapstructure:"trace_buffer_max_size"`

	// TraceBufferTimeout is how long to wait for a trace to complete
	TraceBufferTimeout string `mapstructure:"trace_buffer_timeout"`

	// DbCompactionScheduleCron is the cron schedule for BoltDB compaction
	DbCompactionScheduleCron string `mapstructure:"db_compaction_schedule_cron"`

	// DbCompactionTargetSize is the target size in bytes for the database
	DbCompactionTargetSize int64 `mapstructure:"db_compaction_target_size"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the processor configuration is valid
func (cfg *Config) Validate() error {
	if cfg.SizeK <= 0 {
		return fmt.Errorf("size_k must be greater than 0, got %d", cfg.SizeK)
	}

	if cfg.WindowDuration == "" {
		return fmt.Errorf("window_duration must be specified")
	}

	windowDuration, err := time.ParseDuration(cfg.WindowDuration)
	if err != nil {
		return fmt.Errorf("invalid window_duration format: %w", err)
	}

	if windowDuration <= 0 {
		return fmt.Errorf("window_duration must be positive, got %s", cfg.WindowDuration)
	}

	if cfg.CheckpointPath == "" {
		return fmt.Errorf("checkpoint_path must be specified")
	}

	if cfg.CheckpointInterval == "" {
		return fmt.Errorf("checkpoint_interval must be specified")
	}

	checkpointInterval, err := time.ParseDuration(cfg.CheckpointInterval)
	if err != nil {
		return fmt.Errorf("invalid checkpoint_interval format: %w", err)
	}

	if checkpointInterval <= 0 {
		return fmt.Errorf("checkpoint_interval must be positive, got %s", cfg.CheckpointInterval)
	}

	if cfg.TraceAware {
		if cfg.TraceBufferMaxSize <= 0 {
			return fmt.Errorf("trace_buffer_max_size must be greater than 0 when trace_aware is true, got %d", cfg.TraceBufferMaxSize)
		}

		if cfg.TraceBufferTimeout == "" {
			return fmt.Errorf("trace_buffer_timeout must be specified when trace_aware is true")
		}

		bufferTimeout, err := time.ParseDuration(cfg.TraceBufferTimeout)
		if err != nil {
			return fmt.Errorf("invalid trace_buffer_timeout format: %w", err)
		}

		if bufferTimeout <= 0 {
			return fmt.Errorf("trace_buffer_timeout must be positive, got %s", cfg.TraceBufferTimeout)
		}
	}

	return nil
}

// CreateDefaultConfig creates the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		SizeK:                    5000,
		WindowDuration:           "60s",
		CheckpointPath:           "",
		CheckpointInterval:       "10s",
		TraceAware:               true,
		TraceBufferMaxSize:       100000,
		TraceBufferTimeout:       "10s",
		DbCompactionScheduleCron: "",
		DbCompactionTargetSize:   0,
	}
}
