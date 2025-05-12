# NR-DOT Integration Example

This example demonstrates how to integrate the trace-aware reservoir sampling processor with the New Relic Distribution of OpenTelemetry (NR-DOT).

## Prerequisites

- New Relic account with a valid license key
- Docker or Kubernetes environment
- NR-DOT installation or repository

## Quick Start

### 1. Set your New Relic license key

```bash
export NR_LICENSE_KEY=your-license-key
```

### 2. Run with Docker

```bash
# Run with Docker
docker run -v $(pwd)/nrdot-config.yaml:/etc/otel/config.yaml \
           -v $(pwd)/data:/var/otelpersist \
           -p 4317:4317 -p 4318:4318 -p 8888:8888 \
           -e NR_LICENSE_KEY=$NR_LICENSE_KEY \
           otel/opentelemetry-collector-contrib:latest \
           --config /etc/otel/config.yaml
```

### 3. Generate additional configurations

Use the `pte` command-line tool to generate optimized configurations:

```bash
# Generate NR-DOT configuration optimized for high-volume
pte nrdot-integration --generate-config --optimization high --output nrdot-high-volume.yaml

# Generate NR-DOT configuration optimized for APM entity
pte nrdot-integration --generate-config --entity-type apm --output nrdot-apm.yaml

# Generate NR-DOT configuration optimized for browser entity
pte nrdot-integration --generate-config --entity-type browser --output nrdot-browser.yaml
```

### 4. Register with NR-DOT

If you have an existing NR-DOT installation, you can register the reservoir sampler with it:

```bash
# Register with NR-DOT
pte nrdot-integration --nrdot-path /path/to/nrdot
```

## Configuration Details

The provided `nrdot-config.yaml` includes:

- OTLP receiver for both gRPC and HTTP protocols
- Batch processor for efficient trace processing
- Trace-aware reservoir sampling processor for sampling
- New Relic OTLP exporter for sending sampled traces to New Relic
- Health check, pprof, and zpages extensions for monitoring and debugging

## Testing the Integration

Send test traces to verify your integration:

```bash
# For Docker mode
export OTLP_ENDPOINT=http://localhost:4318
./test-traces.sh

# For Kubernetes mode
export OTLP_ENDPOINT=http://<service-ip>:4318
./test-traces.sh
```

## Viewing Traces in New Relic

After sending traces, you can view them in New Relic:

1. Log in to your New Relic account
2. Navigate to One Dashboard
3. Select APM & Services
4. Look for your service name in the list
5. Click on Distributed Tracing to view your traces

## New Relic Dashboard

You can also create a dashboard in New Relic to visualize the performance of your reservoir sampler. See the [NR-DOT Dashboard Guide](../../docs/nrdot/dashboards.md) for details.