# ── Stage 1: Build ─────────────────────────────────────────────────────────────
# Build context must be the module root so go.mod / go.sum are accessible.
# docker build -f gateway/Dockerfile -t smuggle-gateway:dev .
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Resolve dependencies in a separate layer so it is cached independently of
# source changes. A cache miss on go.mod/go.sum invalidates this layer and
# everything below it, but an ordinary source edit does not.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /smuggle-gateway \
        ./gateway/

# ── Stage 2: Runtime ───────────────────────────────────────────────────────────
FROM alpine:3.23.2

# iptables is called by go-iptables to manage NAT and FORWARD rules.
# iproute2 (ip) is useful for inspecting the tunnel interface at runtime.
RUN apk add --no-cache iptables iproute2

# The gateway needs root: it creates kernel interfaces, programs iptables
# rules, and writes to /proc/sys/net/ipv4/ip_forward.
USER root

COPY --from=builder /smuggle-gateway /usr/local/bin/smuggle-gateway

# ── Runtime defaults ───────────────────────────────────────────────────────────
# All values can be overridden at deploy time via env vars or CLI flags.

# Overlay network this gateway sits in front of.
ENV SMUGGLE_GW_OVERLAY_CIDR=10.10.0.0/16

# Smuggle overlay interface — must match the interface name used by the
# Smuggle agent on the same host.
ENV SMUGGLE_GW_OVERLAY_IFACE=smuggle0

# Customer-facing WireGuard interface created by the gateway itself.
ENV SMUGGLE_GW_WG_IFACE=wg-gw0

# UDP port customers connect to. Expose this in your cluster/firewall rules.
ENV SMUGGLE_GW_WG_PORT=51820

# Private key is written to /data so it can be persisted via a mounted volume.
# Without persistence the key regenerates on restart and all registered
# customers lose connectivity until they re-register.
ENV SMUGGLE_GW_WG_KEY_FILE=/data/wg-gw0.key

# HTTP API — keep this internal; do not expose port 9091 externally.
ENV SMUGGLE_GW_HTTP_ADDR=0.0.0.0:9091

# Mount a persistent volume at /data to preserve the WireGuard key across
# container restarts.
VOLUME ["/data"]

# 9091  — HTTP registration API (internal only)
# 51820 — WireGuard UDP (must be reachable by customers)
EXPOSE 9091
EXPOSE 51820/udp

ENTRYPOINT ["/usr/local/bin/smuggle-gateway"]
