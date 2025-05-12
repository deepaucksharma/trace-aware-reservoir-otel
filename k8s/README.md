# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the trace-aware reservoir sampling processor with various monitoring configurations.

## Directory Structure

- `00-namespace.yaml` - Creates the monitoring namespace
- `00-newrelic-secrets.yaml` - Secret template for New Relic credentials
- `10-nria-*.yaml` - New Relic Infrastructure Agent deployment
- `20-otel-*.yaml` - Standard OpenTelemetry Collector deployment
- `30-otel-processor-*.yaml` - OpenTelemetry Collector with custom processor

## Deployment Options

### 1. New Relic Infrastructure Agent

Basic VM monitoring with the New Relic Infrastructure Agent.

### 2. OpenTelemetry Collector (NR-DOT)

OpenTelemetry Collector configured for standardized host metrics.

### 3. OpenTelemetry with Custom Processor (NR-DOT-Plus)

OpenTelemetry Collector with the trace-aware reservoir sampling processor.

## Deployment

These manifests are designed to be deployed using the scripts in the `../kind/` directory:

```bash
../kind/deploy-monitoring.sh
```

## Configuration

The manifests use ConfigMaps for configuration. You can customize:

- Collection intervals
- Sampling parameters
- Processor behavior
- Exporters configuration

## Monitoring Stack Architecture

```
┌─────────────────────────────────────────────┐
│                   Kubernetes                 │
│                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐  │
│  │     NRIA    │   │  NR-DOT Std │   │ NR-DOT Plus │  │
│  │             │   │             │   │             │  │
│  │  Infra      │   │  OTel       │   │  OTel +     │  │
│  │  Agent      │   │  Collector  │   │  Processor  │  │
│  └─────┬───────┘   └─────┬───────┘   └─────┬───────┘  │
│        │                 │                 │          │
└────────┼─────────────────┼─────────────────┼──────────┘
         │                 │                 │
         v                 v                 v
   ┌─────────────────────────────────────────────┐
   │             New Relic Platform              │
   └─────────────────────────────────────────────┘
```