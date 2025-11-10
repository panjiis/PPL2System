package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

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
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Failed to close Redis: %v", err)
		}
	}()

	dsn := os.Getenv("INVENTORY_DSN")
	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

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

	go func() {
		log.Println(" 📦 Inventory service listening on :50052")
		if err := s.Serve(lis); err != nil {
			log.Printf("Inventory gRPC server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Inventory service shutdown signal received...")

	s.GracefulStop()
	log.Println("Inventory service shutdown complete")
}
