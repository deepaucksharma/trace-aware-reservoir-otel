# Examples

This directory contains examples and sample configurations for the trace-aware reservoir sampling processor.

## Directory Structure

- [configs/](configs/): Ready-to-use configuration files
- [basic/](basic/): Basic examples for getting started
- [docker/](docker/): Docker and Docker Compose examples
- [kubernetes/](kubernetes/): Kubernetes deployment examples
- [nrdot/](nrdot/): New Relic Distribution of OpenTelemetry integration examples

## Quick Start

### 1. Generate Configuration File

Use the `pte` command-line tool to generate a configuration file:

```bash
# Generate default configuration
pte generate-config --output config.yaml

# Generate high-volume configuration
pte generate-config --template high-volume --output config-high-volume.yaml

# Generate low-resource configuration
pte generate-config --template low-resource --output config-low-resource.yaml
```

### 2. Run with Docker

```bash
# Run with Docker
docker run -v $(pwd)/config.yaml:/etc/otel/config.yaml \
           -v $(pwd)/data:/var/otelpersist \
           -p 4317:4317 -p 4318:4318 -p 8888:8888 \
           otel/opentelemetry-collector-contrib:latest \
           --config /etc/otel/config.yaml
```

### 3. Run with Docker Compose

```bash
# Run with Docker Compose
cd examples/docker
docker-compose up -d
```

### 4. Deploy to Kubernetes

```bash
# Deploy to Kubernetes
cd examples/kubernetes
kubectl apply -f .
```

### 5. Integrate with NR-DOT

```bash
# Generate NR-DOT configuration
pte nrdot-integration --generate-config --output nrdot-config.yaml

# Register with NR-DOT
pte nrdot-integration --nrdot-path /path/to/nrdot
```

## Sample Usage

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

	// Create a trace
	ctx, span := tracer.Start(context.Background(), "example-span")
	defer span.End()

	// Create child spans
	for i := 0; i < 5; i++ {
		_, childSpan := tracer.Start(ctx, "child-span")
		time.Sleep(10 * time.Millisecond)
		childSpan.End()
	}

	log.Println("Traces sent to the collector")
}
```

## Configuration Examples

### Basic Configuration

```yaml
processors:
  reservoir_sampler:
    size_k: 5000
    window_duration: "60s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "10s"
    trace_aware: true
    trace_buffer_max_size: 100000
    trace_buffer_timeout: "30s"
```

### High-Volume Configuration

```yaml
processors:
  reservoir_sampler:
    size_k: 10000
    window_duration: "30s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "15s"
    trace_aware: true
    trace_buffer_max_size: 200000
    trace_buffer_timeout: "15s"
    db_compaction_schedule_cron: "0 */6 * * *" # Every 6 hours
    db_compaction_target_size: 1073741824 # 1GB
```

### NR-DOT Configuration

```yaml
processors:
  reservoir_sampler:
    size_k: 5000
    window_duration: "60s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "10s"
    trace_aware: true
    trace_buffer_max_size: 100000
    trace_buffer_timeout: "30s"

exporters:
  otlphttp/newrelic:
    endpoint: "https://otlp.nr-data.net:4318"
    headers:
      api-key: YOUR_LICENSE_KEY_HERE
```