# Performance Tests

This directory is reserved for performance tests and benchmarks for the trace-aware reservoir sampler.

## Future Test Plans

- Memory usage benchmarks for various reservoir sizes
- Throughput tests for different span volumes
- Latency tests for checkpoint operations
- Serialization performance across different data sizes

## Running Benchmarks

To run the existing benchmarks:

```bash
make bench
```

This will run the benchmark tests in the reservoirsampler package.