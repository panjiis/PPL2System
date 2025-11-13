FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /app/server ./cmd/gateway

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/config/ ./config/

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]