package main

import (
	"log"
	"net"
	"os"

	rds "syntra-system/config"
	"syntra-system/internal/database"
	"syntra-system/internal/services/inventory/handler"
	proto "syntra-system/proto/protogen/inventory"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	godotenv.Load()
	server := rds.LoadConfig()

	redisClient := rds.NewRedisClient(server.Redis)
	defer redisClient.Close()

	dsn := os.Getenv("INVENTORY_DSN")
	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}

	if err := database.MigrateInventoryDB(db); err != nil {
		log.Fatalf("Failed to migrate Inventory database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()

	inventoryHandler := handler.NewInventoryHandler(db, redisClient)
	proto.RegisterInventoryServiceServer(s, inventoryHandler)

	reflection.Register(s)

	log.Println(" 📦 inventory service listening on :50052")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
