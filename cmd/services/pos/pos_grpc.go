package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	rds "syntra-system/config"
	"syntra-system/internal/database"
	"syntra-system/internal/services/pos/handler"
	proto "syntra-system/proto/protogen/pos"

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

	dsn := os.Getenv("POS_DSN")
	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}
	defer func() {
		sqlDB, err := db.DB()
		if err != nil {
			log.Printf("Failed to get SQL DB: %v", err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Printf("Failed to close DB: %v", err)
		}
	}()

	if err := database.MigratePOSDB(db); err != nil {
		log.Fatalf("Failed to migrate User database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	posHandler := handler.NewPOSHandler(db, redisClient)

	ctx, cancel := context.WithCancel(context.Background())

	go posHandler.StartDiscountScheduler(ctx)

	proto.RegisterPOSServiceServer(s, posHandler)
	reflection.Register(s)

	go func() {
		log.Println(" 💰 POS service listening on :50053")
		if err := s.Serve(lis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown signal received, initiating graceful shutdown...")

	cancel()

	s.GracefulStop()

	log.Println("POS service shutdown complete")
}
