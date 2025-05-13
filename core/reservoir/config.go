package reservoir

import (
	"encoding/json"
	"fmt"
	"time"
)

// Config defines configuration for the reservoir sampler processor.
type Config struct {
	// SizeK is the max number of spans to store in the reservoir
	SizeK int `mapstructure:"size_k"`

	// WindowDuration is the duration of each sampling window
	WindowDuration time.Duration `mapstructure:"window_duration"`

	// CheckpointPath is the file path to use for reservoir checkpoints
	CheckpointPath string `mapstructure:"checkpoint_path"`

	// CheckpointInterval is how often to checkpoint the reservoir state to disk
	CheckpointInterval time.Duration `mapstructure:"checkpoint_interval"`

	// TraceAware determines whether to use trace-aware sampling
	TraceAware bool `mapstructure:"trace_aware"`

	// TraceBufferMaxSize is the maximum number of traces to keep in memory at once
	TraceBufferMaxSize int `mapstructure:"trace_buffer_max_size"`

	// TraceBufferTimeout is how long to wait for a trace to complete
	TraceBufferTimeout time.Duration `mapstructure:"trace_buffer_timeout"`

	// DbCompactionScheduleCron is the cron schedule for DB compaction
	DbCompactionScheduleCron string `mapstructure:"db_compaction_schedule_cron"`

	// DbCompactionTargetSize is the target size in bytes for the database
	DbCompactionTargetSize int64 `mapstructure:"db_compaction_target_size"`
}

// Validate checks if the processor configuration is valid
func (cfg *Config) Validate() error {
	if cfg.SizeK <= 0 {
		return fmt.Errorf("size_k must be greater than 0, got %d", cfg.SizeK)
	}

	if cfg.WindowDuration <= 0 {
		return fmt.Errorf("window_duration must be positive, got %s", cfg.WindowDuration)
	}

	if cfg.CheckpointPath != "" && cfg.CheckpointInterval <= 0 {
		return fmt.Errorf("checkpoint_interval must be positive when checkpoint_path is set, got %s", cfg.CheckpointInterval)
	}

	if cfg.TraceAware {
		if cfg.TraceBufferMaxSize <= 0 {
			return fmt.Errorf("trace_buffer_max_size must be greater than 0 when trace_aware is true, got %d", cfg.TraceBufferMaxSize)
		}

		if cfg.TraceBufferTimeout <= 0 {
			return fmt.Errorf("trace_buffer_timeout must be positive when trace_aware is true, got %s", cfg.TraceBufferTimeout)
		}
	}

	return nil
}

// MarshalJSON implements json.Marshaler to properly serialize time.Duration fields
func (cfg *Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	
	// Create a struct with string fields for durations
	aux := struct {
		WindowDuration     string `json:"window_duration"`
		CheckpointInterval string `json:"checkpoint_interval"`
		TraceBufferTimeout string `json:"trace_buffer_timeout"`
		*Alias
	}{
		WindowDuration:     cfg.WindowDuration.String(),
		CheckpointInterval: cfg.CheckpointInterval.String(),
		TraceBufferTimeout: cfg.TraceBufferTimeout.String(),
		Alias:              (*Alias)(cfg),
	}
	
	return json.Marshal(aux)
}

// UnmarshalJSON implements json.Unmarshaler to properly deserialize time.Duration fields
func (cfg *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	
	// Create a struct with string fields for durations
	aux := struct {
		WindowDuration     string `json:"window_duration"`
		CheckpointInterval string `json:"checkpoint_interval"`
		TraceBufferTimeout string `json:"trace_buffer_timeout"`
		*Alias
	}{
		Alias: (*Alias)(cfg),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	// Parse durations
	var err error
	if aux.WindowDuration != "" {
		cfg.WindowDuration, err = time.ParseDuration(aux.WindowDuration)
		if err != nil {
			return fmt.Errorf("invalid window_duration: %w", err)
		}
	}
	
	if aux.CheckpointInterval != "" {
		cfg.CheckpointInterval, err = time.ParseDuration(aux.CheckpointInterval)
		if err != nil {
			return fmt.Errorf("invalid checkpoint_interval: %w", err)
		}
	}
	
	if aux.TraceBufferTimeout != "" {
		cfg.TraceBufferTimeout, err = time.ParseDuration(aux.TraceBufferTimeout)
		if err != nil {
			return fmt.Errorf("invalid trace_buffer_timeout: %w", err)
		}
	}
	
	return nil
}

// CreateDefaultConfig creates the default configuration for the processor.
func CreateDefaultConfig() component.Config {
	return &Config{
		SizeK:                    5000,
		WindowDuration:           60 * time.Second,
		CheckpointPath:           "",
		CheckpointInterval:       10 * time.Second,
		TraceAware:               true,
		TraceBufferMaxSize:       100000,
		TraceBufferTimeout:       10 * time.Second,
		DbCompactionScheduleCron: "",
		DbCompactionTargetSize:   0,
	}
}