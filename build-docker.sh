#!/bin/bash
# Build script using Docker to avoid dependency on Make

# Configuration
NRDOT_REPO="https://github.com/newrelic/opentelemetry-collector-releases.git"
BRANCH="feat/reservoir-sampler"
IMAGE="ghcr.io/your-org/nrdot-reservoir:v0.1.0"

# Create a temporary Dockerfile for building
cat > Dockerfile.builder << EOF
FROM golang:1.21 as builder

# Clone NR-DOT repository
RUN git clone ${NRDOT_REPO} /build && \
    cd /build && \
    git checkout -b ${BRANCH}

# Copy updated manifests (if needed)
COPY manifest.yaml.patches /tmp/

# Apply manifest changes (example - adjust as needed)
RUN if [ -f /tmp/manifest.yaml.patches ]; then \
      cd /build && \
      cp /tmp/manifest.yaml.patches distributions/nrdot-collector-k8s/manifest.yaml; \
    fi

# Build the distribution
RUN cd /build && make dist

FROM alpine:3.19

# Copy the collector binary from the builder
COPY --from=builder /build/_dist/nrdot/otelcol-nrdot /otelcol-nrdot

# Create a non-root user to run the collector
RUN addgroup -g 10001 otel && \
    adduser -D -G otel -u 10001 otel

# Set up directories for checkpoints and data
RUN mkdir -p /var/otelpersist/badger && \
    chown -R otel:otel /var/otelpersist

USER otel

ENTRYPOINT ["/otelcol-nrdot"]
CMD ["--config=/etc/otelcol-nrdot/config.yaml"]

EXPOSE 4317 4318 8888
EOF

# Build the Docker image
docker build -t ${IMAGE} -f Dockerfile.builder .

echo "Build complete. Image: ${IMAGE}"
echo "To push the image, run: docker push ${IMAGE}"
