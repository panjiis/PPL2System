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

# Start all microservices in background, then wait 10s before gateway
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
