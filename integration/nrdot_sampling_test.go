package integration

import (
	"context"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/integration/nrdot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNRDOT_SamplingIntegration(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB())
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Create NR-DOT integration for configuration
	integration := nrdot.NewIntegration(tf.logger)

	// Create a configuration optimized for NR-DOT
	nrdotConfig := integration.CreateDefaultConfig()
	nrdotConfig.SizeK = 10 // Small reservoir for testing
	nrdotConfig.WindowDuration = "1s" // Short window for testing
	nrdotConfig.CheckpointPath = "" // In-memory storage
	nrdotConfig.CheckpointInterval = "500ms"
	nrdotConfig.TraceAware = true
	nrdotConfig.TraceBufferMaxSize = 1000
	nrdotConfig.TraceBufferTimeout = "500ms"

	// Setup processor with NR-DOT configuration
	ctx := context.Background()
	err = tf.Setup(ctx,
		WithReservoirSize(nrdotConfig.SizeK),
		WithWindowDuration(nrdotConfig.WindowDuration),
		WithCheckpointInterval(nrdotConfig.CheckpointInterval),
		WithTraceAware(nrdotConfig.TraceAware),
		WithTraceBufferMaxSize(nrdotConfig.TraceBufferMaxSize),
		WithTraceBufferTimeout(nrdotConfig.TraceBufferTimeout),
	)
	require.NoError(t, err, "Failed to setup processor with NR-DOT configuration")

	// Send traces to the processor
	numTraces := 20 // More traces than the reservoir size
	spansPerTrace := 3
	err = tf.SendTestTraces(ctx, 0, numTraces, spansPerTrace)
	require.NoError(t, err, "Failed to send test traces")

	// Wait for processor to process traces
	time.Sleep(2 * time.Second)

	// Force export
	tf.ForceExport()
	time.Sleep(500 * time.Millisecond)

	// Verify results
	capturedTraces := tf.GetCapturedTraces()
	assert.NotEmpty(t, capturedTraces, "No traces were captured")

	uniqueTraceCount := tf.CountUniqueTraces()
	assert.Greater(t, uniqueTraceCount, 0, "No unique traces were captured")
	assert.LessOrEqual(t, uniqueTraceCount, nrdotConfig.SizeK, 
		"Captured traces exceeds reservoir size")

	// Shutdown the processor
	err = tf.Shutdown(ctx)
	require.NoError(t, err, "Failed to shutdown processor")
}

func TestNRDOT_OptimizedConfigurations(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB())
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Create NR-DOT integration for configuration
	integration := nrdot.NewIntegration(tf.logger)

	// Test different optimization levels
	optimizationLevels := []string{
		nrdot.OptimizationLevelLow,
		nrdot.OptimizationLevelMedium,
		nrdot.OptimizationLevelHigh,
	}

	for _, level := range optimizationLevels {
		t.Run(level, func(t *testing.T) {
			// Reset for each optimization level
			err := tf.Reset(ctx)
			require.NoError(t, err, "Failed to reset test framework")

			// Create configuration with this optimization level
			config := integration.CreateDefaultConfig()
			config = integration.OptimizeConfig(config, level)
			config.CheckpointPath = "" // In-memory storage for testing

			// Setup processor with this configuration
			ctx := context.Background()
			err = tf.Setup(ctx,
				WithReservoirSize(config.SizeK),
				WithWindowDuration(config.WindowDuration),
				WithCheckpointInterval(config.CheckpointInterval),
				WithTraceAware(config.TraceAware),
				WithTraceBufferMaxSize(config.TraceBufferMaxSize),
				WithTraceBufferTimeout(config.TraceBufferTimeout),
			)
			require.NoError(t, err, "Failed to setup processor with optimization level %s", level)

			// Send traces to the processor
			numTraces := 50 // Enough traces to test the configuration
			spansPerTrace := 3
			err = tf.SendTestTraces(ctx, 0, numTraces, spansPerTrace)
			require.NoError(t, err, "Failed to send test traces")

			// Wait for processor to process traces
			time.Sleep(2 * time.Second)

			// Force export
			tf.ForceExport()
			time.Sleep(500 * time.Millisecond)

			// Verify results
			capturedTraces := tf.GetCapturedTraces()
			assert.NotEmpty(t, capturedTraces, "No traces were captured with optimization level %s", level)

			uniqueTraceCount := tf.CountUniqueTraces()
			assert.Greater(t, uniqueTraceCount, 0, 
				"No unique traces were captured with optimization level %s", level)
			assert.LessOrEqual(t, uniqueTraceCount, config.SizeK, 
				"Captured traces exceeds reservoir size with optimization level %s", level)

			// Shutdown the processor
			err = tf.Shutdown(ctx)
			require.NoError(t, err, "Failed to shutdown processor with optimization level %s", level)
		})
	}
}

func TestNRDOT_EntitySpecificOptimizations(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test framework with in-memory DB for faster testing
	tf, err := NewTestFramework(t, WithInMemoryDB())
	require.NoError(t, err, "Failed to create test framework")
	defer tf.Cleanup()

	// Create NR-DOT integration for configuration
	integration := nrdot.NewIntegration(tf.logger)

	// Test different entity types
	entityTypes := []string{
		"apm",
		"browser",
		"mobile",
		"serverless",
	}

	for _, entityType := range entityTypes {
		t.Run(entityType, func(t *testing.T) {
			// Reset for each entity type
			err := tf.Reset(context.Background())
			require.NoError(t, err, "Failed to reset test framework")

			// Create configuration with this entity type
			config := integration.CreateDefaultConfig()
			config = integration.OptimizeForNewRelicEntity(config, entityType)
			config.CheckpointPath = "" // In-memory storage for testing
			config.WindowDuration = "1s" // Short window for testing
			config.CheckpointInterval = "500ms" // Short checkpoint interval for testing

			// Adjust sampling settings for testing
			if config.SizeK > 50 {
				config.SizeK = 50 // Cap reservoir size for testing
			}

			// Setup processor with this configuration
			ctx := context.Background()
			err = tf.Setup(ctx,
				WithReservoirSize(config.SizeK),
				WithWindowDuration(config.WindowDuration),
				WithCheckpointInterval(config.CheckpointInterval),
				WithTraceAware(config.TraceAware),
				WithTraceBufferMaxSize(config.TraceBufferMaxSize),
				WithTraceBufferTimeout(config.TraceBufferTimeout),
			)
			require.NoError(t, err, "Failed to setup processor with entity type %s", entityType)

			// Send traces to the processor
			numTraces := config.SizeK * 2 // Twice the reservoir size
			spansPerTrace := 3
			err = tf.SendTestTraces(ctx, 0, numTraces, spansPerTrace)
			require.NoError(t, err, "Failed to send test traces")

			// Wait for processor to process traces
			time.Sleep(2 * time.Second)

			// Force export
			tf.ForceExport()
			time.Sleep(500 * time.Millisecond)

			// Verify results
			capturedTraces := tf.GetCapturedTraces()
			assert.NotEmpty(t, capturedTraces, "No traces were captured with entity type %s", entityType)

			uniqueTraceCount := tf.CountUniqueTraces()
			assert.Greater(t, uniqueTraceCount, 0, 
				"No unique traces were captured with entity type %s", entityType)
			assert.LessOrEqual(t, uniqueTraceCount, config.SizeK, 
				"Captured traces exceeds reservoir size with entity type %s", entityType)

			// Verify entity-specific behavior
			switch entityType {
			case "browser":
				// Browser optimization typically has higher reservoir size but shorter timeouts
				assert.GreaterOrEqual(t, config.SizeK, 1000, "Browser entity should have larger reservoir")
				assert.Less(t, ParseDuration(config.TraceBufferTimeout), 10*time.Second, 
					"Browser entity should have shorter trace buffer timeout")
			case "mobile":
				// Mobile optimization typically has longer timeouts
				assert.GreaterOrEqual(t, ParseDuration(config.TraceBufferTimeout), 10*time.Second, 
					"Mobile entity should have longer trace buffer timeout")
			case "serverless":
				// Serverless optimization typically has shorter windows
				assert.Less(t, ParseDuration(config.WindowDuration), 60*time.Second, 
					"Serverless entity should have shorter window duration")
			}

			// Shutdown the processor
			err = tf.Shutdown(ctx)
			require.NoError(t, err, "Failed to shutdown processor with entity type %s", entityType)
		})
	}
}

// Helper function to parse duration
func ParseDuration(s string) time.Duration {
	duration, _ := time.ParseDuration(s)
	return duration
}