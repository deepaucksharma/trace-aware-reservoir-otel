# User Guide

This user guide provides information on how to install, configure, and use the trace-aware reservoir sampling processor with OpenTelemetry.

## Contents

- [Getting Started](getting-started.md) - Quick start guide for first-time users
- [Installation](installation.md) - Detailed installation instructions
- [Configuration](configuration.md) - Configuration options and examples
- [Usage Scenarios](usage-scenarios.md) - Common usage scenarios and examples
- [Performance Tuning](performance-tuning.md) - Optimizing for different environments
- [Monitoring](monitoring.md) - How to monitor the reservoir sampler
- [Troubleshooting](troubleshooting.md) - Common issues and solutions

## Installation Options

### Option 1: Docker Mode (Local Development)

```bash
# Set Docker mode
export NRDOT_MODE=docker

# Run the installer
./install.sh
```

### Option 2: Kubernetes Mode (Production)

```bash
# Set Kubernetes mode (default)
export NRDOT_MODE=kubernetes

# Run the installer
./install.sh
```

## Basic Configuration Example

```yaml
# Size of the reservoir (number of traces to sample)
size_k: 5000

# Duration of each sampling window
window_duration: "60s"

# How often to save checkpoint state
checkpoint_interval: "10s"

# Whether to keep entire traces together
trace_aware: true

# Maximum spans to buffer while waiting for trace completion
trace_buffer_max_size: 100000

# Maximum time to wait for spans in a trace
trace_buffer_timeout: "30s"
```

## Key Metrics to Monitor

| Metric | Description | Target | Alert When |
|--------|-------------|--------|------------|
| `pte_reservoir_traces_in_reservoir_count` | Current number of traces | Matches size_k | < 10% of size_k |
| `pte_reservoir_checkpoint_age_seconds` | Time since last checkpoint | < checkpoint_interval | > 2x checkpoint_interval |
| `pte_reservoir_db_size_bytes` | Size of checkpoint file | Stable growth | Sudden spike |
| `pte_reservoir_lru_evictions_total` | Buffer evictions count | Low | High or rapidly increasing |