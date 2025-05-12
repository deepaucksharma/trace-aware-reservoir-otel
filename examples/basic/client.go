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