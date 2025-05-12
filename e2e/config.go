package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

// TestConfig defines the configuration for an e2e test
type TestConfig struct {
	// Reservoir sampler configuration
	ReservoirSize        int    `yaml:"reservoir_size"`
	WindowDuration       string `yaml:"window_duration"`
	CheckpointPath       string `yaml:"checkpoint_path"`
	CheckpointInterval   string `yaml:"checkpoint_interval"`
	TraceAware           bool   `yaml:"trace_aware"`
	TraceBufferMaxSize   int    `yaml:"trace_buffer_max_size"`
	TraceBufferTimeout   string `yaml:"trace_buffer_timeout"`
	CompactionSchedule   string `yaml:"compaction_schedule"`
	CompactionTargetSize int64  `yaml:"compaction_target_size"`

	// Test configuration
	InputRate           int    `yaml:"input_rate"`           // Spans per second
	TestDuration        string `yaml:"test_duration"`        
	SpansPerTrace       int    `yaml:"spans_per_trace"`
	DistributionPattern string `yaml:"distribution_pattern"` // uniform, zipfian, etc.
	Concurrency         int    `yaml:"concurrency"`          // Number of concurrent senders
}

// DefaultTestConfig returns a default configuration for e2e tests
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		ReservoirSize:        10000,
		WindowDuration:       "1m",
		CheckpointPath:       "./data/checkpoint.db",
		CheckpointInterval:   "10s",
		TraceAware:           true,
		TraceBufferMaxSize:   100000,
		TraceBufferTimeout:   "5s",
		CompactionSchedule:   "0 * * * *",
		CompactionTargetSize: 1024 * 1024 * 1024, // 1GB

		InputRate:           1000,
		TestDuration:        "1m",
		SpansPerTrace:       10,
		DistributionPattern: "uniform",
		Concurrency:         4,
	}
}

// GenerateConfigFile creates a collector config file with the specified test configuration
func (c *TestConfig) GenerateConfigFile() (string, error) {
	// Create temp directory for config file if it doesn't exist
	tempDir := filepath.Join(".", "e2e", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Generate unique filename
	timestamp := time.Now().Format("20060102_150405")
	id := uuid.New().String()[:8]
	filename := filepath.Join(tempDir, fmt.Sprintf("values_%s_%s.yaml", timestamp, id))

	// Load template and execute it
	tmplContent := `
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    send_batch_size: 1000
    timeout: 1s
    
  reservoir_sampler:
    size_k: {{ .ReservoirSize }}
    window_duration: {{ .WindowDuration }}
    checkpoint_path: {{ .CheckpointPath }}
    checkpoint_interval: {{ .CheckpointInterval }}
    trace_aware: {{ .TraceAware }}
    trace_buffer_max_size: {{ .TraceBufferMaxSize }}
    trace_buffer_timeout: {{ .TraceBufferTimeout }}
    db_compaction_schedule_cron: {{ .CompactionSchedule }}
    db_compaction_target_size: {{ .CompactionTargetSize }}

exporters:
  logging:
    verbosity: detailed
    
  otlp:
    endpoint: localhost:4320
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, reservoir_sampler]
      exporters: [logging, otlp]
  
  telemetry:
    logs:
      level: debug
`

	tmpl, err := template.New("collector-config").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, c); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return filename, nil
}

// LoadTestConfig loads a test configuration from a file
func LoadTestConfig(path string) (*TestConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config TestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// SaveTestConfig saves a test configuration to a file
func (c *TestConfig) SaveTestConfig(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}