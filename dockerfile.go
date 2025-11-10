FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /app/main-binary ./cmd/gateway


FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /

COPY --from=builder /app/main-binary /main-binary

EXPOSE 8080

CMD ["/main-binary"]