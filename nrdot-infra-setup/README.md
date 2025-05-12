# New Relic Infrastructure Agent & NRDOT Dual Setup

This setup provides a dual configuration for monitoring host and process metrics using both the New Relic Infrastructure Agent and the New Relic NRDOT (OpenTelemetry) Collector.

## Overview

This dual setup offers the following benefits:
- **Complete host metrics** from the New Relic Infrastructure Agent (CPU, memory, disk, network)
- **Detailed process metrics** from both Infrastructure Agent and NRDOT
- **Containerized deployment** for easy installation and management
- **Complementary data collection** for comprehensive monitoring

## Components

1. **New Relic Infrastructure Agent**:
   - Provides system inventory (packages, services)
   - Monitors host metrics (CPU, memory, disk, network)
   - Collects process metrics
   - Automatically detects Docker containers

2. **NRDOT OpenTelemetry Collector**:
   - Collects host metrics using the OpenTelemetry standard
   - Gathers detailed process metrics
   - Provides standardized metrics format

## Getting Started

### Prerequisites

- Docker and Docker Compose installed
- A New Relic account and license key

### Installation

1. Clone this repository
2. Configure your New Relic license key:
   ```bash
   cp .env.example .env
   # Edit .env to set your New Relic license key
   ```
3. Run the installation script:
   ```bash
   chmod +x scripts/install.sh
   ./scripts/install.sh
   ```

### Managing the Services

Use the management script to control the services:

```bash
chmod +x scripts/manage.sh
./scripts/manage.sh status  # Check service status
./scripts/manage.sh logs    # View logs from both services
```

Available commands:
- `start`: Start all services
- `stop`: Stop all services
- `restart`: Restart all services
- `status`: Show the status of all services
- `logs [svc]`: Show logs for all services or a specific one
- `update`: Update Docker images and restart services

## Configuration

### New Relic Infrastructure Agent

The configuration file is located at `newrelic/newrelic-infra.yml`. You can customize it to change:
- Collection intervals
- Host metrics settings
- Process metrics settings

### NRDOT Collector

The configuration file is located at `nrdot/config.yaml`. Key configuration areas:
- Host metrics receivers
- Process metrics settings
- Metrics exporters

## Viewing Data in New Relic

After setting up, you can view your metrics in New Relic:

1. Go to New Relic One
2. Navigate to Infrastructure > Hosts
3. Your host should appear with metrics from both sources
4. Process metrics will be available in both Infrastructure and in Metrics explorer

## Troubleshooting

If you're experiencing issues:

1. Check logs for both services:
   ```bash
   ./scripts/manage.sh logs
   ```

2. Verify network connectivity to New Relic endpoints:
   - otlp.nr-data.net:4317 (NRDOT)
   - infrastructure-api.newrelic.com (Infrastructure Agent)

3. Confirm your license key is valid and properly configured in the .env file