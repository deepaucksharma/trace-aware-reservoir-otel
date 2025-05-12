# Examples

This directory contains examples and sample configurations for the trace-aware reservoir sampling processor.

## Quick Start Examples

- [Basic Configuration](basic-configuration.yaml) - A simple configuration for getting started
- [Production Configuration](production-configuration.yaml) - A production-ready configuration
- [High-Volume Configuration](high-volume-configuration.yaml) - Configuration for high-volume environments
- [Resource-Constrained Configuration](resource-constrained-configuration.yaml) - Configuration for resource-constrained environments

## Deployment Examples

- [Docker Compose](docker-compose.yaml) - Docker Compose deployment
- [Kubernetes](kubernetes.yaml) - Kubernetes deployment
- [Helm Chart](helm-chart.yaml) - Helm chart deployment

## NR-DOT Integration Examples

- [NR-DOT Basic Integration](nrdot-basic-integration.yaml) - Basic integration with NR-DOT
- [NR-DOT Production Integration](nrdot-production-integration.yaml) - Production integration with NR-DOT
- [NR-DOT High-Volume Integration](nrdot-high-volume-integration.yaml) - High-volume integration with NR-DOT

## Usage Examples

### Basic Configuration

```yaml
processors:
  reservoir_sampler:
    size_k: 5000
    window_duration: "60s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "10s"
    trace_aware: true
    trace_buffer_max_size: 100000
    trace_buffer_timeout: "30s"
```

### High-Volume Configuration

```yaml
processors:
  reservoir_sampler:
    size_k: 10000
    window_duration: "30s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "15s"
    trace_aware: true
    trace_buffer_max_size: 200000
    trace_buffer_timeout: "15s"
    db_compaction_schedule_cron: "0 */6 * * *" # Every 6 hours
    db_compaction_target_size: 1073741824 # 1GB
```

### Resource-Constrained Configuration

```yaml
processors:
  reservoir_sampler:
    size_k: 1000
    window_duration: "120s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "30s"
    trace_aware: true
    trace_buffer_max_size: 20000
    trace_buffer_timeout: "45s"
    db_compaction_schedule_cron: "0 0 * * *" # Once a day
    db_compaction_target_size: 536870912 # 512MB
```

## Docker Compose Example

```yaml
version: "3"
services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    command: ["--config=/etc/otel/config.yaml"]
    volumes:
      - ./config.yaml:/etc/otel/config.yaml
      - ./data:/var/otelpersist
    ports:
      - "4317:4317" # OTLP gRPC
      - "4318:4318" # OTLP HTTP
      - "8888:8888" # Metrics
      - "13133:13133" # Health
    environment:
      - RESERVOIR_SIZE_K=5000
      - RESERVOIR_WINDOW_DURATION=60s
```

## Kubernetes Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-collector
  namespace: observability
spec:
  replicas: 1
  selector:
    matchLabels:
      app: otel-collector
  template:
    metadata:
      labels:
        app: otel-collector
    spec:
      containers:
      - name: otel-collector
        image: otel/opentelemetry-collector-contrib:latest
        args:
        - "--config=/etc/otel/config.yaml"
        volumeMounts:
        - name: config
          mountPath: /etc/otel/config.yaml
          subPath: config.yaml
        - name: data
          mountPath: /var/otelpersist
        ports:
        - containerPort: 4317 # OTLP gRPC
        - containerPort: 4318 # OTLP HTTP
        - containerPort: 8888 # Metrics
        - containerPort: 13133 # Health
      volumes:
      - name: config
        configMap:
          name: otel-collector-config
      - name: data
        persistentVolumeClaim:
          claimName: otel-collector-data
```