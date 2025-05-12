package nrdot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewIntegration(t *testing.T) {
	// Test with nil logger
	integration := NewIntegration(nil)
	assert.NotNil(t, integration, "Should create integration with nil logger")

	// Test with logger
	logger, _ := zap.NewDevelopment()
	integration = NewIntegration(logger)
	assert.NotNil(t, integration, "Should create integration with logger")
}

func TestCreateDefaultConfig(t *testing.T) {
	integration := NewIntegration(nil)
	
	// Get default config
	cfg := integration.CreateDefaultConfig()
	
	// Verify defaults
	assert.Equal(t, 5000, cfg.SizeK, "Incorrect default SizeK")
	assert.Equal(t, "60s", cfg.WindowDuration, "Incorrect default WindowDuration")
	assert.Equal(t, "/var/otelpersist/reservoir.db", cfg.CheckpointPath, "Incorrect default CheckpointPath")
	assert.Equal(t, "10s", cfg.CheckpointInterval, "Incorrect default CheckpointInterval")
	assert.True(t, cfg.TraceAware, "TraceAware should be true by default")
	assert.Equal(t, 100000, cfg.TraceBufferMaxSize, "Incorrect default TraceBufferMaxSize")
	assert.Equal(t, "30s", cfg.TraceBufferTimeout, "Incorrect default TraceBufferTimeout")
}

func TestGenerateConfigYAML(t *testing.T) {
	integration := NewIntegration(nil)
	
	// Generate config with default settings
	yaml := integration.GenerateConfigYAML(nil)
	
	// Verify YAML contains expected sections
	assert.Contains(t, yaml, "processors:", "YAML should contain processors section")
	assert.Contains(t, yaml, "reservoir_sampler:", "YAML should contain reservoir_sampler section")
	assert.Contains(t, yaml, "size_k: 5000", "YAML should contain default size_k")
	assert.Contains(t, yaml, "service:", "YAML should contain service section")
	assert.Contains(t, yaml, "pipelines:", "YAML should contain pipelines section")
	
	// Test with custom config
	cfg := &reservoirsampler.Config{
		SizeK:                    10000,
		WindowDuration:           "30s",
		CheckpointPath:           "/custom/path/reservoir.db",
		CheckpointInterval:       "5s",
		TraceAware:               true,
		TraceBufferMaxSize:       200000,
		TraceBufferTimeout:       "15s",
		DbCompactionScheduleCron: "0 * * * *",
		DbCompactionTargetSize:   1048576,
	}
	
	yaml = integration.GenerateConfigYAML(cfg)
	
	// Verify custom values
	assert.Contains(t, yaml, "size_k: 10000", "YAML should contain custom size_k")
	assert.Contains(t, yaml, `window_duration: "30s"`, "YAML should contain custom window_duration")
	assert.Contains(t, yaml, `checkpoint_path: "/custom/path/reservoir.db"`, "YAML should contain custom checkpoint_path")
}

func TestOptimizeConfig(t *testing.T) {
	integration := NewIntegration(nil)
	
	// Test low optimization
	lowCfg := integration.OptimizeConfig(nil, OptimizationLevelLow)
	assert.Equal(t, 1000, lowCfg.SizeK, "Low optimization should have smaller reservoir")
	assert.Equal(t, "120s", lowCfg.WindowDuration, "Low optimization should have longer window")
	
	// Test medium optimization
	medCfg := integration.OptimizeConfig(nil, OptimizationLevelMedium)
	assert.Equal(t, 5000, medCfg.SizeK, "Medium optimization should have medium reservoir")
	assert.Equal(t, "60s", medCfg.WindowDuration, "Medium optimization should have medium window")
	
	// Test high optimization
	highCfg := integration.OptimizeConfig(nil, OptimizationLevelHigh)
	assert.Equal(t, 10000, highCfg.SizeK, "High optimization should have larger reservoir")
	assert.Equal(t, "30s", highCfg.WindowDuration, "High optimization should have shorter window")
}

func TestOptimizeForNewRelicEntity(t *testing.T) {
	integration := NewIntegration(nil)
	
	// Test APM optimization
	apmCfg := integration.OptimizeForNewRelicEntity(nil, "apm")
	assert.Equal(t, 150000, apmCfg.TraceBufferMaxSize, "APM optimization incorrect")
	
	// Test browser optimization
	browserCfg := integration.OptimizeForNewRelicEntity(nil, "browser")
	assert.Equal(t, 7500, browserCfg.SizeK, "Browser optimization incorrect")
	
	// Test mobile optimization
	mobileCfg := integration.OptimizeForNewRelicEntity(nil, "mobile")
	assert.Equal(t, "45s", mobileCfg.TraceBufferTimeout, "Mobile optimization incorrect")
	
	// Test serverless optimization
	serverlessCfg := integration.OptimizeForNewRelicEntity(nil, "serverless")
	assert.Equal(t, "45s", serverlessCfg.WindowDuration, "Serverless optimization incorrect")
}

func TestNewRelicExporterConfig(t *testing.T) {
	integration := NewIntegration(nil)
	
	// Test US region
	usConfig := integration.NewRelicExporterConfig("LICENSE_KEY", "US")
	assert.Contains(t, usConfig, "https://otlp.nr-data.net:4318", "US endpoint incorrect")
	assert.Contains(t, usConfig, "api-key: LICENSE_KEY", "License key not set correctly")
	
	// Test EU region
	euConfig := integration.NewRelicExporterConfig("LICENSE_KEY", "EU")
	assert.Contains(t, euConfig, "https://otlp.eu01.nr-data.net:4318", "EU endpoint incorrect")
}

func TestRegisterWithNRDOT(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "nrdot-test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)
	
	// Create distribution directory structure
	distDir := filepath.Join(tempDir, "distributions/nrdot-collector")
	err = os.MkdirAll(distDir, 0755)
	require.NoError(t, err, "Failed to create distribution directory")
	
	// Create a mock distribution.yaml file
	distYaml := `extensions:
  health_check:
  pprof:
receivers:
  otlp:
processors:
  batch:
exporters:
  otlp:
`
	
	distFile := filepath.Join(distDir, "distribution.yaml")
	err = os.WriteFile(distFile, []byte(distYaml), 0644)
	require.NoError(t, err, "Failed to write distribution.yaml")
	
	// Create integration
	logger, _ := zap.NewDevelopment()
	integration := NewIntegration(logger)
	
	// Test registration
	err = integration.RegisterWithNRDOT(tempDir)
	assert.NoError(t, err, "Registration should succeed")
	
	// Read the modified file
	modifiedData, err := os.ReadFile(distFile)
	require.NoError(t, err, "Failed to read modified file")
	modifiedYaml := string(modifiedData)
	
	// Check if components were added
	assert.True(t, strings.Contains(modifiedYaml, "github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"), 
		"Reservoir sampler should be added")
	assert.True(t, strings.Contains(modifiedYaml, "github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension"), 
		"File storage extension should be added")
}