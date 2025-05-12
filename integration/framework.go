// Package integration provides a framework for integration testing of the
// trace-aware reservoir sampling processor.
package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

// TestOption defines functional options for configuring the test framework
type TestOption func(*TestFramework)

// ProcessorOption defines functional options for configuring the processor
type ProcessorOption func(*reservoirsampler.Config)

// TestFramework provides a framework for integration testing
type TestFramework struct {
	// Test configuration
	logger         *zap.Logger
	dataDir        string
	useInMemoryDB  bool
	cleanupDataDir bool

	// Processor configuration
	processorConfig *reservoirsampler.Config
	sink            consumer.Traces
	capturingSink   *CapturingSink
	processor       component.Component
	settings        processor.Settings

	// Test state
	traces          []ptrace.Traces
	uniqueTraceIDs  map[string]struct{}
	totalSpanCount  int
	uniqueTraceCount int
}

// WithDataDir specifies a custom data directory for the test
func WithDataDir(dir string) TestOption {
	return func(tf *TestFramework) {
		tf.dataDir = dir
		tf.cleanupDataDir = false // Don't clean up custom directories
	}
}

// WithInMemoryDB specifies to use an in-memory DB for testing
func WithInMemoryDB() TestOption {
	return func(tf *TestFramework) {
		tf.useInMemoryDB = true
	}
}

// WithCleanupDataDir specifies whether to clean up the data directory after the test
func WithCleanupDataDir(cleanup bool) TestOption {
	return func(tf *TestFramework) {
		tf.cleanupDataDir = cleanup
	}
}

// WithLogger specifies a custom logger for the test
func WithLogger(logger *zap.Logger) TestOption {
	return func(tf *TestFramework) {
		tf.logger = logger
	}
}

// WithReservoirSize sets the reservoir size for the processor
func WithReservoirSize(size int) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.SizeK = size
	}
}

// WithWindowDuration sets the window duration for the processor
func WithWindowDuration(duration string) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.WindowDuration = duration
	}
}

// WithCheckpointInterval sets the checkpoint interval for the processor
func WithCheckpointInterval(interval string) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.CheckpointInterval = interval
	}
}

// WithTraceAware sets whether the processor is trace-aware
func WithTraceAware(traceAware bool) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.TraceAware = traceAware
	}
}

// WithTraceBufferMaxSize sets the trace buffer max size for the processor
func WithTraceBufferMaxSize(size int) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.TraceBufferMaxSize = size
	}
}

// WithTraceBufferTimeout sets the trace buffer timeout for the processor
func WithTraceBufferTimeout(timeout string) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.TraceBufferTimeout = timeout
	}
}

// WithDbCompactionSchedule sets the DB compaction schedule for the processor
func WithDbCompactionSchedule(schedule string) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.DbCompactionScheduleCron = schedule
	}
}

// WithDbCompactionTargetSize sets the DB compaction target size for the processor
func WithDbCompactionTargetSize(size int64) ProcessorOption {
	return func(cfg *reservoirsampler.Config) {
		cfg.DbCompactionTargetSize = size
	}
}

// NewTestFramework creates a new test framework with the given options
func NewTestFramework(t zaptest.TestingT, options ...TestOption) (*TestFramework, error) {
	// Create base framework with defaults
	tf := &TestFramework{
		logger:         zaptest.NewLogger(t, zaptest.Level(zapcore.DebugLevel)),
		dataDir:        "",
		useInMemoryDB:  false,
		cleanupDataDir: true,
		uniqueTraceIDs: make(map[string]struct{}),
	}

	// Apply options
	for _, opt := range options {
		opt(tf)
	}

	// Create data directory if needed
	if tf.dataDir == "" {
		var err error
		tf.dataDir, err = os.MkdirTemp("", "reservoir-test")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Create the base sink
	sink := NewNoopTracesSink()
	tf.sink = sink
	tf.capturingSink = NewCapturingSink(sink)

	return tf, nil
}

// Setup creates and starts a new processor with the given options
func (tf *TestFramework) Setup(ctx context.Context, options ...ProcessorOption) error {
	// Create default config
	cfg := &reservoirsampler.Config{
		SizeK:                    50,
		WindowDuration:           "10s",
		TraceAware:               true,
		TraceBufferMaxSize:       10000,
		TraceBufferTimeout:       "5s",
		CheckpointInterval:       "5s",
		DbCompactionScheduleCron: "*/5 * * * *", // Every 5 minutes
		DbCompactionTargetSize:   104857600,     // 100MB
	}

	// Set checkpoint path based on configuration
	if tf.useInMemoryDB {
		cfg.CheckpointPath = "" // Empty means in-memory storage
	} else {
		cfg.CheckpointPath = filepath.Join(tf.dataDir, "reservoir.db")
	}

	// Apply processor options
	for _, opt := range options {
		opt(cfg)
	}

	// Save config for later use
	tf.processorConfig = cfg

	// Create telemetry settings
	telemetrySettings := componenttest.NewNopTelemetrySettings()
	telemetrySettings.Logger = tf.logger

	// Create processor settings
	tf.settings = processor.Settings{
		TelemetrySettings: telemetrySettings,
	}

	// Create processor
	proc, err := reservoirsampler.CreateTracesProcessorForTesting(
		ctx,
		tf.settings,
		cfg,
		tf.capturingSink,
	)
	if err != nil {
		return fmt.Errorf("failed to create processor: %w", err)
	}
	tf.processor = proc

	// Start processor
	err = proc.Start(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start processor: %w", err)
	}

	tf.logger.Info("Processor started", zap.String("checkpoint_path", cfg.CheckpointPath))
	return nil
}

// SendTraces sends the provided traces to the processor
func (tf *TestFramework) SendTraces(ctx context.Context, traces ptrace.Traces) error {
	if tf.processor == nil {
		return fmt.Errorf("processor not started, call Setup first")
	}

	// Add to internal state
	tf.traces = append(tf.traces, traces)
	tf.totalSpanCount += traces.SpanCount()

	// Count unique trace IDs
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				traceID := span.TraceID().String()
				tf.uniqueTraceIDs[traceID] = struct{}{}
			}
		}
	}
	tf.uniqueTraceCount = len(tf.uniqueTraceIDs)

	// Send to processor
	return tf.processor.(consumer.Traces).ConsumeTraces(ctx, traces)
}

// SendTestTraces generates and sends test traces with the specified parameters
func (tf *TestFramework) SendTestTraces(ctx context.Context, startIdx, count, spansPerTrace int) error {
	traces := generateTestTraces(startIdx, count, spansPerTrace)
	return tf.SendTraces(ctx, traces)
}

// ForceExport forces the processor to export traces
func (tf *TestFramework) ForceExport() {
	if tf.processor == nil {
		tf.logger.Warn("Processor not started, cannot force export")
		return
	}

	reservoirsampler.ForceReservoirExport(tf.processor)
}

// Shutdown stops the processor
func (tf *TestFramework) Shutdown(ctx context.Context) error {
	if tf.processor == nil {
		return nil
	}

	err := tf.processor.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("failed to shutdown processor: %w", err)
	}

	tf.logger.Info("Processor shutdown", zap.Int("unique_traces", tf.uniqueTraceCount))
	return nil
}

// Cleanup cleans up all resources used by the test framework
func (tf *TestFramework) Cleanup() error {
	// Cleanup data directory if needed
	if tf.cleanupDataDir && tf.dataDir != "" {
		if err := os.RemoveAll(tf.dataDir); err != nil {
			return fmt.Errorf("failed to remove data directory: %w", err)
		}
	}

	return nil
}

// Reset resets the test framework for a new test
func (tf *TestFramework) Reset(ctx context.Context) error {
	// Shutdown existing processor
	if tf.processor != nil {
		if err := tf.Shutdown(ctx); err != nil {
			return err
		}
	}

	// Reset capturing sink
	tf.capturingSink.Reset()

	// Reset trace state
	tf.traces = nil
	tf.uniqueTraceIDs = make(map[string]struct{})
	tf.totalSpanCount = 0
	tf.uniqueTraceCount = 0

	return nil
}

// GetCapturedTraces returns all captured traces
func (tf *TestFramework) GetCapturedTraces() []ptrace.Traces {
	return tf.capturingSink.GetAllTraces()
}

// CountUniqueTraces counts unique traces in the captured traces
func (tf *TestFramework) CountUniqueTraces() int {
	traces := tf.GetCapturedTraces()
	return countUniqueTraces(traces)
}

// GetCheckpointPath returns the path to the checkpoint file
func (tf *TestFramework) GetCheckpointPath() string {
	if tf.processorConfig == nil {
		return ""
	}
	return tf.processorConfig.CheckpointPath
}

// CheckpointFileExists checks if the checkpoint file exists
func (tf *TestFramework) CheckpointFileExists() bool {
	path := tf.GetCheckpointPath()
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// GetCheckpointFileSize returns the size of the checkpoint file
func (tf *TestFramework) GetCheckpointFileSize() (int64, error) {
	path := tf.GetCheckpointPath()
	if path == "" {
		return 0, fmt.Errorf("no checkpoint path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// NoopTracesSink is a consumer that discards all traces
type NoopTracesSink struct{}

// ConsumeTraces discards traces
func (s *NoopTracesSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	return nil
}

// Capabilities returns consumer capabilities
func (s *NoopTracesSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// NewNoopTracesSink creates a sink that discards all traces
func NewNoopTracesSink() consumer.Traces {
	return &NoopTracesSink{}
}

// CapturingSink is a consumer that captures all traces before passing them to the next consumer
type CapturingSink struct {
	nextConsumer consumer.Traces
	traces       []ptrace.Traces
	mutex        *sync.Mutex
}

// NewCapturingSink creates a capturing sink that records all traces sent to it
func NewCapturingSink(nextConsumer consumer.Traces) *CapturingSink {
	return &CapturingSink{
		nextConsumer: nextConsumer,
		traces:       make([]ptrace.Traces, 0),
		mutex:        &sync.Mutex{},
	}
}

// ConsumeTraces captures traces and forwards them
func (s *CapturingSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Make a copy of the traces
	tracesCopy := ptrace.NewTraces()
	td.CopyTo(tracesCopy)
	s.traces = append(s.traces, tracesCopy)

	return s.nextConsumer.ConsumeTraces(ctx, td)
}

// GetAllTraces returns all captured traces
func (s *CapturingSink) GetAllTraces() []ptrace.Traces {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.traces
}

// Reset clears all captured traces
func (s *CapturingSink) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.traces = make([]ptrace.Traces, 0)
}

// Capabilities returns consumer capabilities
func (s *CapturingSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// TeeLogger creates a logger that writes to both a file and the testing log
func TeeLogger(t zaptest.TestingT, logPath string) (*zap.Logger, error) {
	// Create the directory for the log file if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create the log file
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// Create a core that tees the output to both test log and file
	consoleCore := zaptest.NewLogger(t, zaptest.Level(zapcore.DebugLevel)).Core()
	fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	fileCore := zapcore.NewCore(fileEncoder, zapcore.AddSync(logFile), zapcore.DebugLevel)

	// Create a logger with the tee core
	return zap.New(zapcore.NewTee(consoleCore, fileCore)), nil
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// generateTestTraces creates test traces with unique trace IDs
func generateTestTraces(startIdx, count, spansPerTrace int) ptrace.Traces {
	traces := ptrace.NewTraces()

	for i := 0; i < count; i++ {
		traceIdx := startIdx + i
		traceID := generateTraceID(traceIdx)

		for j := 0; j < spansPerTrace; j++ {
			rs := traces.ResourceSpans().AppendEmpty()
			res := rs.Resource()

			// Add minimal resource attributes to avoid excessive memory use
			attrs := res.Attributes()
			attrs.PutStr("service.name", fmt.Sprintf("svc-%d", traceIdx))

			ss := rs.ScopeSpans().AppendEmpty()
			scope := ss.Scope()
			scope.SetName("test")

			span := ss.Spans().AppendEmpty()
			span.SetTraceID(traceID)
			span.SetSpanID(generateSpanID(traceIdx*100 + j))

			// Set parent span ID for all except the first span
			if j > 0 {
				span.SetParentSpanID(generateSpanID(traceIdx * 100))
			}

			span.SetName(fmt.Sprintf("sp-%d-%d", traceIdx, j))
			span.SetKind(ptrace.SpanKindServer)

			// Set timestamps
			startTime := time.Now().Add(-10 * time.Second)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(startTime.Add(100 * time.Millisecond)))

			// Add minimal span attributes
			spanAttrs := span.Attributes()
			spanAttrs.PutInt("idx", int64(traceIdx))
		}
	}

	return traces
}

// generateTraceID creates a deterministic trace ID from an index
func generateTraceID(index int) pcommon.TraceID {
	var traceID pcommon.TraceID
	traceID[0] = byte(index >> 8)
	traceID[1] = byte(index)
	// Fill the rest with non-zero values
	for i := 2; i < len(traceID); i++ {
		traceID[i] = byte(i)
	}
	return traceID
}

// generateSpanID creates a deterministic span ID from an index
func generateSpanID(index int) pcommon.SpanID {
	var spanID pcommon.SpanID
	spanID[0] = byte(index >> 8)
	spanID[1] = byte(index)
	// Fill the rest with non-zero values
	for i := 2; i < len(spanID); i++ {
		spanID[i] = byte(i)
	}
	return spanID
}

// countUniqueTraces counts unique traces across all trace batches
func countUniqueTraces(traceBatches []ptrace.Traces) int {
	// Use a map to track unique trace IDs
	uniqueTraces := make(map[string]struct{})

	for _, batch := range traceBatches {
		for i := 0; i < batch.ResourceSpans().Len(); i++ {
			rs := batch.ResourceSpans().At(i)

			for j := 0; j < rs.ScopeSpans().Len(); j++ {
				ss := rs.ScopeSpans().At(j)

				for k := 0; k < ss.Spans().Len(); k++ {
					span := ss.Spans().At(k)
					traceID := span.TraceID().String()
					uniqueTraces[traceID] = struct{}{}
				}
			}
		}
	}

	return len(uniqueTraces)
}