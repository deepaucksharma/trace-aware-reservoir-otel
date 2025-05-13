package reservoir

import (
	"time"
)

// SpanData represents a single span in a trace-agnostic way
type SpanData struct {
	ID           string
	TraceID      string
	ParentID     string
	Name         string
	StartTime    int64
	EndTime      int64
	Attributes   map[string]interface{}
	Events       []Event
	Links        []Link
	StatusCode   int
	StatusMsg    string
	ResourceInfo ResourceInfo
	ScopeInfo    ScopeInfo
}

// Event represents a timed event within a span
type Event struct {
	Name       string
	Timestamp  int64
	Attributes map[string]interface{}
}

// Link represents a relationship to another span
type Link struct {
	TraceID    string
	SpanID     string
	Attributes map[string]interface{}
}

// ResourceInfo represents metadata about the resource that produced the span
type ResourceInfo struct {
	Attributes map[string]interface{}
}

// ScopeInfo represents metadata about the instrumentation scope
type ScopeInfo struct {
	Name    string
	Version string
}

// SpanWithResource is a wrapper that includes span data with its associated resource
// This is used for serialization and checkpointing
type SpanWithResource struct {
	Span     SpanData
	Resource ResourceInfo
	Scope    ScopeInfo
}
