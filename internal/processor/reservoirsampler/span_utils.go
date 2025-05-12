package reservoirsampler

import (
	"github.com/cespare/xxhash/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// SpanKey is a structure that uniquely identifies a span
// It contains the trace ID and span ID that form a composite key for spans.
type SpanKey struct {
	TraceID pcommon.TraceID
	SpanID  pcommon.SpanID
}

// createSpanKey creates a unique key for a span
func createSpanKey(span ptrace.Span) SpanKey {
	return SpanKey{
		TraceID: span.TraceID(),
		SpanID:  span.SpanID(),
	}
}

// hashSpanKey creates a hash from a span key
func hashSpanKey(key SpanKey) uint64 {
	// Combine trace ID and span ID into a single hash
	h := xxhash.New()
	h.Write(key.TraceID[:])
	h.Write(key.SpanID[:])
	return h.Sum64()
}

// isRootSpan determines if a span is a root span (has no parent)
func isRootSpan(span ptrace.Span) bool {
	return span.ParentSpanID().IsEmpty()
}

// SpanWithResource is a wrapper that keeps a span together with its resource information
// This structure allows preserving the original resource and scope context when handling spans.
type SpanWithResource struct {
	Span     ptrace.Span
	Resource pcommon.Resource
	Scope    pcommon.InstrumentationScope
}

// cloneSpanWithContext creates a deep copy of a span with its associated resource and scope
func cloneSpanWithContext(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) SpanWithResource {
	// Create a new traces object to hold the cloned data
	traces := ptrace.NewTraces()

	// Add a resource span
	rs := traces.ResourceSpans().AppendEmpty()
	resource.CopyTo(rs.Resource())

	// Add a scope span
	ss := rs.ScopeSpans().AppendEmpty()
	scope.CopyTo(ss.Scope())

	// Add the span
	newSpan := ss.Spans().AppendEmpty()
	span.CopyTo(newSpan)

	return SpanWithResource{
		Span:     newSpan,
		Resource: rs.Resource(),
		Scope:    ss.Scope(),
	}
}

// insertSpanIntoTraces inserts a SpanWithResource into a ptrace.Traces object
func insertSpanIntoTraces(traces ptrace.Traces, swr SpanWithResource) {
	// Try to find a matching resource
	resourceSpans := traces.ResourceSpans()
	var matchingRS ptrace.ResourceSpans
	var matchingSS ptrace.ScopeSpans
	foundResource := false
	foundScope := false

	// Look for a matching resource
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)

		// TODO: Implement proper resource matching
		// For now, just append to a new resource span
		if !foundResource {
			matchingRS = rs
			foundResource = true

			// Look for a matching scope within this resource
			scopeSpans := rs.ScopeSpans()
			for j := 0; j < scopeSpans.Len(); j++ {
				ss := scopeSpans.At(j)

				// TODO: Implement proper scope matching
				// For now, just append to a new scope span
				if !foundScope {
					matchingSS = ss
					foundScope = true
					break
				}
			}

			if !foundScope {
				// No matching scope found, create a new one
				matchingSS = rs.ScopeSpans().AppendEmpty()
				swr.Scope.CopyTo(matchingSS.Scope())
				foundScope = true
			}

			break
		}
	}

	if !foundResource {
		// No matching resource found, create a new one
		matchingRS = resourceSpans.AppendEmpty()
		swr.Resource.CopyTo(matchingRS.Resource())

		// Create a new scope span
		matchingSS = matchingRS.ScopeSpans().AppendEmpty()
		swr.Scope.CopyTo(matchingSS.Scope())
	}

	// Add the span to the scope
	newSpan := matchingSS.Spans().AppendEmpty()
	swr.Span.CopyTo(newSpan)
}
