# Troubleshooting Guide for Trace-Aware Reservoir Sampler

This guide provides detailed steps for diagnosing and resolving common issues with the trace-aware reservoir sampler when integrated with NR-DOT.

## Common Issues

### 1. Processor Not Found in Components List

**Symptoms:**
- `reservoir_sampler` not visible in `otelcol-nrdot components` output
- Configuration errors about unknown processor

**Potential Causes:**
- Incorrect import path in distribution.yaml
- Version mismatch between Go modules
- Processor code not correctly included in the build

**Diagnosis:**
```bash
# Check distribution.yaml for correct import path
grep -A 20 "processors:" distributions/nrdot-collector/distribution.yaml

# Verify Go module version
grep "github.com/deepaucksharma/nr-trace-aware-reservoir-otel" go.mod

# Check the binary for the component
./_dist/nrdot/otelcol-nrdot components | grep -i reservoir
```

**Resolution:**
1. Update the import path in distribution.yaml to match your module path exactly
2. Run `go mod tidy` to resolve dependencies
3. Make sure you run `make dist` after changes
4. Verify the processor is imported in the code

### 2. Checkpoint File Not Created

**Symptoms:**
- No `reservoir.db` file in the persistence volume
- `pte_reservoir_checkpoint_errors_total` metric increasing

**Potential Causes:**
- Permission issues with the volume mount
- Missing filestorageextension in configuration
- Incorrect checkpoint path configuration

**Diagnosis:**
```bash
# Check if PV is properly mounted
kubectl exec -n observability $POD -- ls -la /var/otelpersist/

# Check pod security context
kubectl describe pod -n observability $POD | grep -A 5 "Security Context"

# Verify storage extension is in config
kubectl exec -n observability $POD -- grep -A 10 "extensions:" /etc/otelcol-config.yaml
```

**Resolution:**
1. Ensure `persistence.enabled: true` in Helm values
2. Set the correct `fsGroup` to match the collector user (typically 10001)
3. Include `file_storage` in the extensions list in configuration
4. Set `checkpoint_path: "/var/otelpersist/reservoir.db"` in the configuration

### 3. High Memory Usage

**Symptoms:**
- Pod OOMKilled
- High memory usage shown in container metrics
- Pod restarts frequently

**Potential Causes:**
- Reservoir size too large for available memory
- Trace buffer max size too large
- High concurrency causing memory pressure

**Diagnosis:**
```bash
# Check memory usage metrics
kubectl top pod -n observability $POD

# Look for OOM events
kubectl describe pod -n observability $POD | grep -i "OOMKilled"

# Check current configuration values
kubectl exec -n observability $POD -- grep -A 20 "reservoir_sampler:" /etc/otelcol-config.yaml
```

**Resolution:**
1. Reduce `size_k` in reservoir sampler configuration
2. Decrease `trace_buffer_max_size` to limit concurrent traces
3. Ensure memory limits in the pod spec are sufficient:
   ```yaml
   resources:
     limits:
       memory: 4Gi
     requests:
       memory: 1Gi
   ```
4. Consider enabling memory ballast to stabilize GC

### 4. Excessive Trace Buffer Evictions

**Symptoms:**
- `pte_reservoir_lru_evictions_total` metric increasing rapidly
- Incomplete traces appearing in New Relic
- `pte_reservoir_trace_buffer_size` consistently near max

**Potential Causes:**
- Trace buffer too small for workload
- Long-running traces exceeding timeout
- High trace cardinality overwhelming the buffer

**Diagnosis:**
```bash
# Check buffer metrics
kubectl exec -n observability $POD -- curl -s localhost:8888/metrics | grep buffer

# Compare with traffic volume
kubectl exec -n observability $POD -- curl -s localhost:8888/metrics | grep "receiver_accepted_spans_total"
```

**Resolution:**
1. Increase `trace_buffer_max_size` to accommodate more concurrent traces
2. Adjust `trace_buffer_timeout` if traces take longer to complete
3. Consider if sampling can be done earlier in the pipeline for extremely high volume

### 5. Poor Sampling Distribution

**Symptoms:**
- Uneven trace representation in New Relic
- Some services overrepresented/underrepresented
- Irregular sampling rates across time

**Potential Causes:**
- Window duration too short for stable sampling
- Uneven trace distribution in input
- Random seed issues causing bias

**Diagnosis:**
```bash
# Check sampling metrics over time
kubectl exec -n observability $POD -- curl -s localhost:8888/metrics | grep "pte_reservoir_window_count"
kubectl exec -n observability $POD -- curl -s localhost:8888/metrics | grep "pte_reservoir_traces_in_reservoir_count"

# Compare with New Relic span distribution
# Use NRQL: SELECT count(*) FROM Span FACET service.name LIMIT 100
```

**Resolution:**
1. Increase `window_duration` for more stable sampling
2. Check for span attributes to ensure trace diversity
3. Consider multiple reservoir samplers with different parameters for complex environments

### 6. Database Size Growing Uncontrollably

**Symptoms:**
- `pte_reservoir_db_size_bytes` increasing steadily
- Disk space warnings on persistent volume
- Slow checkpoint operations

**Potential Causes:**
- Missing or ineffective database compaction
- Very large span attributes being stored
- Too frequent checkpoints creating file bloat

**Diagnosis:**
```bash
# Check database size
kubectl exec -n observability $POD -- du -sh /var/otelpersist/reservoir.db

# Verify compaction settings
kubectl exec -n observability $POD -- grep -A 3 "db_compaction" /etc/otelcol-config.yaml

# Check compaction metrics
kubectl exec -n observability $POD -- curl -s localhost:8888/metrics | grep "pte_reservoir_db_compactions"
```

**Resolution:**
1. Configure proper compaction with:
   ```yaml
   db_compaction_schedule_cron: "0 0 * * *"  # Daily at midnight
   db_compaction_target_size: 104857600  # 100MB
   ```
2. Consider a slightly larger checkpoint interval to reduce write frequency
3. Ensure adequate disk space on the persistent volume

### 7. Missing Reservoir Metrics

**Symptoms:**
- No `pte_reservoir_*` metrics visible in New Relic
- Processor appears in config but isn't exporting telemetry
- No data in monitoring dashboards

**Potential Causes:**
- Processor not in the active pipeline
- Telemetry disabled at processor or service level
- Metrics permission issues

**Diagnosis:**
```bash
# Check if processor is in pipeline
kubectl exec -n observability $POD -- grep -A 10 "pipelines:" /etc/otelcol-config.yaml

# Verify metrics are being generated
kubectl exec -n observability $POD -- curl -s localhost:8888/metrics | grep pte

# Check telemetry config
kubectl exec -n observability $POD -- grep -A 5 "telemetry:" /etc/otelcol-config.yaml
```

**Resolution:**
1. Ensure processor is included in the pipeline:
   ```yaml
   pipelines:
     traces:
       receivers: [otlp]
       processors: [memory_limiter, batch, reservoir_sampler]
       exporters: [otlphttp/newrelic]
   ```
2. Set detailed metrics level in service telemetry
3. Verify metrics are properly scraped/exported to New Relic

## Advanced Diagnostics

### Processor State Inspection

To examine the internal state of the reservoir sampler:

```bash
# Create a debug build with enhanced logging
go build -tags debug ./cmd/pte

# Enable debug logs in config
service:
  telemetry:
    logs:
      level: debug
```

### BoltDB Checkpoint Recovery

If the checkpoint database is corrupted:

1. Create a backup:
   ```bash
   kubectl cp observability/$POD:/var/otelpersist/reservoir.db ./reservoir.db.bak
   ```

2. Check database integrity:
   ```bash
   # Install boltdb tools
   go install github.com/boltdb/bolt/cmd/bolt@latest
   
   # Check database
   bolt stats ./reservoir.db.bak
   ```

3. Recover or reset as needed:
   ```bash
   # Option 1: Delete to force fresh start
   kubectl exec -n observability $POD -- rm /var/otelpersist/reservoir.db
   
   # Option 2: Upload fixed version
   kubectl cp ./fixed.db observability/$POD:/var/otelpersist/reservoir.db
   ```

### Performance Profiling

For advanced performance investigation:

```bash
# Enable pprof in NR-DOT config
extensions:
  pprof:
    endpoint: 0.0.0.0:1777

service:
  extensions: [pprof]

# Port forward pprof endpoint
kubectl port-forward -n observability $POD 1777:1777

# Collect CPU profile during high load
go tool pprof -seconds 30 http://localhost:1777/debug/pprof/profile

# Analyze heap during memory issues
go tool pprof http://localhost:1777/debug/pprof/heap
```

## Quick Recovery Actions

### 1. Reset Checkpoint Database

```bash
kubectl exec -n observability $POD -- rm /var/otelpersist/reservoir.db
```
*Effect*: Reservoir starts fresh, losing current sampling state but fixing corrupted DB issues.

### 2. Restart Pod with Clean State

```bash
kubectl delete pod -n observability $POD
```
*Effect*: Forces reload of configuration and processor initialization.

### 3. Scale Down Sampling Parameters

```bash
# Create a ConfigMap patch with reduced settings
kubectl create configmap sampler-emergency-config --from-literal=patch='
processors:
  reservoir_sampler:
    size_k: 1000  # Reduced from 5000
    trace_buffer_max_size: 10000  # Reduced from 100000
'

# Apply to running pod (requires configuration reloading enabled)
kubectl patch ... # Apply the configuration update
```
*Effect*: Reduces memory pressure in emergency situations.

## Prevention Best Practices

1. **Regular Monitoring**:
   - Set up the recommended alerts
   - Create dashboard for reservoir metrics
   - Include in regular system checks

2. **Capacity Planning**:
   - Size reservoirs based on expected trace volume
   - Allow headroom in memory and CPU resources 
   - Scale trace buffer with peak concurrent trace expectations

3. **Regular Testing**:
   - Validate checkpoint recovery process
   - Test resource limits with load testing
   - Verify sampling accuracy with controlled inputs

4. **Configuration Review**:
   - Audit processor configuration regularly
   - Adjust based on traffic patterns
   - Ensure alignment with NR-DOT version requirements