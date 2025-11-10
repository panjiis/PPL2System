package handler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	INVENTORY_CACHE_PREFIX     = "inventory:"
	INVENTORY_STOCKS_CACHE_KEY = "inventory:stocks"
	PRODUCTS_CACHE_KEY         = "inventory:products"
	WAREHOUSE_CACHE_KEY        = "inventory:warehouses"
	PRODUCTS_TYPE_CACHE_KEY    = "inventory:products-type"
	CACHE_TTL_SHORT            = 5 * time.Minute
	CACHE_TTL_MEDIUM           = 30 * time.Minute
	CACHE_TTL_LONG             = 2 * time.Hour
)

// -- PUB/Sub Related --

type Manager struct {
	Found       bool   `json:"found"`
	Error       string `json:"error,omitempty"`
	ManagerName string `json:"manager_name,omitempty"`
	Email       string `json:"email,omitempty"`
}

// Product sync events (Inventory → POS)
type ProductCreatedEvent struct {
	EventID     string    `json:"event_id"`
	Timestamp   time.Time `json:"timestamp"`
	ProductCode string    `json:"product_code"`
	ProductName string    `json:"product_name"`
	IsActive    bool      `json:"is_active"`
}

type ProductUpdatedEvent struct {
	EventID       string    `json:"event_id"`
	Timestamp     time.Time `json:"timestamp"`
	ProductCode   string    `json:"product_code"`
	ProductName   string    `json:"product_name"`
	IsActive      bool      `json:"is_active"`
	UpdatedFields []string  `json:"updated_fields"`
}

// Stock events (Inventory → POS, POS → Inventory)
type StockChangedEvent struct {
	EventID          string    `json:"event_id"`
	Timestamp        time.Time `json:"timestamp"`
	ProductCode      string    `json:"product_code"`
	WarehouseID      int32     `json:"warehouse_id"`
	PreviousQuantity int32     `json:"previous_quantity"`
	NewQuantity      int32     `json:"new_quantity"`
	ChangeReason     string    `json:"change_reason"` // "SALE", "RESTOCK", "ADJUSTMENT", "TRANSFER"
	TransactionID    *string   `json:"transaction_id,omitempty"`
}

// Sale events (POS → Inventory)
type SaleItem struct {
	ProductCode string `json:"product_code"`
	Quantity    int32  `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
}

type SaleCompletedEvent struct {
	EventID       string     `json:"event_id"`
	Timestamp     time.Time  `json:"timestamp"`
	TransactionID string     `json:"transaction_id"`
	DocumentID    int64      `json:"document_id"`
	WarehouseID   int32      `json:"warehouse_id"`
	Items         []SaleItem `json:"items"`
}

type SaleRefundedEvent struct {
	EventID       string     `json:"event_id"`
	Timestamp     time.Time  `json:"timestamp"`
	TransactionID string     `json:"transaction_id"`
	DocumentID    int64      `json:"document_id"`
	WarehouseID   int32      `json:"warehouse_id"`
	Items         []SaleItem `json:"items"`
}

type InventoryProduct struct {
	ProductCode   string `gorm:"size:100;primaryKey"`
	ProductName   string `gorm:"size:255"`
	ProductTypeID int32
	SupplierID    int32
	UnitOfMeasure string `gorm:"size:50"`
	ReorderLevel  int32
	MaxStockLevel int32
	CreatedAt     time.Time
	UpdatedAt     time.Time

	ProductType *ProductType `gorm:"foreignKey:ProductTypeID;references:ID"`
	Supplier    *Supplier    `gorm:"foreignKey:SupplierID"`
	Stocks      []Stock      `gorm:"foreignKey:ProductCode"`
}

type Warehouse struct {
	ID            int32   `gorm:"primaryKey"`
	WarehouseCode string  `gorm:"size:100;uniqueIndex"`
	WarehouseName string  `gorm:"size:255"`
	Location      *string `gorm:"size:255"`
	ManagerID     *int64
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Stocks []Stock `gorm:"foreignKey:WarehouseID"`

	ManagerName string `gorm:"-"`
}

type ProductType struct {
	ID              int32   `gorm:"primaryKey"`
	ProductTypeName string  `gorm:"size:100"`
	ProductTypeCode string  `gorm:"size:100;uniqueIndex"`
	LastProductId   int32   `gorm:"default:0"`
	Description     *string `gorm:"size:255"`
	CreatedAt       time.Time
	UpdatedAt       time.Time

	Products []InventoryProduct `gorm:"foreignKey:ProductTypeID;references:ID"`
}

type Supplier struct {
	ID            int32   `gorm:"primaryKey"`
	SupplierCode  string  `gorm:"size:100;uniqueIndex"`
	SupplierName  string  `gorm:"size:255"`
	ContactPerson *string `gorm:"size:100"`
	Phone         *string `gorm:"size:50"`
	Email         *string `gorm:"size:100"`
	Address       *string `gorm:"size:255"`
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Products []InventoryProduct `gorm:"foreignKey:SupplierID"`
}

type Stock struct {
	ID                int64 `gorm:"primaryKey"`
	ProductCode       string
	WarehouseID       int32
	AvailableQuantity int32
	ReservedQuantity  int32
	UnitCost          string  `gorm:"size:50"`
	LastRestockDate   *string `gorm:"size:50"`
	CreatedAt         time.Time
	UpdatedAt         time.Time

	Product   *InventoryProduct `gorm:"foreignKey:ProductCode;references:ProductCode"`
	Warehouse *Warehouse        `gorm:"foreignKey:WarehouseID;references:ID"`
}

type StockMovement struct {
	ID            int64 `gorm:"primaryKey"`
	ProductCode   string
	WarehouseID   int32
	MovementType  int32
	Quantity      int32
	UnitCost      *string `gorm:"size:50"`
	ReferenceType int32
	ReferenceID   *string `gorm:"size:100"`
	Notes         *string `gorm:"size:255"`
	CreatedBy     int64
	CreatedAt     time.Time

	ManagerName string `gorm:"-"`
}

func (c *InventoryHandler) SubscribeToEmployeeEvents(ctx context.Context) error {
	log.Println(("test"))
	_, err := c.nats.Subscribe("employee.>", func(msg *nats.Msg) {
		subject := msg.Subject
		log.Printf("Received employee event from subject: %s", subject)
	})
	if err != nil {
		return err
	}

	log.Println("Successfully subscribed to employee events")
	<-ctx.Done()
	return ctx.Err()
}

// -- NATS Events Handler --

// -- User Events Handler --
func (c *InventoryHandler) GetManagerDetails(ctx context.Context, managerID int64) (*Manager, error) {
	managerId, err := json.Marshal(map[string]interface{}{
		"manager_id": managerID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal request  %v", err)
	}

	msg, err := c.nats.RequestWithContext(ctx, "employee.manager.get", managerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to request manager data via NATS: %v", err)
	}

	var response struct {
		Found       bool   `json:"found"`
		Error       string `json:"error,omitempty"`
		ManagerName string `json:"manager_name,omitempty"`
		Email       string `json:"email,omitempty"`
	}

	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to parse manager response: %v", err)
	}

	if !response.Found {
		return nil, status.Errorf(codes.NotFound, "Manager with ID %d not found", managerID)
	}

	manager := &Manager{
		Found:       response.Found,
		Error:       response.Error,
		ManagerName: response.ManagerName,
		Email:       response.Email,
	}

	log.Println(manager.ManagerName)

	return manager, nil
}

func (c *InventoryHandler) GetManagerNamesBatch(ctx context.Context, managerIDs []int64) (map[int64]string, error) {
	managerNameMap := make(map[int64]string)

	for _, empID := range managerIDs {
		employee, err := c.GetManagerDetails(ctx, empID)
		if err != nil {
			continue
		}
		managerNameMap[empID] = employee.ManagerName
	}

	return managerNameMap, nil
}
