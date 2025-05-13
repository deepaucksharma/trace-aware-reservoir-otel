package adapter

import (
	"github.com/deepaucksharma/reservoir"
)

// OTelAdapter defines the interface for converting between OpenTelemetry 
// data structures and reservoir domain models
type OTelAdapter interface {
	// ConvertSpan converts an OTEL span to a domain model span
	ConvertSpan(span interface{}, resource interface{}, scope interface{}) reservoir.SpanData
	
	// ConvertToOTEL converts domain model spans back to OTEL format
	ConvertToOTEL(spans []reservoir.SpanData) interface{}
	
	// ConvertTraces converts a complete OTEL traces object to domain model spans
	ConvertTraces(traces interface{}) []reservoir.SpanData
}
