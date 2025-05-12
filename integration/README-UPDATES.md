# Integration Testing Framework: Enhancements and Improvements

## Overview of Changes

The integration testing framework for the trace-aware reservoir sampler has been significantly enhanced to provide a more comprehensive, modular, and efficient testing environment. The updates focus on better organization, increased test coverage, and improved developer experience.

## Core Improvements

### 1. Enhanced TestFramework

The `TestFramework` in `framework.go` has been completely redesigned to provide:

- **Functional Options Pattern**: Options like `WithDataDir()`, `WithInMemoryDB()`, `WithLogger()` for flexible configuration
- **Lifecycle Management**: Proper setup, execution, and cleanup of test resources
- **Stateful Testing**: Support for stateful tests with proper state reset between test cases
- **Capturing Sink**: Enhanced capturing of traces for verification
- **Checkpoint Management**: Utilities for managing and validating checkpoint files
- **Metrics Collection**: Support for collecting and analyzing metrics during tests

### 2. New Test Utilities

Added a dedicated `test_utils.go` file providing test utilities to:

- Generate and send test traces with configurable parameters
- Verify sampling rate and trace completeness
- Simulate high load for performance testing
- Extract statistics from captured traces
- Compare checkpoint files for growth monitoring

### 3. Specialized Test Types

Added specialized test files for different testing scenarios:

- `performance_test.go`: Tests focused on throughput, memory usage, and scaling
- `stress_test.go`: Tests that apply extreme load and verify resilience
  - Includes stress tests, longevity tests, and recovery tests

### 4. Test Runner Script

Added `run_integration_tests.sh` script that:

- Provides an interactive way to run all test suites
- Captures test logs for analysis
- Reports successes, failures, and timing information
- Supports skipping time-intensive tests

### 5. Makefile Integration

Updated the Makefile with new targets:

- `stress-tests`: For running stress and longevity tests
- `performance-tests`: For running performance and scalability tests

## Detailed Improvements

### TestFramework Interface

The TestFramework now provides a comprehensive set of methods:

```go
// Setup creates and starts a new processor with options
func (tf *TestFramework) Setup(ctx context.Context, options ...ProcessorOption) error

// SendTraces sends traces to the processor
func (tf *TestFramework) SendTraces(ctx context.Context, traces ptrace.Traces) error

// SendTestTraces generates and sends test traces
func (tf *TestFramework) SendTestTraces(ctx context.Context, startIdx, count, spansPerTrace int) error

// ForceExport forces an immediate export
func (tf *TestFramework) ForceExport()

// Shutdown stops the processor
func (tf *TestFramework) Shutdown(ctx context.Context) error

// Cleanup releases all resources
func (tf *TestFramework) Cleanup() error

// Reset resets for a new test
func (tf *TestFramework) Reset(ctx context.Context) error

// GetCapturedTraces retrieves captured traces
func (tf *TestFramework) GetCapturedTraces() []ptrace.Traces

// CountUniqueTraces counts unique traces
func (tf *TestFramework) CountUniqueTraces() int

// CheckpointManagement methods
func (tf *TestFramework) GetCheckpointPath() string
func (tf *TestFramework) CheckpointFileExists() bool
func (tf *TestFramework) GetCheckpointFileSize() (int64, error)
```

### Test Utils Interface

The TestUtils provides utilities for common testing scenarios:

```go
// GenerateAndSendTraces generates and sends traces
func (tu *TestUtils) GenerateAndSendTraces(ctx context.Context, startIdx, numTraces, spansPerTrace int) ([]string, error)

// WaitForProcessing pauses for processing to complete
func (tu *TestUtils) WaitForProcessing(duration time.Duration)

// ForceExportAndWait forces export and waits
func (tu *TestUtils) ForceExportAndWait(waitDuration time.Duration)

// VerifySamplingRate checks sampling rate against expectations
func (tu *TestUtils) VerifySamplingRate(sentTraces []string, reservoirSize int) (float64, bool)

// VerifyTraceCompleteness verifies all spans from sampled traces are present
func (tu *TestUtils) VerifyTraceCompleteness(spansPerTrace int) (bool, map[string]int)

// CompareCheckpointSizes calculates growth ratio
func (tu *TestUtils) CompareCheckpointSizes(before, after int64) float64

// SimulateHighLoad generates high volume of traces
func (tu *TestUtils) SimulateHighLoad(ctx context.Context, numBatches, tracesPerBatch, spansPerTrace int, delayBetweenBatches time.Duration) error

// ExtractTraceStatistics gets detailed statistics
func (tu *TestUtils) ExtractTraceStatistics() map[string]interface{}
```

### New Test Cases

Added comprehensive test cases across different categories:

#### Performance Tests
- `TestReservoirSampling_Performance`: Tests throughput and memory usage
- `TestReservoirSampling_Scalability`: Tests scaling with increasing load
- `TestReservoirSampling_ResourceUsage`: Tests resource usage with different configurations

#### Stress Tests
- `TestReservoirSampling_StressTest`: Tests extreme load conditions
- `TestReservoirSampling_Longevity`: Tests extended operation over time
- `TestReservoirSampling_Recovery`: Tests recovery from simulated failures

#### Core Functionality Tests
- `TestReservoirSampling_BasicOperation`: Tests basic sampling
- `TestReservoirSampling_Persistence`: Tests checkpoint persistence
- `TestReservoirSampling_TraceAwareness`: Tests preservation of complete traces
- `TestReservoirSampling_WindowRollover`: Tests window-based sampling

#### NR-DOT Integration Tests
- `TestNRDOT_SamplingIntegration`: Tests integration with NR-DOT
- `TestNRDOT_OptimizedConfigurations`: Tests different optimization levels
- `TestNRDOT_EntitySpecificOptimizations`: Tests entity-specific optimizations

## Usage Examples

### Basic Integration Test

```go
func TestBasicSampling(t *testing.T) {
    // Create test framework
    tf, err := NewTestFramework(t, WithInMemoryDB())
    require.NoError(t, err, "Failed to create test framework")
    defer tf.Cleanup()

    // Setup processor
    ctx := context.Background()
    err = tf.Setup(ctx,
        WithReservoirSize(10),
        WithWindowDuration("1s"),
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

### Performance Test with Utils

```go
func TestPerformance(t *testing.T) {
    // Create test framework
    tf, err := NewTestFramework(t, WithInMemoryDB())
    require.NoError(t, err, "Failed to create test framework")
    defer tf.Cleanup()

    // Create test utilities
    utils := NewTestUtils(tf)

    // Setup processor
    ctx := context.Background()
    err = tf.Setup(ctx, WithReservoirSize(1000))
    require.NoError(t, err, "Failed to setup processor")

    // Run high load simulation
    err = utils.SimulateHighLoad(ctx, 10, 1000, 5, 100*time.Millisecond)
    require.NoError(t, err, "High load simulation failed")

    // Force export and wait
    utils.ForceExportAndWait(1 * time.Second)

    // Get statistics
    stats := utils.ExtractTraceStatistics()
    t.Logf("Trace statistics: %+v", stats)

    // Verify sampling rate
    uniqueTraceCount := tf.CountUniqueTraces()
    assert.LessOrEqual(t, uniqueTraceCount, 1000, "Too many traces captured")

    // Shutdown processor
    err = tf.Shutdown(ctx)
    require.NoError(t, err, "Failed to shutdown processor")
}
```

## Benefits

These improvements provide several benefits:

1. **Comprehensive Testing**: Tests now cover a wider range of scenarios
2. **Consistency**: Common test patterns are standardized
3. **Reusability**: Test utilities can be reused across different test types
4. **Maintainability**: Better organization makes tests easier to understand and modify
5. **Developer Experience**: Interactive test runner and helpful utilities improve DX

## Conclusion

The enhanced integration testing framework provides a solid foundation for verifying the correctness, performance, and reliability of the trace-aware reservoir sampling processor. It ensures that the processor meets its requirements and can handle real-world load and edge cases.