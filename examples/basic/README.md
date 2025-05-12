# Basic Example

This example demonstrates the basic usage of the trace-aware reservoir sampling processor with the OpenTelemetry Collector.

## Prerequisites

- OpenTelemetry Collector (v0.82.0 or later)
- Go 1.20 or later (for building the example client)

## Quick Start

### 1. Run the collector

```bash
# Create a data directory
mkdir -p data

# Run the collector
otelcol --config config.yaml
```

### 2. Send traces

Run the example client to send traces to the collector:

```bash
# Run the client
go run client.go
```

## Configuration Details

The provided `config.yaml` includes:

- OTLP receiver for both gRPC and HTTP protocols
- Batch processor for efficient trace processing
- Trace-aware reservoir sampling processor for sampling
- Logging exporter for debugging
- Health check extension for monitoring

## Example Client

The `client.go` file contains an example client that sends traces to the collector:

```go
package main

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	// Create OTLP exporter
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint("localhost:4317"),
		otlptracegrpc.WithInsecure(),
	)

	exp, err := otlptrace.New(context.Background(), client)
	if err != nil {
		log.Fatalf("Failed to create OTLP trace exporter: %v", err)
	}

	// Create resource and tracer provider
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("example-service"),
		semconv.ServiceVersion("1.0.0"),
		semconv.DeploymentEnvironment("example"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)

	// Create tracer
	tracer := tp.Tracer("example-tracer")

	// Send 100 traces
	for i := 0; i < 100; i++ {
		// Create a trace
		ctx, span := tracer.Start(context.Background(), "example-span")
		
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		
		// Create child spans
		for j := 0; j < 5; j++ {
			createChildSpan(ctx, tracer, j)
		}
		
		span.End()
		
		log.Printf("Sent trace %d/100", i+1)
	}

	log.Println("All traces sent to the collector")
}

func createChildSpan(ctx context.Context, tracer trace.Tracer, id int) {
	_, childSpan := tracer.Start(ctx, "child-span")
	
	// Simulate some work
	time.Sleep(5 * time.Millisecond)
	
	childSpan.End()
}
```

## Expected Output

When running the collector, you should see log messages indicating the traces are being processed and sampled. The reservoir sampling processor will maintain a sample of up to 5000 traces, allowing for statistical analysis of the trace data.