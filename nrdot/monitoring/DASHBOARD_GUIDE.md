# New Relic Dashboard Guide for Trace-Aware Reservoir Sampler

This guide outlines the recommended widgets and charts to include in your New Relic dashboard for monitoring the trace-aware reservoir sampler integration with NR-DOT.

## Essential Widgets

### 1. Reservoir Stats

#### Reservoir Size Widget
- **Metric**: `pte_reservoir_traces_in_reservoir_count`
- **Chart Type**: Line chart
- **Purpose**: Track the number of traces currently stored in the reservoir
- **Threshold**: Alert if consistently near the configured `size_k` value (reservoir full)

#### Window Count Widget
- **Metric**: `pte_reservoir_window_count`
- **Chart Type**: Line chart
- **Purpose**: Monitor the total number of spans seen in the current sampling window
- **Additional Metric**: Calculate sampling rate `pte_reservoir_traces_in_reservoir_count / pte_reservoir_window_count`

### 2. Checkpoint Monitoring

#### Checkpoint Age Widget
- **Metric**: `pte_reservoir_checkpoint_age_seconds`
- **Chart Type**: Line chart with threshold
- **Purpose**: Monitor time since last successful checkpoint
- **Threshold**: Alert if exceeds 2x the configured checkpoint interval

#### Checkpoint Database Size Widget
- **Metric**: `pte_reservoir_db_size_bytes`
- **Chart Type**: Line chart with threshold
- **Purpose**: Track the size of the BoltDB checkpoint file
- **Threshold**: Alert if approaching PV size limit

#### Checkpoint Errors Widget
- **Metric**: `pte_reservoir_checkpoint_errors_total`
- **Chart Type**: Line chart
- **Purpose**: Monitor checkpoint failures
- **Threshold**: Alert on any non-zero value

### 3. Trace Buffer Monitoring

#### Trace Buffer Size Widget
- **Metric**: `pte_reservoir_trace_buffer_size`
- **Chart Type**: Line chart
- **Purpose**: Monitor traces currently held in the buffer
- **Threshold**: Alert if consistently above 80% of `trace_buffer_max_size`

#### Span Buffer Count Widget
- **Metric**: `pte_reservoir_trace_buffer_span_count`
- **Chart Type**: Line chart
- **Purpose**: Track total spans across all traces in buffer
- **Analysis**: Use to inform right-sizing of buffer capacity

#### LRU Eviction Rate Widget
- **Metric**: `pte_reservoir_lru_evictions_total`
- **Chart Type**: Rate line chart (change over time)
- **Purpose**: Monitor trace buffer evictions due to capacity limits
- **Threshold**: Alert if consistently above 0, indicates buffer too small

### 4. Performance Metrics

#### Sampling Efficiency Widget
- **Metrics**: 
  - `pte_reservoir_sampled_spans_total` (rate)
  - `otelcol_receiver_accepted_spans_total` (rate)
  - Calculation: `pte_reservoir_sampled_spans_total / otelcol_receiver_accepted_spans_total`
- **Chart Type**: Line chart
- **Purpose**: Track effective sampling rate
- **Target**: Should align with target sampling rate (k/n)

#### Processing Rate Widget
- **Metrics**:
  - `otelcol_receiver_accepted_spans_total` (rate)
  - `otelcol_exporter_sent_spans_total` (rate)
- **Chart Type**: Line chart
- **Purpose**: Monitor input vs output span rates
- **Analysis**: Shows processing throughput and sampling reduction

### 5. Resource Usage

#### Memory Usage Widget
- **Metric**: `container_memory_usage_bytes{pod=~"nr-otel.*"}`
- **Chart Type**: Line chart with threshold
- **Purpose**: Monitor memory consumption
- **Threshold**: Alert if approaching container limit

#### CPU Usage Widget
- **Metric**: `container_cpu_usage_seconds_total{pod=~"nr-otel.*"}`
- **Chart Type**: Rate line chart
- **Purpose**: Monitor CPU utilization
- **Threshold**: Alert if consistently above 80% of limit

## Dashboard Layout

Organize the dashboard into sections:
1. **Overview** - Key metrics at a glance
   - Sampling rate
   - Reservoir size
   - Trace buffer utilization
   - Recent checkpoint status

2. **Reservoir Performance** - Detailed reservoir metrics
   - Window counts
   - Sampling efficiency
   - Span processing rates

3. **Storage & Durability** - Checkpoint and storage metrics
   - Checkpoint age
   - Database size
   - Compaction events

4. **Trace Buffer** - Buffer performance metrics
   - Buffer sizes
   - Eviction rates
   - Trace completion metrics

5. **Resource Usage** - Container resource metrics
   - Memory usage
   - CPU utilization
   - Disk I/O for checkpoint operations

## Alert Conditions

Set up alerts for these critical conditions:

1. **Checkpoint Failures**
   - Trigger on `pte_reservoir_checkpoint_errors_total > 0`
   - Indicates potential persistence issues

2. **High Eviction Rate**
   - Trigger on sustained eviction rate
   - Indicates trace buffer is undersized

3. **Full Reservoir**
   - Trigger when reservoir stays at capacity
   - May indicate need for larger reservoir or shorter window

4. **Resource Constraints**
   - Trigger on memory/CPU approaching limits
   - May impact sampling performance

5. **Database Size Growth**
   - Trigger on checkpoint DB growing too large
   - May indicate need for more frequent compaction

## Custom Queries

For advanced monitoring, consider these NRQL queries:

```sql
-- Calculate effective sampling rate
SELECT rate(filter(sum(pte_reservoir_sampled_spans_total), WHERE collector='nrdot'), 1 minute) / 
       rate(filter(sum(otelcol_receiver_accepted_spans_total), WHERE collector='nrdot'), 1 minute) * 100 
FROM Metric FACET collector TIMESERIES

-- Detect trace buffer saturation
SELECT average(pte_reservoir_trace_buffer_size) / max(configValue) * 100 as 'Buffer Utilization %' 
FROM Metric, NrIntegrationConfiguration 
WHERE configName = 'trace_buffer_max_size' 
FACET collector TIMESERIES

-- Monitor checkpoint health
SELECT 
  latest(pte_reservoir_checkpoint_age_seconds) as 'Age (s)', 
  latest(pte_reservoir_db_size_bytes) / 1024 / 1024 as 'Size (MB)', 
  sum(pte_reservoir_checkpoint_errors_total) as 'Errors' 
FROM Metric FACET collector
```