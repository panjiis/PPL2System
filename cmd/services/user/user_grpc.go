package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	rds "syntra-system/config"
	"syntra-system/internal/database"
	"syntra-system/internal/services/user/handler"
	proto "syntra-system/proto/protogen/user"

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

	dsn := os.Getenv("USER_DSN")
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

	if err := database.MigrateUserDB(db); err != nil {
		log.Fatalf("Failed to migrate User database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	userHandler := handler.NewUserHandler(db, redisClient)
	proto.RegisterUserServiceServer(s, userHandler)
	reflection.Register(s)

	go func() {
		log.Println(" 👱🏻‍♂️ User service listening on :50051")
		if err := s.Serve(lis); err != nil {
			log.Printf("User gRPC server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("User service shutdown signal received...")

	s.GracefulStop()
	log.Println("User service shutdown complete")
}
