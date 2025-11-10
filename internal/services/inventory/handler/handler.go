package handler

import (
	"context"
	"fmt"
	"log"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/inventory"

	"github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type InventoryHandler struct {
	proto.UnimplementedInventoryServiceServer
	db    *gorm.DB
	redis *redis.Client
	nats  *nats.Conn
}

func NewInventoryHandler(db *gorm.DB, redisClient *redis.Client) *InventoryHandler {
	nc, err := nats.Connect("nats://10.147.17.76:4222",
		nats.ConnectHandler(func(nc *nats.Conn) {
			log.Printf("Inventory service connected to NATS at %v", nc.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(nc *nats.Conn) {
			log.Printf("Inventory service disconnected from NATS")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("Inventory service reconnected to NATS at %v", nc.ConnectedUrl())
		}),
	)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}

	handler := &InventoryHandler{
		db:    db,
		redis: redisClient,
		nats:  nc,
	}

	go func() {
		log.Println("Starting to listen for employee events...")
		if err := handler.SubscribeToEmployeeEvents(context.Background()); err != nil {
			log.Printf("Error subscribing to employee events: %v", err)
		}
	}()

	return handler
}

func (s *InventoryHandler) InvalidateInventoryCaches(ctx context.Context, ProductCode ...int32) {
	_ = s.redis.Del(ctx, INVENTORY_STOCKS_CACHE_KEY, PRODUCTS_CACHE_KEY, PRODUCTS_TYPE_CACHE_KEY, WAREHOUSE_CACHE_KEY)

	for _, id := range ProductCode {
		cacheKey := fmt.Sprintf("%s%d", INVENTORY_CACHE_PREFIX, id)
		_ = s.redis.Del(ctx, cacheKey)
	}
}

// --- Proto Conversions ---
func (s *InventoryHandler) inventoryProductsToProto(inventoryProduct InventoryProduct) *proto.InventoryProduct {
	protoProduct := &proto.InventoryProduct{
		ProductCode:   inventoryProduct.ProductCode,
		ProductName:   lib.StrPtr(inventoryProduct.ProductName),
		ProductTypeId: lib.Int32Ptr(inventoryProduct.ProductTypeID),
		SupplierId:    lib.Int32Ptr(inventoryProduct.SupplierID),
		UnitOfMeasure: lib.StrPtr(inventoryProduct.UnitOfMeasure),
		ReorderLevel:  lib.Int32Ptr(inventoryProduct.ReorderLevel),
		MaxStockLevel: lib.Int32Ptr(inventoryProduct.MaxStockLevel),
		CreatedAt:     timestamppb.New(lib.TimeNowOrZero(&inventoryProduct.CreatedAt)),
		UpdatedAt:     timestamppb.New(lib.TimeNowOrZero(&inventoryProduct.UpdatedAt)),
	}

	if inventoryProduct.ProductType != nil {
		protoProduct.ProductType = s.productTypeToProto(*inventoryProduct.ProductType)
	}

	if inventoryProduct.Supplier != nil {
		protoProduct.Supplier = s.supplierToProto(*inventoryProduct.Supplier)
	}

	if len(inventoryProduct.Stocks) > 0 {
		protoProduct.Stocks = make([]*proto.Stock, len(inventoryProduct.Stocks))
		for i, stock := range inventoryProduct.Stocks {
			protoProduct.Stocks[i] = s.stockToProto(stock)
		}
	}

	return protoProduct
}

func (s *InventoryHandler) productTypeToProto(productType ProductType) *proto.ProductType {
	return &proto.ProductType{
		Id:              productType.ID,
		ProductTypeName: lib.StrPtr(productType.ProductTypeName),
		Description:     productType.Description,
		CreatedAt:       timestamppb.New(lib.TimeNowOrZero(&productType.CreatedAt)),
		UpdatedAt:       timestamppb.New(lib.TimeNowOrZero(&productType.UpdatedAt)),
	}
}

func (s *InventoryHandler) supplierToProto(supplier Supplier) *proto.Supplier {
	protoSupplier := &proto.Supplier{
		Id:           supplier.ID,
		SupplierCode: supplier.SupplierCode,
		SupplierName: lib.StrPtr(supplier.SupplierName),
		IsActive:     supplier.IsActive,
		CreatedAt:    timestamppb.New(lib.TimeNowOrZero(&supplier.CreatedAt)),
		UpdatedAt:    timestamppb.New(lib.TimeNowOrZero(&supplier.UpdatedAt)),
	}

	if supplier.ContactPerson != nil {
		protoSupplier.ContactPerson = supplier.ContactPerson
	}
	if supplier.Phone != nil {
		protoSupplier.Phone = supplier.Phone
	}
	if supplier.Email != nil {
		protoSupplier.Email = supplier.Email
	}
	if supplier.Address != nil {
		protoSupplier.Address = supplier.Address
	}

	return protoSupplier
}

func (s *InventoryHandler) stockToProto(stock Stock) *proto.Stock {
	protoStock := &proto.Stock{
		ProductCode:       stock.ProductCode,
		WarehouseId:       stock.WarehouseID,
		AvailableQuantity: lib.Int32Ptr(stock.AvailableQuantity),
		ReservedQuantity:  lib.Int32Ptr(stock.ReservedQuantity),
		UnitCost:          stock.UnitCost,
		CreatedAt:         timestamppb.New(lib.TimeNowOrZero(&stock.CreatedAt)),
		UpdatedAt:         timestamppb.New(lib.TimeNowOrZero(&stock.UpdatedAt)),
	}

	if stock.LastRestockDate != nil {
		protoStock.LastRestockDate = stock.LastRestockDate
	}

	if stock.Product != nil {
		protoStock.Product = s.inventoryProductsToProto(*stock.Product)
	}
	if stock.Warehouse != nil {
		protoStock.Warehouse = s.warehouseToProto(*stock.Warehouse)
	}

	return protoStock
}

func (s *InventoryHandler) warehouseToProto(warehouse Warehouse) *proto.Warehouse {
	protoWarehouse := &proto.Warehouse{
		Id:            warehouse.ID,
		WarehouseCode: warehouse.WarehouseCode,
		WarehouseName: lib.StrPtr(warehouse.WarehouseName),
		IsActive:      warehouse.IsActive,
		CreatedAt:     timestamppb.New(lib.TimeNowOrZero(&warehouse.CreatedAt)),
		UpdatedAt:     timestamppb.New(lib.TimeNowOrZero(&warehouse.UpdatedAt)),
	}

	if warehouse.Location != nil {
		protoWarehouse.Location = warehouse.Location
	}
	if warehouse.ManagerID != nil {
		protoWarehouse.ManagerId = warehouse.ManagerID
	}
	if warehouse.ManagerName != "" {
		protoWarehouse.ManagerName = warehouse.ManagerName
	}

	return protoWarehouse
}

func (s *InventoryHandler) movementToProto(movement StockMovement) *proto.StockMovement {
	protoMovement := &proto.StockMovement{
		ProductCode:   movement.ProductCode,
		WarehouseId:   movement.WarehouseID,
		MovementType:  proto.MovementType(movement.MovementType),
		Quantity:      movement.Quantity,
		ReferenceType: proto.ReferenceType(movement.ReferenceType),
		CreatedBy:     movement.CreatedBy,
		CreatedAt:     timestamppb.New(movement.CreatedAt),
	}

	if movement.ReferenceID != nil {
		protoMovement.ReferenceId = movement.ReferenceID
	}
	if movement.Notes != nil {
		protoMovement.Notes = movement.Notes
	}
	if movement.UnitCost != nil {
		protoMovement.UnitCost = movement.UnitCost
	}
	if movement.ManagerName != "" {
		protoMovement.ManagerName = lib.StrPtr(movement.ManagerName)
	}
	return protoMovement
}
