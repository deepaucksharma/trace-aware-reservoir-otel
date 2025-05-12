# Environment Configuration Guide

This project uses environment variables to make all configuration names and values customizable. You can configure the entire stack through a simple `.env` file.

## Getting Started

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit the `.env` file to customize your configuration

3. Run the scripts which will automatically use your configuration

## Available Configuration Options

### Kind Cluster Configuration
- `K8S_VERSION`: Kubernetes version to install (default: 1.27.0)
- `VM_CPU`: Number of CPUs to allocate (default: 2)
- `VM_MEMORY`: Memory allocation in GB (default: 4G)

### Kubernetes Configuration
- `NAMESPACE`: Namespace for all Kubernetes resources (default: monitoring-suite)
- `NRIA_DEPLOYMENT_NAME`: Name for the NRIA deployment (default: nria-deployment)
- `OTEL_DEPLOYMENT_NAME`: Name for the standard OTEL deployment (default: otel-deployment)
- `OTEL_PROCESSOR_DEPLOYMENT_NAME`: Name for the OTEL with processor deployment (default: otel-processor-deployment)

### New Relic Configuration
- `NR_LICENSE_KEY`: Your New Relic license key (also used as the API key for OTLP ingest)
- `NR_USER_API_KEY`: (Optional) Your New Relic User API key for REST API access
- `NR_ACCOUNT_ID`: Your New Relic account ID
- `NR_ENDPOINT`: New Relic OTLP endpoint (default: https://otlp.nr-data.net:4318)

### Display Name Configuration
- `NRIA_DISPLAY_NAME_SUFFIX`: Suffix for NRIA's display name (default: infra)
- `OTEL_DISPLAY_NAME_SUFFIX`: Suffix for OTEL's display name (default: nrdot)
- `OTEL_PROCESSOR_DISPLAY_NAME_SUFFIX`: Suffix for OTEL processor's display name (default: nrdot-plus)

### OpenTelemetry Configuration
- `OTEL_IMAGE`: Image for OpenTelemetry collector (default: otel/opentelemetry-collector-contrib:latest)
- `OTEL_METRICS_COLLECTION_INTERVAL`: Interval for metrics collection (default: 15s)
- `OTEL_SERVICE_NAME`: Service name for metadata (default: vm-monitoring-service)

### Processor Configuration
- `RESERVOIR_SIZE_K`: Size of the reservoir (default: 1000)
- `WINDOW_DURATION`: Window duration for the reservoir (default: 1m)
- `TRACE_AWARE`: Enable trace-aware mode (default: true)
- `TRACE_BUFFER_MAX_SIZE`: Maximum trace buffer size (default: 10000)
- `TRACE_BUFFER_TIMEOUT`: Trace buffer timeout (default: 30s)
- `CHECKPOINT_PATH`: Path for checkpoint storage (default: /data/checkpoint)
- `CHECKPOINT_INTERVAL`: Checkpoint interval (default: 1m)
- `DB_COMPACTION_SCHEDULE`: Cron schedule for DB compaction (default: 0 0 * * *)
- `DB_COMPACTION_TARGET_SIZE`: Target size for DB compaction (default: 104857600 bytes / 100MB)

### Storage Configuration
- `PVC_NAME`: Name of the PersistentVolumeClaim (default: checkpoint-pvc)
- `PVC_SIZE`: Size of the PersistentVolumeClaim (default: 1Gi)

### Network Configuration
- `OTLP_GRPC_PORT`: Port for OTLP gRPC (default: 4317)
- `OTLP_HTTP_PORT`: Port for OTLP HTTP (default: 4318)

## Deployment Control

You can also control which components get deployed:

- `DEPLOY_INFRA`: Deploy New Relic Infrastructure Agent (default: true)
- `DEPLOY_OTEL`: Deploy standard OpenTelemetry Collector (default: true)
- `DEPLOY_OTEL_PROCESSOR`: Deploy OpenTelemetry with custom processor (default: true)

## How It Works

The `.env` file is loaded by all scripts, which then template the Kubernetes manifests with your custom values before deploying them. This ensures a consistent configuration across all components.

## Adding New Configuration

To add new configuration options:

1. Add the variable to `.env.example`
2. Update the environment loading code in the Kind scripts
3. Update the relevant Kubernetes manifest to use the variable with `{{ .VARIABLE_NAME }}` syntax