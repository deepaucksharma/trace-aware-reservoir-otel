// Package integration provides utilities for integration testing of the
// trace-aware reservoir sampling processor.
package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// TestUtils provides utility functions for integration testing
type TestUtils struct {
	framework *TestFramework
}

// NewTestUtils creates a new test utilities instance
func NewTestUtils(framework *TestFramework) *TestUtils {
	return &TestUtils{
		framework: framework,
	}
}

// GenerateAndSendTraces generates and sends the specified number of traces
// Returns the trace IDs that were sent
func (tu *TestUtils) GenerateAndSendTraces(ctx context.Context, startIdx, numTraces, spansPerTrace int) ([]string, error) {
	// Generate test traces
	traces := generateTestTraces(startIdx, numTraces, spansPerTrace)
	
	// Extract trace IDs for verification
	traceIDs := extractTraceIDs(traces)
	
	// Send traces to the processor
	err := tu.framework.SendTraces(ctx, traces)
	if err != nil {
		return nil, fmt.Errorf("failed to send traces: %w", err)
	}
	
	return traceIDs, nil
}

// WaitForProcessing waits for the processor to process traces
func (tu *TestUtils) WaitForProcessing(duration time.Duration) {
	time.Sleep(duration)
}

// ForceExportAndWait forces the processor to export traces and waits for completion
func (tu *TestUtils) ForceExportAndWait(waitDuration time.Duration) {
	tu.framework.ForceExport()
	time.Sleep(waitDuration)
}

// VerifySamplingRate checks that the sampling rate is within expected bounds
func (tu *TestUtils) VerifySamplingRate(sentTraces []string, reservoirSize int) (float64, bool) {
	// Get captured traces
	capturedTraces := tu.framework.GetCapturedTraces()
	if len(capturedTraces) == 0 {
		return 0, false
	}
	
	// Extract trace IDs from captured traces
	capturedTraceIDs := make(map[string]struct{})
	for _, batch := range capturedTraces {
		for i := 0; i < batch.ResourceSpans().Len(); i++ {
			rs := batch.ResourceSpans().At(i)
			for j := 0; j < rs.ScopeSpans().Len(); j++ {
				ss := rs.ScopeSpans().At(j)
				for k := 0; k < ss.Spans().Len(); k++ {
					span := ss.Spans().At(k)
					capturedTraceIDs[span.TraceID().String()] = struct{}{}
				}
			}
		}
	}
	
	// Count how many of the sent traces were captured
	var capturedCount int
	for _, traceID := range sentTraces {
		if _, found := capturedTraceIDs[traceID]; found {
			capturedCount++
		}
	}
	
	// Calculate sampling rate
	samplingRate := float64(capturedCount) / float64(len(sentTraces))
	
	// If we sent fewer traces than the reservoir size, we expect nearly all to be sampled
	if len(sentTraces) <= reservoirSize {
		return samplingRate, samplingRate >= 0.9 // Allow for some tolerance
	}
	
	// Otherwise, we expect approximately reservoirSize/len(sentTraces)
	expectedRate := float64(reservoirSize) / float64(len(sentTraces))
	tolerance := 0.25 // Allow for 25% tolerance due to randomness in sampling
	
	minExpectedRate := expectedRate * (1 - tolerance)
	maxExpectedRate := expectedRate * (1 + tolerance)
	
	return samplingRate, samplingRate >= minExpectedRate && samplingRate <= maxExpectedRate
}

// VerifyTraceCompleteness checks that sampled traces include all spans from that trace
func (tu *TestUtils) VerifyTraceCompleteness(spansPerTrace int) (bool, map[string]int) {
	// Get captured traces
	capturedTraces := tu.framework.GetCapturedTraces()
	if len(capturedTraces) == 0 {
		return false, nil
	}
	
	// Count spans per trace ID
	traceSpanCounts := make(map[string]int)
	
	for _, batch := range capturedTraces {
		for i := 0; i < batch.ResourceSpans().Len(); i++ {
			rs := batch.ResourceSpans().At(i)
			for j := 0; j < rs.ScopeSpans().Len(); j++ {
				ss := rs.ScopeSpans().At(j)
				for k := 0; k < ss.Spans().Len(); k++ {
					span := ss.Spans().At(k)
					traceID := span.TraceID().String()
					traceSpanCounts[traceID]++
				}
			}
		}
	}
	
	// Check if all sampled traces have the expected number of spans
	for traceID, count := range traceSpanCounts {
		if count != spansPerTrace {
			return false, traceSpanCounts
		}
	}
	
	return true, traceSpanCounts
}

// CompareCheckpointSizes compares two checkpoint file sizes and returns the growth ratio
func (tu *TestUtils) CompareCheckpointSizes(before, after int64) float64 {
	if before == 0 {
		return 0 // Cannot calculate ratio
	}
	return float64(after) / float64(before)
}

// SimulateHighLoad generates a high load of traces for performance testing
func (tu *TestUtils) SimulateHighLoad(ctx context.Context, numBatches, tracesPerBatch, spansPerTrace int, delayBetweenBatches time.Duration) error {
	tu.framework.logger.Info("Starting high load simulation",
		reservoirsampler.Int("num_batches", numBatches),
		reservoirsampler.Int("traces_per_batch", tracesPerBatch),
		reservoirsampler.Int("spans_per_trace", spansPerTrace),
		reservoirsampler.String("delay_between_batches", delayBetweenBatches.String()))
	
	startTime := time.Now()
	totalTraces := 0
	
	for i := 0; i < numBatches; i++ {
		batchStartTime := time.Now()
		
		// Generate and send a batch of traces
		traces := generateTestTraces(i*tracesPerBatch, tracesPerBatch, spansPerTrace)
		err := tu.framework.SendTraces(ctx, traces)
		if err != nil {
			return fmt.Errorf("failed to send batch %d: %w", i, err)
		}
		
		totalTraces += tracesPerBatch
		batchDuration := time.Since(batchStartTime)
		
		tu.framework.logger.Info("Sent batch",
			reservoirsampler.Int("batch", i+1),
			reservoirsampler.Int("total_traces", totalTraces),
			reservoirsampler.String("batch_duration", batchDuration.String()))
		
		// Wait between batches if specified
		if delayBetweenBatches > 0 && i < numBatches-1 {
			time.Sleep(delayBetweenBatches)
		}
	}
	
	totalDuration := time.Since(startTime)
	tracesPerSecond := float64(totalTraces) / totalDuration.Seconds()
	
	tu.framework.logger.Info("High load simulation completed",
		reservoirsampler.Int("total_traces", totalTraces),
		reservoirsampler.String("total_duration", totalDuration.String()),
		reservoirsampler.Float64("traces_per_second", tracesPerSecond))
	
	return nil
}

// ExtractTraceStatistics extracts statistics from captured traces
func (tu *TestUtils) ExtractTraceStatistics() map[string]interface{} {
	capturedTraces := tu.framework.GetCapturedTraces()
	
	stats := map[string]interface{}{
		"batch_count":        len(capturedTraces),
		"unique_trace_count": tu.framework.CountUniqueTraces(),
		"total_span_count":   0,
		"resource_count":     0,
		"scope_count":        0,
	}
	
	// Service names encountered
	serviceNames := make(map[string]int)
	
	// Traversal for detailed stats
	totalSpanCount := 0
	resourceCount := 0
	scopeCount := 0
	
	for _, batch := range capturedTraces {
		batchResourceCount := 0
		batchScopeCount := 0
		batchSpanCount := 0
		
		for i := 0; i < batch.ResourceSpans().Len(); i++ {
			rs := batch.ResourceSpans().At(i)
			batchResourceCount++
			
			// Extract service name if present
			if serviceName, ok := rs.Resource().Attributes().Get("service.name"); ok {
				serviceNames[serviceName.Str()]++
			}
			
			for j := 0; j < rs.ScopeSpans().Len(); j++ {
				ss := rs.ScopeSpans().At(j)
				batchScopeCount++
				batchSpanCount += ss.Spans().Len()
			}
		}
		
		totalSpanCount += batchSpanCount
		resourceCount += batchResourceCount
		scopeCount += batchScopeCount
	}
	
	stats["total_span_count"] = totalSpanCount
	stats["resource_count"] = resourceCount
	stats["scope_count"] = scopeCount
	stats["service_names"] = serviceNames
	
	return stats
}

// extractTraceIDs extracts trace IDs from a trace batch
func extractTraceIDs(traces ptrace.Traces) []string {
	var traceIDs []string
	uniqueTraceIDs := make(map[string]struct{})
	
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				traceID := span.TraceID().String()
				
				if _, exists := uniqueTraceIDs[traceID]; !exists {
					uniqueTraceIDs[traceID] = struct{}{}
					traceIDs = append(traceIDs, traceID)
				}
			}
		}
	}
	
	return traceIDs
}

// This file adds reusable testing utilities to complement the core framework