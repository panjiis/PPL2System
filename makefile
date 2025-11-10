.PHONY: proto build run-auth run-psn run-gateway run-all clean test

# Generate proto files
proto:
	@echo "🔧 Generating proto files..."
	protoc --proto_path=proto \
		--go_out=proto/protogen --go_opt=paths=source_relative \
		--go-grpc_out=proto/protogen --go-grpc_opt=paths=source_relative \
		proto/analytics/*.proto \
		proto/commissions/*.proto \
		proto/inventory/*.proto \
		proto/pos/*.proto \
		proto/user/*.proto
# Build all services
build:
	@echo "🔨 Building services..."
	go build -o bin/auth-service ./cmd/services/auth
	go build -o bin/psn-service ./cmd/services/psn
	go build -o bin/kpbu-service ./cmd/services/kpbu
	go build -o bin/gateway ./cmd/gateway

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod tidy
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Run individual services
run-auth:
	@echo "🔐 Starting auth service..."
	go run ./cmd/auth-service

run-psn:
	@echo "👤 Starting user service..."
	go run ./cmd/psn-service

run-kpbu:
	@echo "📁 Starting KPBU service..."
	go run ./cmd/kpbu-service

run-gateway:
	@echo "🚀 Starting gateway..."
	go run ./cmd/gateway

# Run all services with Docker
run-all:
	@echo "🐳 Starting all services with Docker..."
	docker-compose up --build

# Run all services locally (requires 3 terminals)
run-local:
	@echo "💻 To run locally, use these commands in separate terminals:"
	@echo "Terminal 1: make run-auth"
	@echo "Terminal 2: make run-psn"
	@echo "Terminal 3: make run-kpbu"
	@echo "Terminal 4: make run-gateway"

# Test endpoints
test:
	@echo "🧪 Testing endpoints..."
	curl -X POST http://localhost:8080/api/v1/auth/register \
		-H "Content-Type: application/json" \
		-d '{"username":"testuser","password":"password123"}'
	@echo "\n"
	curl -X POST http://localhost:8080/api/v1/auth/login \
		-H "Content-Type: application/json" \
		-d '{"username":"testuser","password":"password123"}'

# Clean build artifacts
clean:
	@echo "🧹 Cleaning up..."
	rm -rf bin/
	docker-compose down -v
	docker system prune -f

# Development setup
dev-setup: deps proto build
	@echo "✅ Development environment ready!"