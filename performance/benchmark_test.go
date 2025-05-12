package performance

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap"
)

func BenchmarkReservoirProcessor(b *testing.B) {
	benchmarks := []struct {
		name           string
		traceAware     bool
		persistence    bool
		reservoirSize  int
		windowDuration string
		spansPerBatch  int
		tracesPerBatch int
		spansPerTrace  int
	}{
		{
			name:           "StandardSampling_SmallReservoir",
			traceAware:     false,
			persistence:    false,
			reservoirSize:  100,
			windowDuration: "10s",
			spansPerBatch:  1000,
			tracesPerBatch: 100,
			spansPerTrace:  10,
		},
		{
			name:           "StandardSampling_LargeReservoir",
			traceAware:     false,
			persistence:    false,
			reservoirSize:  10000,
			windowDuration: "10s",
			spansPerBatch:  1000,
			tracesPerBatch: 100,
			spansPerTrace:  10,
		},
		{
			name:           "TraceAwareSampling_SmallReservoir",
			traceAware:     true,
			persistence:    false,
			reservoirSize:  100,
			windowDuration: "10s",
			spansPerBatch:  1000,
			tracesPerBatch: 100,
			spansPerTrace:  10,
		},
		{
			name:           "TraceAwareSampling_LargeReservoir",
			traceAware:     true,
			persistence:    false,
			reservoirSize:  10000,
			windowDuration: "10s",
			spansPerBatch:  1000,
			tracesPerBatch: 100,
			spansPerTrace:  10,
		},
		{
			name:           "PersistentSampling_SmallReservoir",
			traceAware:     true,
			persistence:    true,
			reservoirSize:  100,
			windowDuration: "10s",
			spansPerBatch:  1000,
			tracesPerBatch: 100,
			spansPerTrace:  10,
		},
		{
			name:           "PersistentSampling_LargeReservoir",
			traceAware:     true,
			persistence:    true,
			reservoirSize:  10000,
			windowDuration: "10s",
			spansPerBatch:  1000,
			tracesPerBatch: 100,
			spansPerTrace:  10,
		},
	}

	// Run the benchmarks
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create temporary directory for persistence if needed
			var dbPath string
			if bm.persistence {
				tempDir := b.TempDir()
				dbPath = fmt.Sprintf("%s/reservoir.db", tempDir)
			}

			// Create processor config
			cfg := &reservoirsampler.Config{
				SizeK:                    bm.reservoirSize,
				WindowDuration:           bm.windowDuration,
				TraceAware:               bm.traceAware,
				TraceBufferMaxSize:       100000,
				TraceBufferTimeout:       "5s",
				CheckpointPath:           dbPath,
				CheckpointInterval:       "10s",
				DbCompactionScheduleCron: "0 0 * * *", // Daily at midnight
			}

			// Create logger
			logger := zap.NewNop()

			// Create sink
			sink := &countingSink{}

			// Reset the timer for each benchmark iteration
			b.ResetTimer()

			// Run benchmark
			b.RunParallel(func(pb *testing.PB) {
				// Create processor for each goroutine
				// Factory is not used directly anymore
				// Create telemetry settings with noop meter provider
				telSettings := component.TelemetrySettings{
					Logger:        logger,
					MeterProvider: noop.NewMeterProvider(),
				}

				settings := processor.Settings{
					TelemetrySettings: telSettings,
				}

				// Use exported test function
				processor, err := reservoirsampler.CreateTracesProcessorForTesting(
					context.Background(),
					settings,
					cfg,
					sink,
				)
				require.NoError(b, err)

				// Start the processor
				err = processor.Start(context.Background(), nil)
				require.NoError(b, err)
				defer func() {
					err := processor.Shutdown(context.Background())
					if err != nil {
						b.Logf("Warning: failed to shutdown processor: %v", err)
					}
				}()

				// Generate test data
				traces := generateTestTraces(bm.tracesPerBatch, bm.spansPerTrace)

				// Run the benchmark loop
				for pb.Next() {
					if err := processor.ConsumeTraces(context.Background(), traces); err != nil {
						b.Logf("Warning: failed to consume traces: %v", err)
					}
				}
			})
		})
	}
}

func BenchmarkReservoirSampling_HighThroughput(b *testing.B) {
	// This benchmark tests the processor's performance under high throughput conditions
	// It sends a large volume of spans in rapid succession

	// Create processor config
	cfg := &reservoirsampler.Config{
		SizeK:              10000,
		WindowDuration:     "10s",
		TraceAware:         true,
		TraceBufferMaxSize: 100000,
		TraceBufferTimeout: "5s",
		CheckpointPath:     "",
		CheckpointInterval: "10s",
	}

	// Create logger
	logger := zap.NewNop()

	// Create sink that tracks processed spans
	sink := &countingSink{}

	// Create processor
	// Factory is not used directly anymore
	// Create telemetry settings with noop meter provider
	telSettings := component.TelemetrySettings{
		Logger:        logger,
		MeterProvider: noop.NewMeterProvider(),
	}

	settings := processor.Settings{
		TelemetrySettings: telSettings,
	}

	// Use exported test function
	processor, err := reservoirsampler.CreateTracesProcessorForTesting(
		context.Background(),
		settings,
		cfg,
		sink,
	)
	require.NoError(b, err)

	// Start the processor
	err = processor.Start(context.Background(), nil)
	require.NoError(b, err)
	defer func() {
		err := processor.Shutdown(context.Background())
		if err != nil {
			b.Logf("Warning: failed to shutdown processor: %v", err)
		}
	}()

	// Generate a large number of traces
	const (
		totalTraces   = 5000
		spansPerTrace = 10
		batchSize     = 500
	)

	// Test large batches of traces
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for batchStart := 0; batchStart < totalTraces; batchStart += batchSize {
			// Calculate batch end, ensuring we don't go past totalTraces
			batchEnd := batchStart + batchSize
			if batchEnd > totalTraces {
				batchEnd = totalTraces
			}

			// Generate and send batch
			traces := generateTestTraces(batchEnd-batchStart, spansPerTrace)
			err := processor.ConsumeTraces(context.Background(), traces)
			assert.NoError(b, err)
		}
	}

	// Report additional metrics
	spansProcessed := sink.spans
	b.ReportMetric(float64(spansProcessed)/float64(b.N), "spans/op")
	b.ReportMetric(float64(spansProcessed)/b.Elapsed().Seconds(), "spans/sec")
}

func BenchmarkTraceCompleteness(b *testing.B) {
	// This benchmark tests the processor's ability to maintain trace completeness
	// It sends spans belonging to the same trace with varying patterns

	benchmarks := []struct {
		name            string
		completeTraces  float64 // Percentage of traces that are complete (all spans arrive)
		partialTraces   float64 // Percentage of traces that are partial (some spans missing)
		outOfOrderSpans float64 // Percentage of spans that arrive out of order
	}{
		{
			name:            "AllComplete",
			completeTraces:  1.0,
			partialTraces:   0.0,
			outOfOrderSpans: 0.0,
		},
		{
			name:            "MixedCompleteness",
			completeTraces:  0.7,
			partialTraces:   0.3,
			outOfOrderSpans: 0.1,
		},
		{
			name:            "HighlyFragmented",
			completeTraces:  0.3,
			partialTraces:   0.7,
			outOfOrderSpans: 0.5,
		},
	}

	// Run the benchmarks
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create processor config
			cfg := &reservoirsampler.Config{
				SizeK:              1000,
				WindowDuration:     "10s",
				TraceAware:         true,
				TraceBufferMaxSize: 10000,
				TraceBufferTimeout: "5s",
				CheckpointPath:     "",
				CheckpointInterval: "10s",
			}

			// Create logger
			logger := zap.NewNop()

			// Create sink that tracks processed spans
			sink := &countingSink{}

			// Create processor
			// Factory is not used directly anymore
			// Create telemetry settings with a noop meter provider
			telSettings := component.TelemetrySettings{
				Logger:        logger,
				MeterProvider: noop.NewMeterProvider(),
			}

			settings := processor.Settings{
				TelemetrySettings: telSettings,
			}

			// Use exported test function
			processor, err := reservoirsampler.CreateTracesProcessorForTesting(
				context.Background(),
				settings,
				cfg,
				sink,
			)
			require.NoError(b, err)

			// Start the processor
			err = processor.Start(context.Background(), nil)
			require.NoError(b, err)
			defer func() {
				err := processor.Shutdown(context.Background())
				if err != nil {
					b.Logf("Warning: failed to shutdown processor: %v", err)
				}
			}()

			// Constants for this benchmark
			const (
				totalTraces   = 1000
				spansPerTrace = 10
			)

			// Generate test data with randomization
			// No need to call rand.Seed in Go 1.20+ as it's automatically initialized

			// Create a slice of all spans from all traces
			var allSpans []spanInfo

			for i := 0; i < totalTraces; i++ {
				traceID := generateTraceID(i)

				// Determine if this trace will be complete
				isComplete := rand.Float64() < bm.completeTraces

				// Number of spans to include (all for complete traces, random for partial)
				spansToInclude := spansPerTrace
				if !isComplete {
					// For partial traces, include between 1 and spansPerTrace-1 spans
					spansToInclude = 1 + rand.Intn(spansPerTrace-1)
				}

				// Create spans for this trace
				for j := 0; j < spansToInclude; j++ {
					allSpans = append(allSpans, spanInfo{
						traceID:  traceID,
						spanID:   generateSpanID(i*100 + j),
						parentID: getParentID(j, i), // First span has no parent
						name:     fmt.Sprintf("span-%d-%d", i, j),
						rootSpan: j == 0,
						traceIdx: i,
						spanIdx:  j,
					})
				}
			}

			// Shuffle spans if out-of-order ratio > 0
			if bm.outOfOrderSpans > 0 {
				// Determine how many spans to shuffle
				spansToShuffle := int(float64(len(allSpans)) * bm.outOfOrderSpans)

				// Shuffle a portion of the spans
				for i := 0; i < spansToShuffle; i++ {
					j := rand.Intn(len(allSpans))
					k := rand.Intn(len(allSpans))
					allSpans[j], allSpans[k] = allSpans[k], allSpans[j]
				}
			}

			// Reset the timer for the benchmark
			b.ResetTimer()

			// Run the benchmark
			for i := 0; i < b.N; i++ {
				// Process spans in batches
				batchSize := 100
				for batchStart := 0; batchStart < len(allSpans); batchStart += batchSize {
					batchEnd := batchStart + batchSize
					if batchEnd > len(allSpans) {
						batchEnd = len(allSpans)
					}

					// Create a batch of spans
					traceBatch := ptrace.NewTraces()
					for j := batchStart; j < batchEnd; j++ {
						span := allSpans[j]
						addSpanToTraces(traceBatch, span)
					}

					// Process the batch
					err := processor.ConsumeTraces(context.Background(), traceBatch)
					assert.NoError(b, err)
				}

				// Allow some time for processing
				time.Sleep(100 * time.Millisecond)
			}

			// Report additional metrics
			spansProcessed := sink.spans
			b.ReportMetric(float64(spansProcessed)/float64(b.N), "spans/op")
		})
	}
}

// Helper functions and types

// We'll just use the noop implementation directly from the package

type countingSink struct {
	traces int
	spans  int
}

func (s *countingSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	s.traces++
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			s.spans += ss.Spans().Len()
		}
	}
	return nil
}

func (s *countingSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// spanInfo holds information about a single span
type spanInfo struct {
	traceID  pcommon.TraceID
	spanID   pcommon.SpanID
	parentID pcommon.SpanID
	name     string
	rootSpan bool
	traceIdx int
	spanIdx  int
}

// generateTestTraces creates test traces for benchmarking
func generateTestTraces(traceCount, spansPerTrace int) ptrace.Traces {
	traces := ptrace.NewTraces()

	for i := 0; i < traceCount; i++ {
		traceID := generateTraceID(i)

		for j := 0; j < spansPerTrace; j++ {
			rs := traces.ResourceSpans().AppendEmpty()
			res := rs.Resource()

			// Add resource attributes
			attrs := res.Attributes()
			attrs.PutStr("service.name", fmt.Sprintf("test-service-%d", i))
			attrs.PutStr("environment", "benchmark")

			ss := rs.ScopeSpans().AppendEmpty()
			scope := ss.Scope()
			scope.SetName("benchmark-scope")

			span := ss.Spans().AppendEmpty()
			span.SetTraceID(traceID)
			span.SetSpanID(generateSpanID(i*100 + j))

			// Set parent span ID for all except the first span
			if j > 0 {
				span.SetParentSpanID(generateSpanID(i * 100))
			}

			span.SetName(fmt.Sprintf("span-%d-%d", i, j))
			span.SetKind(ptrace.SpanKindServer)

			// Set timestamps
			startTime := time.Now().Add(-10 * time.Second)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(startTime.Add(100 * time.Millisecond)))

			// Add span attributes
			spanAttrs := span.Attributes()
			spanAttrs.PutInt("trace.idx", int64(i))
			spanAttrs.PutInt("span.idx", int64(j))
		}
	}

	return traces
}

// getParentID returns a parent ID for non-root spans
func getParentID(spanIndex, traceIndex int) pcommon.SpanID {
	if spanIndex > 0 {
		return generateSpanID(traceIndex * 100)
	}
	return pcommon.SpanID{}
}

// addSpanToTraces adds a single span to a Traces object
func addSpanToTraces(traces ptrace.Traces, span spanInfo) {
	rs := traces.ResourceSpans().AppendEmpty()
	res := rs.Resource()

	// Add resource attributes
	attrs := res.Attributes()
	attrs.PutStr("service.name", fmt.Sprintf("test-service-%d", span.traceIdx))
	attrs.PutStr("environment", "benchmark")

	ss := rs.ScopeSpans().AppendEmpty()
	scope := ss.Scope()
	scope.SetName("benchmark-scope")

	pspan := ss.Spans().AppendEmpty()
	pspan.SetTraceID(span.traceID)
	pspan.SetSpanID(span.spanID)

	// Set parent ID if applicable
	if !span.rootSpan {
		pspan.SetParentSpanID(span.parentID)
	}

	pspan.SetName(span.name)
	pspan.SetKind(ptrace.SpanKindServer)

	// Set timestamps
	startTime := time.Now().Add(-10 * time.Second)
	pspan.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	pspan.SetEndTimestamp(pcommon.NewTimestampFromTime(startTime.Add(100 * time.Millisecond)))

	// Add span attributes
	spanAttrs := pspan.Attributes()
	spanAttrs.PutInt("trace.idx", int64(span.traceIdx))
	spanAttrs.PutInt("span.idx", int64(span.spanIdx))
}

// generateTraceID creates a deterministic trace ID based on the index
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

// generateSpanID creates a deterministic span ID based on the index
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
