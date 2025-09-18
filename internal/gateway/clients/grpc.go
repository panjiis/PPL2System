package clients

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	commissions "syntra-system/proto/protogen/commissions"
	inventory "syntra-system/proto/protogen/inventory"
	pos "syntra-system/proto/protogen/pos"
	user "syntra-system/proto/protogen/user"
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

func NewGRPCClientsWithFallback() (*GRPCClients, error) {
	clients := &GRPCClients{}
	connectedServices := 0

	log.Printf("Attempting to connect to User service...")

	if userConn, err := connectToService("localhost:50051"); err != nil {
		log.Printf("Failed to connect to User service: %v", err)
	} else {
		clients.User = user.NewUserServiceClient(userConn)
		clients.userConn = userConn
		log.Printf("✅ Successfully connected to User service")
		connectedServices++
	}

	log.Printf("Attempting to connect to Inventory service...")

	if inventoryConn, err := connectToService("localhost:50052"); err != nil {
		log.Printf("Failed to connect to Inventory service: %v", err)
	} else {
		clients.Inventory = inventory.NewInventoryServiceClient(inventoryConn)
		clients.inventoryConn = inventoryConn
		log.Printf("✅ Successfully connected to Inventory service")
		connectedServices++
	}

	log.Printf("Attempting to connect to POS service...")

	if posConn, err := connectToService("localhost:50053"); err != nil {
		log.Printf("Failed to connect to POS service: %v", err)
	} else {
		clients.POS = pos.NewPOSServiceClient(posConn)
		clients.posConn = posConn
		log.Printf("✅ Successfully connected to POS service")
		connectedServices++
	}

	log.Printf("Attempting to connect to Commissions service...")

	if commissionConn, err := connectToService("localhost:50054"); err != nil {
		log.Printf("Failed to connect to Commissions service: %v", err)
	} else {
		clients.Commissions = commissions.NewCommissionServiceClient(commissionConn)
		clients.commissionConn = commissionConn
		log.Printf("✅ Successfully connected to Commissions service")
		connectedServices++
	}

	if connectedServices == 0 {
		return nil, fmt.Errorf("all gRPC services are currently unavailable")
	}

	log.Println("⚡️ Client initialization complete. Check logs for connection status.")
	return clients, nil
}

func connectToService(addr string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)

	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (g *GRPCClients) Close() {
	if g.userConn != nil {
		log.Printf("Closing User service connection")
		g.userConn.Close()
	}
	if g.inventoryConn != nil {
		log.Printf("Closing Inventory service connection")
		g.inventoryConn.Close()
	}
	if g.posConn != nil {
		log.Printf("Closing POS service connection")
		g.posConn.Close()
	}
	if g.commissionConn != nil {
		log.Printf("Closing Commissions service connection")
		g.commissionConn.Close()
	}
}

func (g *GRPCClients) IsUserServiceHealthy() bool {
	if g.userConn == nil {
		return false
	}
	state := g.userConn.GetState()

	return state == connectivity.Ready
}

func (g *GRPCClients) IsInventoryServiceHealthy() bool {
	if g.inventoryConn == nil {
		return false
	}
	state := g.inventoryConn.GetState()

	return state == connectivity.Ready
}

func (g *GRPCClients) IsPOSServiceHealthy() bool {
	if g.posConn == nil {
		return false
	}
	state := g.posConn.GetState()

	return state == connectivity.Ready
}

func (g *GRPCClients) IsCommissionsServiceHealthy() bool {
	if g.commissionConn == nil {
		return false
	}
	state := g.commissionConn.GetState()

	return state == connectivity.Ready
}

func (g *GRPCClients) GetServiceStatus() map[string]string {
	status := make(map[string]string)

	if g.IsUserServiceHealthy() {
		status["user"] = "healthy"
	} else {
		status["user"] = "unhealthy"
	}
	if g.IsInventoryServiceHealthy() {
		status["inventory"] = "healthy"
	} else {
		status["inventory"] = "unhealthy"
	}
	if g.IsPOSServiceHealthy() {
		status["pos"] = "healthy"
	} else {
		status["pos"] = "unhealthy"
	}
	if g.IsCommissionsServiceHealthy() {
		status["commissions"] = "healthy"
	} else {
		status["commissions"] = "unhealthy"
	}

	return status
}

func (g *GRPCClients) ReconnectUserService() error {
	log.Printf("Attempting to reconnect to User service...")
	if g.userConn != nil {
		g.userConn.Close()
	}

	userConn, err := connectToService("localhost:50051")
	if err != nil {
		g.User = nil
		g.userConn = nil
		return err
	}
	g.User = user.NewUserServiceClient(userConn)
	g.userConn = userConn
	log.Printf("Successfully reconnected to User service")
	return nil
}

func (g *GRPCClients) ReconnectInventoryService() error {
	log.Printf("Attempting to reconnect to Inventory service...")
	if g.inventoryConn != nil {
		g.inventoryConn.Close()
	}

	inventoryConn, err := connectToService("localhost:50052")
	if err != nil {
		g.Inventory = nil
		g.inventoryConn = nil
		return err
	}
	g.Inventory = inventory.NewInventoryServiceClient(inventoryConn)
	g.inventoryConn = inventoryConn
	log.Printf("Successfully reconnected to Inventory service")
	return nil
}

func (g *GRPCClients) ReconnectPOSService() error {
	log.Printf("Attempting to reconnect to POS service...")
	if g.posConn != nil {
		g.posConn.Close()
	}

	posConn, err := connectToService("localhost:50053")
	if err != nil {
		g.POS = nil
		g.posConn = nil
		return err
	}
	g.POS = pos.NewPOSServiceClient(posConn)
	g.posConn = posConn
	log.Printf("Successfully reconnected to POS service")
	return nil
}

func (g *GRPCClients) ReconnectCommissionsService() error {
	log.Printf("Attempting to reconnect to Commissions service...")
	if g.commissionConn != nil {
		g.commissionConn.Close()
	}

	commissionConn, err := connectToService("localhost:50054")
	if err != nil {
		g.Commissions = nil
		g.commissionConn = nil
		return err
	}
	g.Commissions = commissions.NewCommissionServiceClient(commissionConn)
	g.commissionConn = commissionConn
	log.Printf("Successfully reconnected to Commissions service")
	return nil
}

func NewGRPCClients() (*GRPCClients, error) {
	return nil, nil
}
