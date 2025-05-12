FROM alpine:3.19

# Copy the collector binary
COPY otelcol-nrdot /otelcol-nrdot

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
