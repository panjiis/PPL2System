package main

import (
	"log"
	"net"
	"os"

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
	serverCfg := rds.LoadConfig() // Menggunakan nama variabel yang lebih generik

	redisClient := rds.NewRedisClient(serverCfg.Redis)
	defer redisClient.Close()

	dsn := os.Getenv("COMMISSION_DSN")

	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}

	// Anda perlu membuat fungsi migrasi spesifik untuk tabel komisi
	if err := database.MigrateCommissionDB(db); err != nil {
		log.Fatalf("Failed to migrate Commission database: %v", err)
	}

	// Gunakan port yang BERBEDA dari service lain
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()

	// Buat instance dari CommissionHandler
	commissionHandler := handler.NewCommissionHandler(db, redisClient)
	// Daftarkan CommissionServiceServer
	proto.RegisterCommissionServiceServer(s, commissionHandler)

	reflection.Register(s)

	// Ubah pesan log
	log.Println(" 💰 Commission service listening on :50052")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}