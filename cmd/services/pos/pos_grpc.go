package main

import (
	"log"
	"net"
	"os"

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
	defer redisClient.Close()

	dsn := os.Getenv("POS_DSN")
	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}

	if err := database.MigratePOSDB(db); err != nil {
		log.Fatalf("Failed to migrate User database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()

	posHandler := handler.NewPOSHandler(db, redisClient)

	proto.RegisterPOSServiceServer(s, posHandler)

	reflection.Register(s)

	log.Println(" 💰 POS service listening on :50053")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
