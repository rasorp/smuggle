FROM alpine:3.23.2

# TARGETARCH and TARGETOS are automatically set by Docker when using the "docker
# buildx" command.
ARG TARGETARCH
ARG TARGETOS

# Create a non-root user that can be used when running Smuggle in server mode.
RUN \
    addgroup -g 1000 smuggle && \
    adduser -D -u 1000 -G smuggle smuggle

# Copy the pre-built binary from bin/linux_{arch}/smuggle. This ensures the same
# binaries are used no matter if you run Smuggle as a container or natively on
# the host.
COPY bin/${TARGETOS}_${TARGETARCH}/smuggle /usr/local/bin/smuggle

# Default to root user since client mode requires it. This can be overriden with
# the "--user smuggle" flag for server mode deployments.
USER root

ENTRYPOINT ["/usr/local/bin/smuggle"]
CMD ["agent", "--help"]
