package clients

import (
	"fmt"
	"log"

	commissions "syntra-system/proto/protogen/commissions"
	inventory "syntra-system/proto/protogen/inventory"
	pos "syntra-system/proto/protogen/pos"
	user "syntra-system/proto/protogen/user"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCClients struct {
	User           user.UserServiceClient
	Inventory      inventory.InventoryServiceClient
	POS            pos.POSServiceClient
	Commissions    commissions.CommissionServiceClient
	userConn       *grpc.ClientConn
	inventoryConn  *grpc.ClientConn
	posConn        *grpc.ClientConn
	commissionConn *grpc.ClientConn
}

func NewGRPCClients() (*GRPCClients, error) {
	userConn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		userConn.Close()
		return nil, fmt.Errorf("auth service connection failed: %v", err)
	}

	inventoryConn, err := grpc.NewClient("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		inventoryConn.Close()
		return nil, fmt.Errorf("psn service connection failed: %v", err)
	}

	posConn, err := grpc.NewClient("localhost:50053", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		posConn.Close()
		return nil, fmt.Errorf("kpbu service connection failed: %v", err)
	}

	commissionConn, err := grpc.NewClient("localhost:50053", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		commissionConn.Close()
		return nil, fmt.Errorf("notulensi service connection failed: %v", err)
	}

	clients := &GRPCClients{
		User:           user.NewUserServiceClient(userConn),
		Inventory:      inventory.NewInventoryServiceClient(inventoryConn),
		POS:            pos.NewPOSServiceClient(posConn),
		Commissions:    commissions.NewCommissionServiceClient(commissionConn),
		userConn:       userConn,
		inventoryConn:  inventoryConn,
		posConn:        posConn,
		commissionConn: commissionConn,
	}

	log.Println("✅ Connected to all gRPC services")
	return clients, nil
}

func (c *GRPCClients) Close() {
	if c.userConn != nil {
		c.userConn.Close()
	}
	if c.inventoryConn != nil {
		c.inventoryConn.Close()
	}
	if c.posConn != nil {
		c.posConn.Close()
	}
	if c.commissionConn != nil {
		c.commissionConn.Close()
	}
}
