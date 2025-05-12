package reservoirsampler

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Magic bytes for our binary format
const (
	SerializationMagic   = "SPAN"
	SerializationVersion = byte(1)
)

// Binary serialization format:
// - Magic (4 bytes): "SPAN"
// - Version (1 byte): 1
// - Flags (3 bytes): [hasSpanSection, hasResourceSection, hasScopeSection]
// - Section Sizes (12 bytes): [spanSectionSize, resourceSectionSize, scopeSectionSize]
// - Span Section:
//   - TraceID (16 bytes)
//   - SpanID (8 bytes)
//   - ParentSpanID (8 bytes)
//   - Name Length (4 bytes)
//   - Name (variable)
//   - Start Timestamp (8 bytes)
//   - End Timestamp (8 bytes)
// - Resource Section:
//   - Service Name Key Length (4 bytes)
//   - Service Name Key (variable)
//   - Service Name Value Length (4 bytes)
//   - Service Name Value (variable)
// - Scope Section:
//   - Scope Name Length (4 bytes)
//   - Scope Name (variable)

// serializeSpanWithResource serializes a SpanWithResource to bytes
// This implementation completely avoids using Protocol Buffers to prevent stack overflow
func serializeSpanWithResource(swr SpanWithResource) ([]byte, error) {
	// Calculate required buffer size for serialized data
	// Custom binary format: Magic (4) + Version (1) + Sections (3) + SectionLengths (12) + Data
	bufSize := 20 // header bytes

	// Get span data
	traceID := swr.Span.TraceID()
	spanID := swr.Span.SpanID()
	parentSpanID := swr.Span.ParentSpanID()
	name := swr.Span.Name()
	startTime := swr.Span.StartTimestamp()
	endTime := swr.Span.EndTimestamp()

	// Calculate span section size
	spanSectionSize := 16 + 8 + 8 + 4 + len(name) + 16 // IDs + name length + name + timestamps
	bufSize += spanSectionSize

	// Calculate resource section size - only include service.name
	resourceSectionSize := 0
	var serviceName string
	swr.Resource.Attributes().Range(func(k string, v pcommon.Value) bool {
		if k == "service.name" {
			serviceName = v.AsString()
			resourceSectionSize = 4 + len(k) + 4 + len(serviceName)
			return false // Exit after getting service name
		}
		return true
	})
	bufSize += resourceSectionSize

	// Calculate scope section size - just name
	scopeSectionSize := 0
	scopeName := swr.Scope.Name()
	if scopeName != "" {
		scopeSectionSize = 4 + len(scopeName)
	}
	bufSize += scopeSectionSize

	// Create buffer with exact size
	buf := bytes.NewBuffer(make([]byte, 0, bufSize))

	// Write header
	if _, err := buf.WriteString(SerializationMagic); err != nil {
		return nil, fmt.Errorf("failed to write magic bytes: %w", err)
	}
	if err := buf.WriteByte(SerializationVersion); err != nil {
		return nil, fmt.Errorf("failed to write version: %w", err)
	}

	// Write section flags (1 byte each)
	// 1 = section present, 0 = section absent
	if err := buf.WriteByte(1); err != nil { // Span section (always present)
		return nil, fmt.Errorf("failed to write span section flag: %w", err)
	}
	if resourceSectionSize > 0 {
		if err := buf.WriteByte(1); err != nil {
			return nil, fmt.Errorf("failed to write resource section flag: %w", err)
		}
	} else {
		if err := buf.WriteByte(0); err != nil {
			return nil, fmt.Errorf("failed to write resource section flag: %w", err)
		}
	}
	if scopeSectionSize > 0 {
		if err := buf.WriteByte(1); err != nil {
			return nil, fmt.Errorf("failed to write scope section flag: %w", err)
		}
	} else {
		if err := buf.WriteByte(0); err != nil {
			return nil, fmt.Errorf("failed to write scope section flag: %w", err)
		}
	}

	// Write section sizes
	if err := binary.Write(buf, binary.BigEndian, uint32(spanSectionSize)); err != nil {
		return nil, fmt.Errorf("failed to write span section size: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(resourceSectionSize)); err != nil {
		return nil, fmt.Errorf("failed to write resource section size: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(scopeSectionSize)); err != nil {
		return nil, fmt.Errorf("failed to write scope section size: %w", err)
	}

	// Write span data
	if _, err := buf.Write(traceID[:]); err != nil {
		return nil, fmt.Errorf("failed to write trace ID: %w", err)
	}
	if _, err := buf.Write(spanID[:]); err != nil {
		return nil, fmt.Errorf("failed to write span ID: %w", err)
	}
	if _, err := buf.Write(parentSpanID[:]); err != nil {
		return nil, fmt.Errorf("failed to write parent span ID: %w", err)
	}

	// Write span name with length prefix
	if err := binary.Write(buf, binary.BigEndian, uint32(len(name))); err != nil {
		return nil, fmt.Errorf("failed to write name length: %w", err)
	}
	if _, err := buf.WriteString(name); err != nil {
		return nil, fmt.Errorf("failed to write name: %w", err)
	}

	// Write timestamps
	if err := binary.Write(buf, binary.BigEndian, uint64(startTime)); err != nil {
		return nil, fmt.Errorf("failed to write start timestamp: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, uint64(endTime)); err != nil {
		return nil, fmt.Errorf("failed to write end timestamp: %w", err)
	}

	// Write resource data if present
	if resourceSectionSize > 0 {
		// Write service name key
		key := "service.name"
		if err := binary.Write(buf, binary.BigEndian, uint32(len(key))); err != nil {
			return nil, fmt.Errorf("failed to write service name key length: %w", err)
		}
		if _, err := buf.WriteString(key); err != nil {
			return nil, fmt.Errorf("failed to write service name key: %w", err)
		}

		// Write service name value
		if err := binary.Write(buf, binary.BigEndian, uint32(len(serviceName))); err != nil {
			return nil, fmt.Errorf("failed to write service name value length: %w", err)
		}
		if _, err := buf.WriteString(serviceName); err != nil {
			return nil, fmt.Errorf("failed to write service name value: %w", err)
		}
	}

	// Write scope data if present
	if scopeSectionSize > 0 {
		if err := binary.Write(buf, binary.BigEndian, uint32(len(scopeName))); err != nil {
			return nil, fmt.Errorf("failed to write scope name length: %w", err)
		}
		if _, err := buf.WriteString(scopeName); err != nil {
			return nil, fmt.Errorf("failed to write scope name: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// deserializeSpanWithResource deserializes bytes to a SpanWithResource
// This implementation uses direct binary parsing to avoid Protocol Buffers
func deserializeSpanWithResource(data []byte) (SpanWithResource, error) {
	if len(data) < 20 {
		return SpanWithResource{}, fmt.Errorf("invalid span data: too short (expected at least 20 bytes, got %d)", len(data))
	}

	// Create a minimal traces object
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	// Use a buffer reader for easier parsing
	buf := bytes.NewReader(data)

	// Read and verify magic bytes
	magic := make([]byte, 4)
	if _, err := buf.Read(magic); err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read magic bytes: %w", err)
	}

	if string(magic) != SerializationMagic {
		return SpanWithResource{}, fmt.Errorf("invalid magic bytes: expected %s, got %s", SerializationMagic, string(magic))
	}

	// Read version
	version, err := buf.ReadByte()
	if err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read version: %w", err)
	}

	if version != SerializationVersion {
		return SpanWithResource{}, fmt.Errorf("unsupported version: %d", version)
	}

	// Read section flags
	hasSpanSection, err := buf.ReadByte()
	if err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read span section flag: %w", err)
	}

	hasResourceSection, err := buf.ReadByte()
	if err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read resource section flag: %w", err)
	}

	hasScopeSection, err := buf.ReadByte()
	if err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read scope section flag: %w", err)
	}

	// Read section sizes
	var spanSectionSize, resourceSectionSize, scopeSectionSize uint32
	if err := binary.Read(buf, binary.BigEndian, &spanSectionSize); err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read span section size: %w", err)
	}

	if err := binary.Read(buf, binary.BigEndian, &resourceSectionSize); err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read resource section size: %w", err)
	}

	if err := binary.Read(buf, binary.BigEndian, &scopeSectionSize); err != nil {
		return SpanWithResource{}, fmt.Errorf("failed to read scope section size: %w", err)
	}

	// Read span data
	if hasSpanSection == 1 && spanSectionSize >= 32 {
		// Read trace ID
		traceID := pcommon.TraceID{}
		if _, err := buf.Read(traceID[:]); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read trace ID: %w", err)
		}
		span.SetTraceID(traceID)

		// Read span ID
		spanID := pcommon.SpanID{}
		if _, err := buf.Read(spanID[:]); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read span ID: %w", err)
		}
		span.SetSpanID(spanID)

		// Read parent span ID
		parentSpanID := pcommon.SpanID{}
		if _, err := buf.Read(parentSpanID[:]); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read parent span ID: %w", err)
		}
		span.SetParentSpanID(parentSpanID)

		// Read name if present
		if spanSectionSize > 32 {
			var nameLen uint32
			if err := binary.Read(buf, binary.BigEndian, &nameLen); err != nil {
				return SpanWithResource{}, fmt.Errorf("failed to read name length: %w", err)
			}

			if nameLen > 0 {
				nameBytes := make([]byte, nameLen)
				if _, err := buf.Read(nameBytes); err != nil {
					return SpanWithResource{}, fmt.Errorf("failed to read name: %w", err)
				}
				span.SetName(string(nameBytes))
			}

			// Read timestamps if present
			if buf.Len() >= 16 {
				var startTime, endTime uint64
				if err := binary.Read(buf, binary.BigEndian, &startTime); err != nil {
					return SpanWithResource{}, fmt.Errorf("failed to read start timestamp: %w", err)
				}

				if err := binary.Read(buf, binary.BigEndian, &endTime); err != nil {
					return SpanWithResource{}, fmt.Errorf("failed to read end timestamp: %w", err)
				}

				span.SetStartTimestamp(pcommon.Timestamp(startTime))
				span.SetEndTimestamp(pcommon.Timestamp(endTime))
			}
		}
	}

	// Read resource data if present
	if hasResourceSection == 1 && resourceSectionSize > 0 {
		// Read service name key
		var keyLen uint32
		if err := binary.Read(buf, binary.BigEndian, &keyLen); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read service name key length: %w", err)
		}

		keyBytes := make([]byte, keyLen)
		if _, err := buf.Read(keyBytes); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read service name key: %w", err)
		}
		key := string(keyBytes)

		// Read service name value
		var valueLen uint32
		if err := binary.Read(buf, binary.BigEndian, &valueLen); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read service name value length: %w", err)
		}

		valueBytes := make([]byte, valueLen)
		if _, err := buf.Read(valueBytes); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read service name value: %w", err)
		}
		value := string(valueBytes)

		// Set resource attribute
		if key == "service.name" {
			rs.Resource().Attributes().PutStr(key, value)
		}
	}

	// Read scope data if present
	if hasScopeSection == 1 && scopeSectionSize > 0 {
		var nameLen uint32
		if err := binary.Read(buf, binary.BigEndian, &nameLen); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read scope name length: %w", err)
		}

		nameBytes := make([]byte, nameLen)
		if _, err := buf.Read(nameBytes); err != nil {
			return SpanWithResource{}, fmt.Errorf("failed to read scope name: %w", err)
		}
		ss.Scope().SetName(string(nameBytes))
	}

	return SpanWithResource{
		Span:     span,
		Resource: rs.Resource(),
		Scope:    ss.Scope(),
	}, nil
}
