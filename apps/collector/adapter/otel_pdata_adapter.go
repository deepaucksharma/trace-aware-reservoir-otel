package adapter

import (
	"github.com/deepaucksharma/reservoir"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// OTelPDataAdapter converts between OpenTelemetry pdata types and reservoir domain models
type OTelPDataAdapter struct{}

// NewOTelPDataAdapter creates a new OpenTelemetry pdata adapter
func NewOTelPDataAdapter() *OTelPDataAdapter {
	return &OTelPDataAdapter{}
}

// ConvertSpan converts an OTEL span to a domain model span
func (a *OTelPDataAdapter) ConvertSpan(
	span ptrace.Span,
	resource pcommon.Resource,
	scope ptrace.InstrumentationScope,
) reservoir.SpanData {
	// Extract span data
	traceID := span.TraceID().HexString()
	spanID := span.SpanID().HexString()
	
	var parentSpanID string
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().HexString()
	}
	
	// Extract attributes
	attrs := make(map[string]interface{})
	span.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeString:
			attrs[k] = v.StringVal()
		case pcommon.ValueTypeInt:
			attrs[k] = v.IntVal()
		case pcommon.ValueTypeDouble:
			attrs[k] = v.DoubleVal()
		case pcommon.ValueTypeBool:
			attrs[k] = v.BoolVal()
		// Handle other types including slices and maps
		}
		return true
	})
	
	// Convert events
	events := make([]reservoir.Event, span.Events().Len())
	for i := 0; i < span.Events().Len(); i++ {
		srcEvent := span.Events().At(i)
		eventAttrs := make(map[string]interface{})
		
		srcEvent.Attributes().Range(func(k string, v pcommon.Value) bool {
			switch v.Type() {
			case pcommon.ValueTypeString:
				eventAttrs[k] = v.StringVal()
			case pcommon.ValueTypeInt:
				eventAttrs[k] = v.IntVal()
			case pcommon.ValueTypeDouble:
				eventAttrs[k] = v.DoubleVal()
			case pcommon.ValueTypeBool:
				eventAttrs[k] = v.BoolVal()
			// Handle other types
			}
			return true
		})
		
		event := reservoir.Event{
			Name:       srcEvent.Name(),
			Timestamp:  srcEvent.Timestamp().AsTime().UnixNano(),
			Attributes: eventAttrs,
		}
		events[i] = event
	}
	
	// Convert links
	links := make([]reservoir.Link, span.Links().Len())
	for i := 0; i < span.Links().Len(); i++ {
		srcLink := span.Links().At(i)
		linkAttrs := make(map[string]interface{})
		
		srcLink.Attributes().Range(func(k string, v pcommon.Value) bool {
			switch v.Type() {
			case pcommon.ValueTypeString:
				linkAttrs[k] = v.StringVal()
			case pcommon.ValueTypeInt:
				linkAttrs[k] = v.IntVal()
			case pcommon.ValueTypeDouble:
				linkAttrs[k] = v.DoubleVal()
			case pcommon.ValueTypeBool:
				linkAttrs[k] = v.BoolVal()
			// Handle other types
			}
			return true
		})
		
		link := reservoir.Link{
			TraceID:    srcLink.TraceID().HexString(),
			SpanID:     srcLink.SpanID().HexString(),
			Attributes: linkAttrs,
		}
		links[i] = link
	}
	
	// Create resource info
	resourceAttrs := make(map[string]interface{})
	resource.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeString:
			resourceAttrs[k] = v.StringVal()
		case pcommon.ValueTypeInt:
			resourceAttrs[k] = v.IntVal()
		case pcommon.ValueTypeDouble:
			resourceAttrs[k] = v.DoubleVal()
		case pcommon.ValueTypeBool:
			resourceAttrs[k] = v.BoolVal()
		// Handle other types
		}
		return true
	})
	
	resourceInfo := reservoir.ResourceInfo{
		Attributes: resourceAttrs,
	}
	
	// Create scope info
	scopeInfo := reservoir.ScopeInfo{
		Name:    scope.Name(),
		Version: scope.Version(),
	}
	
	// Create the span data object
	return reservoir.SpanData{
		ID:           spanID,
		TraceID:      traceID,
		ParentID:     parentSpanID,
		Name:         span.Name(),
		StartTime:    span.StartTimestamp().AsTime().UnixNano(),
		EndTime:      span.EndTimestamp().AsTime().UnixNano(),
		Attributes:   attrs,
		Events:       events,
		Links:        links,
		StatusCode:   int(span.Status().Code()),
		StatusMsg:    span.Status().Message(),
		ResourceInfo: resourceInfo,
		ScopeInfo:    scopeInfo,
	}
}

// ConvertToOTEL converts domain model spans back to OTEL format
func (a *OTelPDataAdapter) ConvertToOTEL(spans []reservoir.SpanData) ptrace.Traces {
	traces := ptrace.NewTraces()
	
	// Group spans by resource to correctly structure the OTEL data
	resourceMap := make(map[string]map[string][]reservoir.SpanData)
	
	for _, span := range spans {
		// Create a key for the resource based on its attributes
		resourceKey := ""
		for k, v := range span.ResourceInfo.Attributes {
			resourceKey += k + ":" + fmt.Sprintf("%v", v) + ","
		}
		
		// Create a key for the scope
		scopeKey := span.ScopeInfo.Name + ":" + span.ScopeInfo.Version
		
		// Initialize maps if needed
		if _, ok := resourceMap[resourceKey]; !ok {
			resourceMap[resourceKey] = make(map[string][]reservoir.SpanData)
		}
		
		// Add the span to the appropriate resource and scope
		resourceMap[resourceKey][scopeKey] = append(resourceMap[resourceKey][scopeKey], span)
	}
	
	// Create OTEL structure
	for resourceKey, scopeMap := range resourceMap {
		// Create a new ResourceSpans
		rs := traces.ResourceSpans().AppendEmpty()
		resource := rs.Resource()
		
		// Extract resource attrs from the key and set them
		// This is a simplified approach; in practice, you'd deserialize the resourceKey properly
		
		// For each scope in this resource
		for scopeKey, scopeSpans := range scopeMap {
			// Create a new ScopeSpans
			ss := rs.ScopeSpans().AppendEmpty()
			scope := ss.Scope()
			
			// Set scope info
			// Parse scopeKey to get name and version
			scopeParts := strings.Split(scopeKey, ":")
			if len(scopeParts) == 2 {
				scope.SetName(scopeParts[0])
				scope.SetVersion(scopeParts[1])
			}
			
			// Add all spans for this scope
			for _, span := range scopeSpans {
				otelSpan := ss.Spans().AppendEmpty()
				
				// Set span data
				if traceID, err := hex.DecodeString(span.TraceID); err == nil && len(traceID) == 16 {
					otelSpan.SetTraceID(pcommon.TraceID(traceID))
				}
				
				if spanID, err := hex.DecodeString(span.ID); err == nil && len(spanID) == 8 {
					otelSpan.SetSpanID(pcommon.SpanID(spanID))
				}
				
				if span.ParentID != "" {
					if parentID, err := hex.DecodeString(span.ParentID); err == nil && len(parentID) == 8 {
						otelSpan.SetParentSpanID(pcommon.SpanID(parentID))
					}
				}
				
				otelSpan.SetName(span.Name)
				otelSpan.SetStartTimestamp(pcommon.Timestamp(span.StartTime))
				otelSpan.SetEndTimestamp(pcommon.Timestamp(span.EndTime))
				otelSpan.Status().SetCode(ptrace.StatusCode(span.StatusCode))
				otelSpan.Status().SetMessage(span.StatusMsg)
				
				// Set attributes
				for k, v := range span.Attributes {
					setAttributeValue(otelSpan.Attributes(), k, v)
				}
				
				// Set events
				for _, event := range span.Events {
					otelEvent := otelSpan.Events().AppendEmpty()
					otelEvent.SetName(event.Name)
					otelEvent.SetTimestamp(pcommon.Timestamp(event.Timestamp))
					
					for k, v := range event.Attributes {
						setAttributeValue(otelEvent.Attributes(), k, v)
					}
				}
				
				// Set links
				for _, link := range span.Links {
					otelLink := otelSpan.Links().AppendEmpty()
					
					if traceID, err := hex.DecodeString(link.TraceID); err == nil && len(traceID) == 16 {
						otelLink.SetTraceID(pcommon.TraceID(traceID))
					}
					
					if spanID, err := hex.DecodeString(link.SpanID); err == nil && len(spanID) == 8 {
						otelLink.SetSpanID(pcommon.SpanID(spanID))
					}
					
					for k, v := range link.Attributes {
						setAttributeValue(otelLink.Attributes(), k, v)
					}
				}
			}
		}
	}
	
	return traces
}

// ConvertTraces converts a complete OTEL traces object to domain model spans
func (a *OTelPDataAdapter) ConvertTraces(traces ptrace.Traces) []reservoir.SpanData {
	var result []reservoir.SpanData
	
	// Process each resource spans
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		resource := rs.Resource()
		
		// Process each instrumentation scope spans
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			ils := ilss.At(j)
			scope := ils.Scope()
			
			// Process each span
			spans := ils.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				result = append(result, a.ConvertSpan(span, resource, scope))
			}
		}
	}
	
	return result
}

// Helper function to set attribute values in OTEL maps
func setAttributeValue(attrMap pcommon.Map, key string, value interface{}) {
	switch v := value.(type) {
	case string:
		attrMap.PutStr(key, v)
	case int, int8, int16, int32, int64:
		attrMap.PutInt(key, reflect.ValueOf(v).Int())
	case uint, uint8, uint16, uint32, uint64:
		attrMap.PutInt(key, int64(reflect.ValueOf(v).Uint()))
	case float32, float64:
		attrMap.PutDouble(key, reflect.ValueOf(v).Float())
	case bool:
		attrMap.PutBool(key, v)
	default:
		// Attempt to convert to string
		attrMap.PutStr(key, fmt.Sprintf("%v", v))
	}
}