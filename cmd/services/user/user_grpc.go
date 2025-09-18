package main

import (
	"log"
	"net"
	"os"

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
	defer redisClient.Close()

	dsn := os.Getenv("USER_DSN")
	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}

	if err := database.MigrateUserDB(db); err != nil {
		log.Fatalf("Failed to migrate User database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()

	psnHandler := handler.NewUserHandler(db, redisClient)
	proto.RegisterUserServiceServer(s, psnHandler)

	reflection.Register(s)

	log.Println(" 👱🏻‍♂️ User service listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
