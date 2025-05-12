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

	// These writes to a hash never fail in xxhash implementation
	// but we should follow best practices and check the error in case
	// the implementation changes in the future
	if _, err := h.Write(key.TraceID[:]); err != nil {
		// In the extremely unlikely case of an error, just return a simple hash of the first bytes
		result := uint64(key.TraceID[0]) << 56
		result |= uint64(key.SpanID[0])
		return result
	}
	if _, err := h.Write(key.SpanID[:]); err != nil {
		// If we can't write the span ID, use what we have so far
		return h.Sum64()
	}

	return h.Sum64()
}

// isRootSpan determines if a span is a root span (has no parent)
// This function is currently not used but is kept for potential future use
// in trace-aware sampling where root spans may need special handling.
//nolint:unused // Kept for future use
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
			scopeFound := false

			// Try to find a matching scope if there are scopes
			if scopeSpans.Len() > 0 {
				// Look for scope with matching name
				targetName := swr.Scope.Name()
				for i := 0; i < scopeSpans.Len(); i++ {
					ss := scopeSpans.At(i)
					if ss.Scope().Name() == targetName {
						matchingSS = ss
						scopeFound = true
						break
					}
				}

				// If no match found but scopes exist, use the first one
				if !scopeFound {
					matchingSS = scopeSpans.At(0)
					scopeFound = true
				}
			}

			if !scopeFound {
				// No matching scope found, create a new one
				matchingSS = rs.ScopeSpans().AppendEmpty()
				swr.Scope.CopyTo(matchingSS.Scope())
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
