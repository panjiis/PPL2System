FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build all binaries
RUN CGO_ENABLED=0 go build -o /app/bin/gateway ./cmd/gateway && \
    CGO_ENABLED=0 go build -o /app/bin/user ./cmd/services/user && \
    CGO_ENABLED=0 go build -o /app/bin/commissions ./cmd/services/commissions && \
    CGO_ENABLED=0 go build -o /app/bin/pos ./cmd/services/pos && \
    CGO_ENABLED=0 go build -o /app/bin/inventory ./cmd/services/inventory

# Runtime image
FROM alpine:latest

RUN apk add --no-cache tini

WORKDIR /app

COPY --from=builder /app/config/ ./config/
COPY --from=builder /app/bin/ ./bin/

EXPOSE 8080

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["sh", "-c", "./bin/user & ./bin/commissions & ./bin/pos & ./bin/inventory & ./bin/gateway"]
