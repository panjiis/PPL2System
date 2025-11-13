# ---------- BUILD STAGE ----------
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy and download dependencies first (for better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build all microservices
RUN CGO_ENABLED=0 go build -o /app/bin/gateway ./cmd/gateway && \
    CGO_ENABLED=0 go build -o /app/bin/user ./cmd/services/user && \
    CGO_ENABLED=0 go build -o /app/bin/commissions ./cmd/services/commissions && \
    CGO_ENABLED=0 go build -o /app/bin/pos ./cmd/services/pos && \
    CGO_ENABLED=0 go build -o /app/bin/inventory ./cmd/services/inventory

# ---------- RUNTIME STAGE ----------
FROM alpine:latest

# Use tini for proper signal handling
RUN apk add --no-cache tini

WORKDIR /app

# Copy configuration files and built binaries
COPY --from=builder /app/config/ ./config/
COPY --from=builder /app/bin/ ./bin/

# Expose all ports (gRPC + Gateway)
EXPOSE 8080 50051 50052 50053 50054

# Environment variables (can be overridden in docker-compose)
ENV APP_ENV=production \
    APP_PORT=8080

ENTRYPOINT ["/sbin/tini", "--"]

# Start all microservices in background, wait 10s, then start gateway
CMD ["sh", "-c", "\
  echo '🚀 Starting backend microservices...' && \
  ./bin/user & \
  ./bin/commissions & \
  ./bin/pos & \
  ./bin/inventory & \
  echo '🕒 Waiting 10 seconds for all services to initialize...' && \
  sleep 10 && \
  echo '🌐 Starting Gateway service...' && \
  ./bin/gateway"]
