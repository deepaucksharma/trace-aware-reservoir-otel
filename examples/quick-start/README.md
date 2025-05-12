# Quick Start Examples

This directory contains ready-to-use configurations for trace-aware reservoir sampling with NR-DOT.

## Available Configurations

### 1. Balanced Configuration (`config-balanced.yaml`)

General-purpose settings suitable for most environments. This is a good starting point if you're unsure which configuration to use.

```bash
# Copy to use as your configuration
cp examples/quick-start/config-balanced.yaml reservoir-config.yaml

# Or use directly with the environment variable
export NRDOT_CONFIG_PATH=examples/quick-start/config-balanced.yaml
./install.sh
```

### 2. High-Volume Configuration (`config-high-volume.yaml`)

Optimized for environments with high trace volumes (100K+ spans per second). Uses more resources but provides better sampling fidelity and durability.

```bash
# Copy to use as your configuration
cp examples/quick-start/config-high-volume.yaml reservoir-config.yaml

# Or use directly with the environment variable
export NRDOT_CONFIG_PATH=examples/quick-start/config-high-volume.yaml
./install.sh
```

### 3. Low-Resource Configuration (`config-low-resource.yaml`)

Designed for resource-constrained environments or testing. Minimizes resource usage at the cost of reduced sampling volume.

```bash
# Copy to use as your configuration
cp examples/quick-start/config-low-resource.yaml reservoir-config.yaml

# Or use directly with the environment variable
export NRDOT_CONFIG_PATH=examples/quick-start/config-low-resource.yaml
./install.sh
```

## Using with Environment Variables

You can selectively override any configuration parameter using environment variables:

```bash
# Example: Use balanced config but with a larger reservoir
cp examples/quick-start/config-balanced.yaml reservoir-config.yaml
export RESERVOIR_SIZE_K=8000
./install.sh
```

See `.env.example` in the root directory for all available environment variables.