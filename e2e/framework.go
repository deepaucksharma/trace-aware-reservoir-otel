package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TestFramework provides utilities for e2e testing
type TestFramework struct {
	logger       *zap.Logger
	collectorCmd *exec.Cmd
	configPath   string
	otlpEndpoint string
}

// NewTestFramework creates a new TestFramework
func NewTestFramework(configPath string) (*TestFramework, error) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return &TestFramework{
		logger:       logger,
		configPath:   configPath,
		otlpEndpoint: "localhost:4317", // Default OTLP endpoint
	}, nil
}

// StartCollector starts the OpenTelemetry Collector with the specified config
func (f *TestFramework) StartCollector(ctx context.Context) error {
	// Ensure the config file exists
	configPath, err := filepath.Abs(f.configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for config: %w", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}

	// Start the collector
	collectorPath := filepath.Join(".", "bin", "otelcol")
	cmd := exec.CommandContext(ctx, collectorPath, "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start collector: %w", err)
	}

	f.collectorCmd = cmd
	f.logger.Info("Started collector", zap.String("config", configPath))

	// Wait a moment for the collector to start
	time.Sleep(2 * time.Second)

	return nil
}

// StopCollector stops the running collector
func (f *TestFramework) StopCollector() error {
	if f.collectorCmd == nil || f.collectorCmd.Process == nil {
		return nil
	}

	if err := f.collectorCmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill collector: %w", err)
	}

	f.logger.Info("Stopped collector")
	return nil
}

// CreateTraceClient creates a new OTLP trace client for sending traces
func (f *TestFramework) CreateTraceClient(ctx context.Context, serviceName string) (trace.Tracer, *sdktrace.TracerProvider, error) {
	// Create OTLP exporter
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(f.otlpEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Create resource and tracer provider
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	tracer := tp.Tracer(serviceName)
	return tracer, tp, nil
}

// GenerateTestSpans generates a specified number of test spans for benchmarking
func (f *TestFramework) GenerateTestSpans(count int, spansPerTrace int) ptrace.Traces {
	traces := ptrace.NewTraces()
	
	for i := 0; i < count; i += spansPerTrace {
		traceID := generateTraceID(i)
		
		for j := 0; j < spansPerTrace && i+j < count; j++ {
			rs := traces.ResourceSpans().AppendEmpty()
			res := rs.Resource()
			
			// Add resource attributes
			attrs := res.Attributes()
			attrs.PutStr("service.name", fmt.Sprintf("test-service-%d", i/spansPerTrace))
			attrs.PutStr("service.version", "1.0.0")
			attrs.PutStr("deployment.environment", "e2e-test")
			
			ils := rs.ScopeSpans().AppendEmpty()
			scope := ils.Scope()
			scope.SetName("e2e-test-scope")
			scope.SetVersion("1.0.0")
			
			span := ils.Spans().AppendEmpty()
			span.SetTraceID(traceID)
			span.SetSpanID(generateSpanID(i + j))
			
			// Set parent span ID for all except the first span in the trace
			if j > 0 {
				span.SetParentSpanID(generateSpanID(i))
			}
			
			span.SetName(fmt.Sprintf("test-span-%d-%d", i/spansPerTrace, j))
			span.SetKind(ptrace.SpanKindServer)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-1 * time.Second)))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))
			
			// Add span attributes
			spanAttrs := span.Attributes()
			spanAttrs.PutInt("test.iteration", int64(i+j))
			spanAttrs.PutBool("test.valid", true)
			spanAttrs.PutDouble("test.value", float64(i+j)/float64(count))
		}
	}
	
	return traces
}

// generateTraceID creates a deterministic trace ID based on the index
func generateTraceID(index int) pcommon.TraceID {
	var traceID pcommon.TraceID
	traceID[0] = byte(index >> 8)
	traceID[1] = byte(index)
	return traceID
}

// generateSpanID creates a deterministic span ID based on the index
func generateSpanID(index int) pcommon.SpanID {
	var spanID pcommon.SpanID
	spanID[0] = byte(index >> 8)
	spanID[1] = byte(index)
	return spanID
}

// SendTraces sends the provided traces to the collector
func (f *TestFramework) SendTraces(ctx context.Context, traces ptrace.Traces) error {
	// Implementation of sending traces directly to the collector
	// This could use the OTLP client we created or a direct call to the collector API
	
	// This is a placeholder implementation
	log.Printf("Sending %d resource spans", traces.ResourceSpans().Len())
	
	// In a real implementation, you would serialize and send these traces
	// to the collector via OTLP
	
	return nil
}

// VerifyTraceStats checks basic stats about the traces that were processed
func (f *TestFramework) VerifyTraceStats(ctx context.Context) (int, int, error) {
	// This would typically involve querying the collector metrics
	// or checking the output of the collector
	
	// For now, we'll just return placeholder values
	return 0, 0, nil
}