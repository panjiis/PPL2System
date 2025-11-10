package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	rds "syntra-system/config"
	"syntra-system/internal/database"
	"syntra-system/internal/services/commissions/handler"
	proto "syntra-system/proto/protogen/commissions"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	godotenv.Load()
	serverCfg := rds.LoadConfig()

	redisClient := rds.NewRedisClient(serverCfg.Redis)
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Failed to close Redis: %v", err)
		}
	}()

	dsn := os.Getenv("COMMISSIONS_DSN")
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

	if err := database.MigrateCommissionDB(db); err != nil {
		log.Fatalf("Failed to migrate Commission database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	commissionHandler := handler.NewCommissionHandler(db, redisClient)
	proto.RegisterCommissionServiceServer(s, commissionHandler)
	reflection.Register(s)

	go func() {
		log.Println(" 💰 Commission service listening on :50054")
		if err := s.Serve(lis); err != nil {
			log.Printf("Commission gRPC server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Commission service shutdown signal received...")

	s.GracefulStop()
	log.Println("Commission service shutdown complete")
}
