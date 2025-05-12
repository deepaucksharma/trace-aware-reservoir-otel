# Integration Testing Framework

This package provides a comprehensive integration testing framework for the trace-aware reservoir sampling processor.

## Overview

The integration testing framework is designed to test the trace-aware reservoir sampling processor in an environment that closely resembles production. It allows for testing the processor's behavior, performance, and interaction with other components.

## Key Features

- **Test Framework**: A flexible testing framework that allows for configuring and testing the processor with various options.
- **Trace Generation**: Utilities for generating test traces with configurable parameters.
- **State Management**: Support for testing state persistence and restoration.
- **Capture and Verification**: Tools for capturing and verifying trace output from the processor.
- **NR-DOT Integration**: Specialized tests for New Relic Distribution of OpenTelemetry integration.

## Framework Components

- `TestFramework`: The main testing framework that manages processor lifecycle.
- `CapturingSink`: A sink that captures traces for verification.
- `NoopTracesSink`: A sink that discards traces, useful for performance testing.

## Test Categories

The framework includes tests for:

1. **Basic Operation**: Verifies the basic functionality of the reservoir sampler.
2. **Persistence**: Tests checkpoint creation and restoration.
3. **Trace Awareness**: Ensures that trace-aware sampling preserves complete traces.
4. **Window Rollover**: Tests the behavior of the sampler across multiple windows.
5. **NR-DOT Integration**: Tests integration with New Relic Distribution of OpenTelemetry.
6. **Full Lifecycle**: End-to-end testing of the processor's lifecycle.

## Usage

To create a new integration test:

```go
func TestMyFeature(t *testing.T) {
    // Create test framework
    tf, err := NewTestFramework(t, WithInMemoryDB())
    require.NoError(t, err, "Failed to create test framework")
    defer tf.Cleanup()

    // Setup processor with desired options
    ctx := context.Background()
    err = tf.Setup(ctx,
        WithReservoirSize(10),
        WithWindowDuration("1s"),
        WithCheckpointInterval("500ms"),
        WithTraceAware(true),
    )
    require.NoError(t, err, "Failed to setup processor")

    // Send test traces
    err = tf.SendTestTraces(ctx, 0, 20, 2)
    require.NoError(t, err, "Failed to send test traces")

    // Wait for processing
    time.Sleep(2 * time.Second)

    // Force export
    tf.ForceExport()
    time.Sleep(500 * time.Millisecond)

    // Verify results
    capturedTraces := tf.GetCapturedTraces()
    assert.NotEmpty(t, capturedTraces, "No traces were captured")

    // Shutdown processor
    err = tf.Shutdown(ctx)
    require.NoError(t, err, "Failed to shutdown processor")
}
```

## Test Options

The framework provides various options for configuring tests:

- `WithDataDir`: Specify a custom data directory for the test.
- `WithInMemoryDB`: Use an in-memory database for testing.
- `WithCleanupDataDir`: Specify whether to clean up the data directory after the test.
- `WithLogger`: Provide a custom logger for the test.
- `WithReservoirSize`: Set the reservoir size for the processor.
- `WithWindowDuration`: Set the window duration for the processor.
- `WithCheckpointInterval`: Set the checkpoint interval for the processor.
- `WithTraceAware`: Enable or disable trace-aware mode.
- `WithTraceBufferMaxSize`: Set the trace buffer max size.
- `WithTraceBufferTimeout`: Set the trace buffer timeout.

## Helper Functions

The framework includes various helper functions:

- `TeeLogger`: Creates a logger that writes to both a file and the testing log.
- `CopyFile`: Copies a file from src to dst.
- `generateTestTraces`: Creates test traces with unique trace IDs.
- `countUniqueTraces`: Counts unique traces across all trace batches.

## Best Practices

1. Always use `defer tf.Cleanup()` to ensure resources are cleaned up.
2. Use `WithInMemoryDB()` for faster testing when persistence isn't being tested.
3. Use appropriate wait times to ensure asynchronous operations complete.
4. Verify results using the captured traces from the framework.
5. Test with different processor configurations to cover edge cases.