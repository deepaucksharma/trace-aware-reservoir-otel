# Alert Configuration for Trace-Aware Reservoir Sampler

This guide provides recommended alert configurations for monitoring the trace-aware reservoir sampler in New Relic.

## Critical Alerts

### 1. Checkpoint Failure Alert

```json
{
  "name": "Reservoir Sampler Checkpoint Errors",
  "type": "STATIC",
  "nrql": {
    "query": "SELECT latest(pte_reservoir_checkpoint_errors_total) FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE",
    "threshold": 0,
    "threshold_duration": 300,
    "threshold_occurrences": "ALL"
  },
  "description": "Detects failures in the reservoir sampler's checkpoint mechanism, which could lead to data loss during pod restarts.",
  "runbook_url": "https://example.com/runbooks/reservoir-checkpoint-failures",
  "violation_time_limit_seconds": 86400
}
```

### 2. Trace Buffer Saturation Alert

```json
{
  "name": "Reservoir Sampler Buffer Saturation",
  "type": "STATIC",
  "nrql": {
    "query": "SELECT latest(pte_reservoir_trace_buffer_size) / 100000 * 100 FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE",
    "threshold": 90,
    "threshold_duration": 600,
    "threshold_occurrences": "ALL"
  },
  "warning": {
    "operator": "ABOVE",
    "threshold": 80,
    "threshold_duration": 600,
    "threshold_occurrences": "ALL"
  },
  "description": "Detects when the trace buffer is approaching capacity, which could lead to premature trace evictions.",
  "runbook_url": "https://example.com/runbooks/reservoir-buffer-saturation",
  "violation_time_limit_seconds": 86400
}
```

### 3. High Eviction Rate Alert

```json
{
  "name": "Reservoir Sampler High Eviction Rate",
  "type": "BASELINE",
  "nrql": {
    "query": "SELECT rate(sum(pte_reservoir_lru_evictions_total), 1 minute) FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE",
    "threshold": 3,
    "threshold_duration": 300,
    "threshold_occurrences": "ALL"
  },
  "baseline_direction": "UPPER_ONLY",
  "description": "Detects abnormally high eviction rates from the trace buffer, indicating buffer size may be too small for the traffic volume.",
  "runbook_url": "https://example.com/runbooks/reservoir-high-evictions",
  "violation_time_limit_seconds": 86400
}
```

### 4. Checkpoint Age Alert

```json
{
  "name": "Reservoir Sampler Checkpoint Age",
  "type": "STATIC",
  "nrql": {
    "query": "SELECT latest(pte_reservoir_checkpoint_age_seconds) FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE",
    "threshold": 20,
    "threshold_duration": 300,
    "threshold_occurrences": "ALL"
  },
  "warning": {
    "operator": "ABOVE",
    "threshold": 15,
    "threshold_duration": 300,
    "threshold_occurrences": "ALL"
  },
  "description": "Alerts when the checkpoint age exceeds twice the expected checkpoint interval (default 10s), indicating checkpointing issues.",
  "runbook_url": "https://example.com/runbooks/reservoir-checkpoint-age",
  "violation_time_limit_seconds": 86400
}
```

## Warning Alerts

### 1. Database Size Growth Alert

```json
{
  "name": "Reservoir Sampler DB Size Growth",
  "type": "GROWTH",
  "nrql": {
    "query": "SELECT latest(pte_reservoir_db_size_bytes) / 1048576 FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE",
    "threshold": 50,
    "threshold_duration": 3600,
    "threshold_occurrences": "ALL"
  },
  "warning": {
    "operator": "ABOVE",
    "threshold": 25,
    "threshold_duration": 3600,
    "threshold_occurrences": "ALL"
  },
  "description": "Monitors unusually fast growth of the checkpoint database size, which may indicate insufficient compaction.",
  "runbook_url": "https://example.com/runbooks/reservoir-db-growth",
  "violation_time_limit_seconds": 86400
}
```

### 2. Memory Usage Alert

```json
{
  "name": "Reservoir Sampler Memory Usage",
  "type": "STATIC",
  "nrql": {
    "query": "SELECT latest(container_memory_usage_bytes) / 1048576 FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE",
    "threshold": 3500,
    "threshold_duration": 300,
    "threshold_occurrences": "ALL"
  },
  "warning": {
    "operator": "ABOVE",
    "threshold": 3000,
    "threshold_duration": 300,
    "threshold_occurrences": "ALL"
  },
  "description": "Alerts when memory usage approaches container limits (4Gi), which could lead to OOM kills.",
  "runbook_url": "https://example.com/runbooks/reservoir-memory-usage",
  "violation_time_limit_seconds": 86400
}
```

### 3. Low Sampling Rate Alert

```json
{
  "name": "Reservoir Sampler Rate Too Low",
  "type": "STATIC",
  "nrql": {
    "query": "SELECT latest(pte_reservoir_traces_in_reservoir_count) / latest(pte_reservoir_window_count) * 100 FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "warning": {
    "operator": "BELOW",
    "threshold": 1,
    "threshold_duration": 1800,
    "threshold_occurrences": "ALL"
  },
  "description": "Detects when the effective sampling rate falls below 1%, which may indicate a need for reservoir size adjustment.",
  "runbook_url": "https://example.com/runbooks/reservoir-low-sampling-rate",
  "violation_time_limit_seconds": 86400
}
```

## Anomaly Detection Alerts

### 1. Abnormal Span Processing

```json
{
  "name": "Reservoir Sampler Abnormal Processing",
  "type": "BASELINE",
  "nrql": {
    "query": "SELECT rate(sum(otelcol_processor_accepted_spans_total), 1 minute) FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' AND processor = 'reservoir_sampler' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE_OR_BELOW",
    "threshold": 3,
    "threshold_duration": 600,
    "threshold_occurrences": "ALL"
  },
  "baseline_direction": "BOTH",
  "description": "Detects unusual changes in span processing rates, which may indicate upstream issues or collector problems.",
  "runbook_url": "https://example.com/runbooks/reservoir-abnormal-processing",
  "violation_time_limit_seconds": 86400
}
```

### 2. Abnormal Reservoir Size Fluctuation

```json
{
  "name": "Reservoir Sampler Size Fluctuation",
  "type": "BASELINE",
  "nrql": {
    "query": "SELECT latest(pte_reservoir_traces_in_reservoir_count) FROM Metric WHERE instrumentation.provider = 'opentelemetry' AND service.name = 'nrdot-collector' FACET k8s.pod.name"
  },
  "critical": {
    "operator": "ABOVE_OR_BELOW",
    "threshold": 2,
    "threshold_duration": 900,
    "threshold_occurrences": "AT_LEAST_ONCE"
  },
  "baseline_direction": "BOTH",
  "description": "Detects unusual fluctuations in reservoir size, which may indicate sampling inconsistencies.",
  "runbook_url": "https://example.com/runbooks/reservoir-size-fluctuation",
  "violation_time_limit_seconds": 86400
}
```

## Implementation Steps

1. Create a New Relic Alert Policy named "Trace-Aware Reservoir Sampler"

2. Add the above NRQL Alert Conditions to the policy

3. Configure notification channels:
   - Critical alerts: PagerDuty/Slack immediate notification
   - Warning alerts: Email/Slack with lower urgency
   - Anomaly detection: Dashboard notification

4. Test alerts by:
   - Temporarily reducing buffer sizes
   - Blocking checkpoint file writes
   - Generating high trace volumes

5. Adjust thresholds based on your environment and traffic patterns

## Runbook Template

Create runbooks for each alert condition following this structure:

1. **Alert Description**: What triggered this alert
2. **Potential Causes**: Common reasons for this alert
3. **Diagnostic Steps**:
   - Commands to gather relevant metrics
   - Log queries to check
   - Resource inspection procedures
4. **Resolution Steps**:
   - Configuration adjustments
   - Scaling actions
   - Emergency measures
5. **Prevention**: How to avoid this issue in the future