package reservoirsampler

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// TestSerializationRoundtrip tests the serialization and deserialization of spans
func TestSerializationRoundtrip(t *testing.T) {
	// Create test span data
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	res := rs.Resource()
	res.Attributes().PutStr("service.name", "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	scope := ss.Scope()
	scope.SetName("test-scope")

	span := ss.Spans().AppendEmpty()
	span.SetName("test-span")
	span.SetTraceID(createTestTraceID(1))
	span.SetSpanID(createTestSpanID(1))
	span.SetStartTimestamp(123456789)
	span.SetEndTimestamp(987654321)

	// Create SpanWithResource
	spanWithRes := SpanWithResource{
		Span:     span,
		Resource: res,
		Scope:    scope,
	}

	// Serialize using our custom binary format
	serialized, err := serializeSpanWithResource(spanWithRes)
	require.NoError(t, err)

	// Deserialize
	deserialized, err := deserializeSpanWithResource(serialized)
	require.NoError(t, err)

	// Verify deserialized data
	assert.Equal(t, span.TraceID(), deserialized.Span.TraceID())
	assert.Equal(t, span.SpanID(), deserialized.Span.SpanID())
	assert.Equal(t, span.Name(), deserialized.Span.Name())
	assert.Equal(t, span.StartTimestamp(), deserialized.Span.StartTimestamp())
	assert.Equal(t, span.EndTimestamp(), deserialized.Span.EndTimestamp())

	// Verify resource and scope data
	serviceName, found := deserialized.Resource.Attributes().Get("service.name")
	assert.True(t, found)
	assert.Equal(t, "test-service", serviceName.AsString())
	assert.Equal(t, "test-scope", deserialized.Scope.Name())
}

// createTestTraceID creates a test trace ID from an index
func createTestTraceID(id int) pcommon.TraceID {
	var traceID pcommon.TraceID
	binary.BigEndian.PutUint64(traceID[:8], uint64(id))
	binary.BigEndian.PutUint64(traceID[8:], uint64(id))
	return traceID
}

// createTestSpanID creates a test span ID from an index
func createTestSpanID(id int) pcommon.SpanID {
	var spanID pcommon.SpanID
	binary.BigEndian.PutUint64(spanID[:], uint64(id))
	return spanID
}

// TestBinaryFormat tests the binary format structure
func TestBinaryFormat(t *testing.T) {
	// Create minimal test data
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	span.SetTraceID(createTestTraceID(42))
	span.SetSpanID(createTestSpanID(43))
	span.SetName("test-binary-format")

	spanWithRes := SpanWithResource{
		Span:     span,
		Resource: rs.Resource(),
		Scope:    ss.Scope(),
	}

	// Serialize
	data, err := serializeSpanWithResource(spanWithRes)
	require.NoError(t, err)

	// Verify basic structure manually
	buf := bytes.NewReader(data)

	// Check magic
	magic := make([]byte, 4)
	n, err := buf.Read(magic)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	assert.Equal(t, "SPAN", string(magic))

	// Check version
	version, err := buf.ReadByte()
	require.NoError(t, err)
	assert.Equal(t, byte(1), version)

	// Skip 3 flag bytes and 12 section size bytes
	skipBytes := make([]byte, 15)
	n, err = buf.Read(skipBytes)
	require.NoError(t, err)
	require.Equal(t, 15, n)

	// Read trace ID and verify
	traceID := make([]byte, 16)
	n, err = buf.Read(traceID)
	require.NoError(t, err)
	require.Equal(t, 16, n)

	expectedTraceID := createTestTraceID(42)
	assert.Equal(t, expectedTraceID[:], traceID)
}
