package handler

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	proto "syntra-system/proto/protogen/inventory"
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

// --- Helpers ---
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: %v", value)
	}
	return json.Unmarshal(bytes, a)
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	return json.Marshal(a)
}

func timeNowOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

type InventoryProduct struct {
	ID            int32  `gorm:"primaryKey"`
	ProductCode   string `gorm:"size:100;uniqueIndex"`
	ProductName   string `gorm:"size:255"`
	ProductTypeID int32
	SupplierID    int32
	UnitOfMeasure string `gorm:"size:50"`
	ReorderLevel  int32
	MaxStockLevel int32
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	ProductType *ProductType `gorm:"foreignKey:ProductTypeID"`
	Supplier    *Supplier    `gorm:"foreignKey:SupplierID"`
	Stocks      []Stock      `gorm:"foreignKey:ProductID"`
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
}

type ProductType struct {
	ID              int32   `gorm:"primaryKey"`
	ProductTypeName string  `gorm:"size:100"`
	Description     *string `gorm:"size:255"`
	CreatedAt       time.Time
	UpdatedAt       time.Time

	Products []InventoryProduct `gorm:"foreignKey:ProductTypeID"`
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
	ProductID         int32
	WarehouseID       int32
	AvailableQuantity int32
	ReservedQuantity  int32
	UnitCost          string  `gorm:"size:50"`
	LastRestockDate   *string `gorm:"size:50"`
	CreatedAt         time.Time
	UpdatedAt         time.Time

	Product   *InventoryProduct `gorm:"foreignKey:ProductID"`
	Warehouse *Warehouse        `gorm:"foreignKey:WarehouseID"`
}

type StockMovement struct {
	ID            int64 `gorm:"primaryKey"`
	ProductID     int32
	WarehouseID   int32
	MovementType  int32
	Quantity      int32
	UnitCost      *string `gorm:"size:50"`
	ReferenceType int32
	ReferenceID   *string `gorm:"size:100"`
	Notes         *string `gorm:"size:255"`
	CreatedBy     int64
	CreatedAt     time.Time
}

// --- Handler ---

type InventoryHandler struct {
	proto.UnimplementedInventoryServiceServer
	db    *gorm.DB
	redis *redis.Client
}

func NewInventoryHandler(db *gorm.DB, redisClient *redis.Client) *InventoryHandler {
	return &InventoryHandler{
		db:    db,
		redis: redisClient,
	}
}

func (s *InventoryHandler) InvalidateInventoryCaches(ctx context.Context, productID ...int32) {
	_ = s.redis.Del(ctx, INVENTORY_STOCKS_CACHE_KEY, PRODUCTS_CACHE_KEY, PRODUCTS_TYPE_CACHE_KEY, WAREHOUSE_CACHE_KEY)

	for _, id := range productID {
		cacheKey := fmt.Sprintf("%s%d", INVENTORY_CACHE_PREFIX, id)
		_ = s.redis.Del(ctx, cacheKey)
	}
}

// --- Proto Conversions ---
func (s *InventoryHandler) inventoryProductsToProto(inventoryProduct InventoryProduct) *proto.InventoryProduct {
	protoProduct := &proto.InventoryProduct{
		Id:            inventoryProduct.ID,
		ProductCode:   inventoryProduct.ProductCode,
		ProductName:   inventoryProduct.ProductName,
		ProductTypeId: inventoryProduct.ProductTypeID,
		SupplierId:    inventoryProduct.SupplierID,
		UnitOfMeasure: inventoryProduct.UnitOfMeasure,
		ReorderLevel:  inventoryProduct.ReorderLevel,
		MaxStockLevel: inventoryProduct.MaxStockLevel,
		IsActive:      inventoryProduct.IsActive,
		CreatedAt:     timestamppb.New(timeNowOrZero(&inventoryProduct.CreatedAt)),
		UpdatedAt:     timestamppb.New(timeNowOrZero(&inventoryProduct.UpdatedAt)),
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
		ProductTypeName: productType.ProductTypeName,
		Description:     productType.Description,
		CreatedAt:       timestamppb.New(timeNowOrZero(&productType.CreatedAt)),
		UpdatedAt:       timestamppb.New(timeNowOrZero(&productType.UpdatedAt)),
	}
}

func (s *InventoryHandler) supplierToProto(supplier Supplier) *proto.Supplier {
	protoSupplier := &proto.Supplier{
		Id:           supplier.ID,
		SupplierCode: supplier.SupplierCode,
		SupplierName: supplier.SupplierName,
		IsActive:     supplier.IsActive,
		CreatedAt:    timestamppb.New(timeNowOrZero(&supplier.CreatedAt)),
		UpdatedAt:    timestamppb.New(timeNowOrZero(&supplier.UpdatedAt)),
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
		Id:                stock.ID,
		ProductId:         stock.ProductID,
		WarehouseId:       stock.WarehouseID,
		AvailableQuantity: stock.AvailableQuantity,
		ReservedQuantity:  stock.ReservedQuantity,
		UnitCost:          stock.UnitCost,
		CreatedAt:         timestamppb.New(timeNowOrZero(&stock.CreatedAt)),
		UpdatedAt:         timestamppb.New(timeNowOrZero(&stock.UpdatedAt)),
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
		WarehouseName: warehouse.WarehouseName,
		IsActive:      warehouse.IsActive,
		CreatedAt:     timestamppb.New(timeNowOrZero(&warehouse.CreatedAt)),
		UpdatedAt:     timestamppb.New(timeNowOrZero(&warehouse.UpdatedAt)),
	}

	if warehouse.Location != nil {
		protoWarehouse.Location = warehouse.Location
	}
	if warehouse.ManagerID != nil {
		protoWarehouse.ManagerId = warehouse.ManagerID
	}

	return protoWarehouse
}

func (s *InventoryHandler) movementToProto(movement StockMovement) *proto.StockMovement {
	protoMovement := &proto.StockMovement{
		Id:            movement.ID,
		ProductId:     movement.ProductID,
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

	return protoMovement
}

// -- Inventory Products --

func (s *InventoryHandler) CreateInventoryProduct(ctx context.Context, req *proto.CreateProductRequest) (*proto.CreateProductResponse, error) {
	var product InventoryProduct
	if req.GetProductCode() == "" || req.GetProductName() == "" {
		return &proto.CreateProductResponse{
			Success: false,
			Message: strPtr("Product Code and Product Name is Required"),
		}, nil
	}

	product = InventoryProduct{
		ProductCode:   req.GetProductCode(),
		ProductName:   req.GetProductName(),
		ProductTypeID: req.GetProductTypeId(),
		SupplierID:    req.GetSupplierId(),
		UnitOfMeasure: req.GetUnitOfMeasure(),
		ReorderLevel:  req.GetReorderLevel(),
		MaxStockLevel: req.GetMaxStockLevel(),
	}

	if err := s.db.Create(&product).Error; err != nil {
		return &proto.CreateProductResponse{
			Success: false,
			Message: strPtr("error creating Product"),
		}, err
	}

	_ = s.redis.Del(ctx, PRODUCTS_CACHE_KEY)

	return &proto.CreateProductResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}

func (s *InventoryHandler) UpdateInventoryProduct(ctx context.Context, req *proto.UpdateProductRequest) (*proto.UpdateProductResponse, error) {
	var product InventoryProduct

	if req.GetId() == 0 {
		return &proto.UpdateProductResponse{
			Success: false,
			Message: strPtr("Id must be provided"),
		}, nil
	}

	if err := s.db.Preload("InventoryProducts").First(&product, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.UpdateProductResponse{
				Success: false,
				Message: strPtr("Products not found"),
			}, err
		}
		return &proto.UpdateProductResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	if req.ProductName != nil {
		product.ProductName = *req.ProductName
	}
	if req.ProductTypeId != nil {
		product.ProductTypeID = *req.ProductTypeId
	}
	if req.SupplierId != nil {
		product.SupplierID = *req.SupplierId
	}
	if req.ReorderLevel != nil {
		product.ReorderLevel = *req.ReorderLevel
	}
	if req.UnitOfMeasure != nil {
		product.UnitOfMeasure = *req.UnitOfMeasure
	}
	if req.MaxStockLevel != nil {
		product.MaxStockLevel = *req.MaxStockLevel
	}
	if req.IsActive != nil {
		product.IsActive = *req.IsActive
	}

	if err := s.db.Save(&product).Error; err != nil {
		return &proto.UpdateProductResponse{
			Success: false,
			Message: strPtr("error updating products"),
		}, err
	}

	s.InvalidateInventoryCaches(ctx)

	return &proto.UpdateProductResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}

func (s *InventoryHandler) ListProduct(ctx context.Context, req *proto.ListProductsRequest) (*proto.ListProductsResponse, error) {
	var products []InventoryProduct
	var total int64

	query := s.db.Model(&InventoryProduct{}).Preload("ProductType").Preload("Supplier").Preload("Stocks")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.ProductTypeId != nil {
		query = query.Where("product_type_id = ?", req.GetProductTypeId())
	}
	if req.SupplierId != nil {
		query = query.Where("supplier_id = ?", req.GetSupplierId())
	}
	if req.SearchTerm != nil {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		query = query.Where(
			"product_code ILIKE ? OR product_name ILIKE ? OR unit_of_measure ILIKE ?",
			searchTerm, searchTerm, searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoProducts := make([]*proto.InventoryProduct, len(products))
	for i, prod := range products {
		protoProducts[i] = s.inventoryProductsToProto(prod)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListProductsResponse{
		Success:  true,
		Products: protoProducts,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

func (s *InventoryHandler) GetProductsById(ctx context.Context, req *proto.GetProductRequest) (*proto.GetProductResponse, error) {
	var product InventoryProduct

	if req.GetId() == 0 {
		return &proto.GetProductResponse{
			Success: false,
			Message: strPtr("Id must be provided"),
		}, nil
	}

	if err := s.db.Preload("Supplier").Preload("ProductType").Preload("Stocks").First(&product, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetProductResponse{
				Success: false,
				Message: strPtr("Products not Found"),
			}, err
		}
		return &proto.GetProductResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	return &proto.GetProductResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}

func (s *InventoryHandler) GetProductsByCode(ctx context.Context, req *proto.GetProductByCodeRequest) (*proto.GetProductByCodeResponse, error) {
	var product InventoryProduct

	if req.GetProductCode() == "" {
		return &proto.GetProductByCodeResponse{
			Success: false,
			Message: strPtr("Id must be provided"),
		}, nil
	}

	if err := s.db.Preload("Supplier").Preload("ProductType").Preload("Stocks").Where("product_code = ?", req.GetProductCode()).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetProductByCodeResponse{
				Success: false,
				Message: strPtr("Products not Found"),
			}, err
		}
		return &proto.GetProductByCodeResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	return &proto.GetProductByCodeResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}

// -- Stocks --
func (s *InventoryHandler) CheckStock(ctx context.Context, req *proto.CheckStockRequest) (*proto.CheckStockResponse, error) {
	var stocks []Stock

	if req.GetProductId() == 0 {
		return &proto.CheckStockResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}

	if err := s.db.Preload("Warehouse").Where("product_id = ?", req.GetProductId()).Find(&stocks).Error; err != nil {
		return &proto.CheckStockResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	if len(stocks) == 0 {
		return &proto.CheckStockResponse{
			Success:                true,
			IsAvailable:            false,
			TotalAvailableQuantity: 0,
			StockDetails:           []*proto.Stock{},
			Message:                strPtr("No stocks found for this product"),
		}, nil
	}

	var totalAvailable int32
	var stockDetails []*proto.Stock

	for _, stock := range stocks {
		totalAvailable += stock.AvailableQuantity

		protoStock := &proto.Stock{
			Id:                stock.ID,
			ProductId:         stock.ProductID,
			WarehouseId:       stock.WarehouseID,
			AvailableQuantity: stock.AvailableQuantity,
			ReservedQuantity:  stock.ReservedQuantity,
			UnitCost:          stock.UnitCost,
		}

		if stock.LastRestockDate != nil {
			protoStock.LastRestockDate = stock.LastRestockDate
		}

		if stock.Warehouse != nil {
			protoStock.Warehouse.WarehouseName = stock.Warehouse.WarehouseName
		}

		stockDetails = append(stockDetails, protoStock)
	}

	return &proto.CheckStockResponse{
		Success:                true,
		IsAvailable:            totalAvailable > 0,
		TotalAvailableQuantity: totalAvailable,
		StockDetails:           stockDetails,
	}, nil
}

func (s *InventoryHandler) ReserveStock(ctx context.Context, req *proto.ReserveStockRequest) (*proto.ReserveStockResponse, error) {
	if req.GetProductId() == 0 {
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}
	if req.GetWarehouseId() == 0 {
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr("warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr("quantity must be greater than 0"),
		}, nil
	}

	var stock Stock

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("product_id = ? AND warehouse_id = ?", req.GetProductId(), req.GetWarehouseId()).
		First(&stock).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.ReserveStockResponse{
				Success: false,
				Message: strPtr("Stock not found for this product and warehouse"),
			}, nil
		}
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if stock.AvailableQuantity < req.GetQuantity() {
		tx.Rollback()
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr(fmt.Sprintf("Insufficient stock. Available: %d, Requested: %d",
				stock.AvailableQuantity, req.GetQuantity())),
		}, nil
	}

	stock.AvailableQuantity -= req.GetQuantity()
	stock.ReservedQuantity += req.GetQuantity()
	stock.UpdatedAt = time.Now()

	if err := tx.Save(&stock).Error; err != nil {
		tx.Rollback()
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr("Failed to update stock"),
		}, err
	}

	referenceId := req.GetReferenceId()
	movement := StockMovement{
		ProductID:     req.GetProductId(),
		WarehouseID:   req.GetWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_ADJUSTMENT),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_ADJUSTMENT),
		ReferenceID:   &referenceId,
		CreatedBy:     req.GetReservedBy(),
		CreatedAt:     time.Now(),
	}

	if err := tx.Create(&movement).Error; err != nil {
		tx.Rollback()
		return &proto.ReserveStockResponse{
			Success: false,
			Message: strPtr("Failed to create stock movement record"),
		}, err
	}

	tx.Commit()

	protoStock := s.stockToProto(stock)

	return &proto.ReserveStockResponse{
		UpdatedStock: protoStock,
		Success:      true,
	}, nil
}

func (s *InventoryHandler) ReleaseStock(ctx context.Context, req *proto.ReleaseStockRequest) (*proto.ReleaseStockResponse, error) {
	if req.GetProductId() == 0 {
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}
	if req.GetWarehouseId() == 0 {
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr("warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr("quantity must be greater than 0"),
		}, nil
	}

	var stock Stock

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("product_id = ? AND warehouse_id = ?", req.GetProductId(), req.GetWarehouseId()).
		First(&stock).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.ReleaseStockResponse{
				Success: false,
				Message: strPtr("Stock not found for this product and warehouse"),
			}, nil
		}
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if stock.ReservedQuantity < req.GetQuantity() {
		tx.Rollback()
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr(fmt.Sprintf("Insufficient reserved stock. Reserved: %d, Requested: %d",
				stock.ReservedQuantity, req.GetQuantity())),
		}, nil
	}

	stock.ReservedQuantity -= req.GetQuantity()
	stock.AvailableQuantity += req.GetQuantity()
	stock.UpdatedAt = time.Now()

	if err := tx.Save(&stock).Error; err != nil {
		tx.Rollback()
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr("Failed to update stock"),
		}, err
	}

	referenceId := req.GetReferenceId()
	movement := StockMovement{
		ProductID:     req.GetProductId(),
		WarehouseID:   req.GetWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_ADJUSTMENT),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_ADJUSTMENT),
		ReferenceID:   &referenceId,
		CreatedBy:     req.GetReleasedBy(),
		CreatedAt:     time.Now(),
	}

	if err := tx.Create(&movement).Error; err != nil {
		tx.Rollback()
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: strPtr("Failed to create stock movement record"),
		}, err
	}

	tx.Commit()

	protoStock := s.stockToProto(stock)

	return &proto.ReleaseStockResponse{
		UpdatedStock: protoStock,
		Success:      true,
	}, nil
}

func (s *InventoryHandler) UpdateStock(ctx context.Context, req *proto.UpdateStockRequest) (*proto.UpdateStockResponse, error) {
	if req.GetProductId() == 0 {
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}
	if req.GetWarehouseId() == 0 {
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("quantity must be greater than 0"),
		}, nil
	}

	var stock Stock

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	result := tx.Where("product_id = ? AND warehouse_id = ?", req.GetProductId(), req.GetWarehouseId()).
		First(&stock)

	if result.Error == gorm.ErrRecordNotFound {
		stock = Stock{
			ProductID:         req.GetProductId(),
			WarehouseID:       req.GetWarehouseId(),
			AvailableQuantity: 0,
			ReservedQuantity:  0,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
		if req.UnitCost != nil {
			stock.UnitCost = *req.UnitCost
		}
	} else if result.Error != nil {
		tx.Rollback()
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, result.Error
	}

	switch req.GetMovementType() {
	case proto.MovementType_MOVEMENT_TYPE_IN:
		stock.AvailableQuantity += req.GetQuantity()
		if req.UnitCost != nil {
			stock.UnitCost = *req.UnitCost
		}
		restockDate := time.Now().Format("2006-01-02")
		stock.LastRestockDate = &restockDate
	case proto.MovementType_MOVEMENT_TYPE_OUT:
		if stock.AvailableQuantity < req.GetQuantity() {
			tx.Rollback()
			return &proto.UpdateStockResponse{
				Success: false,
				Message: strPtr(fmt.Sprintf("Insufficient stock. Available: %d, Requested: %d",
					stock.AvailableQuantity, req.GetQuantity())),
			}, nil
		}
		stock.AvailableQuantity -= req.GetQuantity()
	case proto.MovementType_MOVEMENT_TYPE_ADJUSTMENT:
		stock.AvailableQuantity += req.GetQuantity()
		if stock.AvailableQuantity < 0 {
			tx.Rollback()
			return &proto.UpdateStockResponse{
				Success: false,
				Message: strPtr("Adjustment would result in negative stock"),
			}, nil
		}
	default:
		tx.Rollback()
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("Invalid movement type"),
		}, nil
	}

	stock.UpdatedAt = time.Now()

	if err := tx.Save(&stock).Error; err != nil {
		tx.Rollback()
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("Failed to update stock"),
		}, err
	}

	movement := StockMovement{
		ProductID:     req.GetProductId(),
		WarehouseID:   req.GetWarehouseId(),
		MovementType:  int32(req.GetMovementType()),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(req.GetReferenceType()),
		CreatedBy:     req.GetCreatedBy(),
		CreatedAt:     time.Now(),
	}

	if req.ReferenceId != nil {
		movement.ReferenceID = req.ReferenceId
	}
	if req.Notes != nil {
		movement.Notes = req.Notes
	}
	if req.UnitCost != nil {
		movement.UnitCost = req.UnitCost
	}

	if err := tx.Create(&movement).Error; err != nil {
		tx.Rollback()
		return &proto.UpdateStockResponse{
			Success: false,
			Message: strPtr("Failed to create stock movement record"),
		}, err
	}

	tx.Commit()

	protoStock := s.stockToProto(stock)
	protoMovement := s.movementToProto(movement)

	return &proto.UpdateStockResponse{
		StockMovement: protoMovement,
		UpdatedStock:  protoStock,
		Success:       true,
	}, nil
}

func (s *InventoryHandler) GetStock(ctx context.Context, req *proto.GetStockRequest) (*proto.GetStockResponse, error) {
	if req.GetProductId() == 0 {
		return &proto.GetStockResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}

	var stocks []Stock
	query := s.db.Preload("Warehouse").Where("product_id = ?", req.GetProductId())

	if req.WarehouseId != nil && *req.WarehouseId != 0 {
		query = query.Where("warehouse_id = ?", *req.WarehouseId)
	}

	if err := query.Find(&stocks).Error; err != nil {
		return &proto.GetStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	var protoStocks []*proto.Stock
	for _, stock := range stocks {
		protoStocks = append(protoStocks, s.stockToProto(stock))
	}

	return &proto.GetStockResponse{
		Stocks:  protoStocks,
		Success: true,
	}, nil
}

func (s *InventoryHandler) ListLowStock(ctx context.Context, req *proto.ListLowStockRequest) (*proto.ListLowStockResponse, error) {
	var stocks []Stock

	query := s.db.Preload("Warehouse").Preload("Product")

	if req.WarehouseId != nil && *req.WarehouseId != 0 {
		query = query.Where("warehouse_id = ?", *req.WarehouseId)
	}

	query = query.Where("available_quantity <= ?", 10)
	pageSize := int32(50)
	pageToken := ""

	if req.Pagination != nil {
		if req.Pagination.GetPageSize() > 0 {
			pageSize = req.Pagination.GetPageSize()
		}
		pageToken = req.Pagination.GetPageToken()
	}

	offset := int32(0)
	if pageToken != "" {
	}

	var totalCount int64
	countQuery := s.db.Model(&Stock{})
	if req.WarehouseId != nil && *req.WarehouseId != 0 {
		countQuery = countQuery.Where("warehouse_id = ?", *req.WarehouseId)
	}
	countQuery = countQuery.Where("available_quantity <= ?", 10)

	if err := countQuery.Count(&totalCount).Error; err != nil {
		return &proto.ListLowStockResponse{
			Success: false,
			Message: strPtr("Failed to count records"),
		}, err
	}

	if err := query.Offset(int(offset)).Limit(int(pageSize)).Find(&stocks).Error; err != nil {
		return &proto.ListLowStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	var protoStocks []*proto.Stock
	for _, stock := range stocks {
		protoStocks = append(protoStocks, s.stockToProto(stock))
	}

	nextPageToken := ""
	if int32(len(stocks)) == pageSize && int64(offset+pageSize) < totalCount {
		nextPageToken = fmt.Sprintf("%d", offset+pageSize)
	}

	paginationResponse := &proto.PaginationResponse{
		NextPageToken: nextPageToken,
		TotalCount:    int32(totalCount),
	}

	return &proto.ListLowStockResponse{
		LowStocks:  protoStocks,
		Pagination: paginationResponse,
		Success:    true,
	}, nil
}

func (s *InventoryHandler) TransferStock(ctx context.Context, req *proto.TransferStockRequest) (*proto.TransferStockResponse, error) {
	if req.GetProductId() == 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("product_id required"),
		}, nil
	}
	if req.GetFromWarehouseId() == 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("from_warehouse_id required"),
		}, nil
	}
	if req.GetToWarehouseId() == 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("to_warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("quantity must be greater than 0"),
		}, nil
	}
	if req.GetFromWarehouseId() == req.GetToWarehouseId() {
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("cannot transfer to the same warehouse"),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var fromStock, toStock Stock

	if err := tx.Where("product_id = ? AND warehouse_id = ?",
		req.GetProductId(), req.GetFromWarehouseId()).First(&fromStock).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.TransferStockResponse{
				Success: false,
				Message: strPtr("Source stock not found"),
			}, nil
		}
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	if fromStock.AvailableQuantity < req.GetQuantity() {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr(fmt.Sprintf("Insufficient stock in source warehouse. Available: %d, Requested: %d",
				fromStock.AvailableQuantity, req.GetQuantity())),
		}, nil
	}

	result := tx.Where("product_id = ? AND warehouse_id = ?",
		req.GetProductId(), req.GetToWarehouseId()).First(&toStock)

	if result.Error == gorm.ErrRecordNotFound {
		toStock = Stock{
			ProductID:         req.GetProductId(),
			WarehouseID:       req.GetToWarehouseId(),
			AvailableQuantity: 0,
			ReservedQuantity:  0,
			UnitCost:          fromStock.UnitCost,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
	} else if result.Error != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, result.Error
	}

	fromStock.AvailableQuantity -= req.GetQuantity()
	fromStock.UpdatedAt = time.Now()

	toStock.AvailableQuantity += req.GetQuantity()
	toStock.UpdatedAt = time.Now()

	if err := tx.Save(&fromStock).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("Failed to update source stock"),
		}, err
	}

	if err := tx.Save(&toStock).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("Failed to update destination stock"),
		}, err
	}

	transferRefId := fmt.Sprintf("TRANSFER_%d_%d_%d", req.GetProductId(), req.GetFromWarehouseId(), time.Now().Unix())

	outMovement := StockMovement{
		ProductID:     req.GetProductId(),
		WarehouseID:   req.GetFromWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_TRANSFER),
		Quantity:      -req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_TRANSFER),
		ReferenceID:   &transferRefId,
		CreatedBy:     req.GetTransferredBy(),
		CreatedAt:     time.Now(),
	}

	inMovement := StockMovement{
		ProductID:     req.GetProductId(),
		WarehouseID:   req.GetToWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_TRANSFER),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_TRANSFER),
		ReferenceID:   &transferRefId,
		CreatedBy:     req.GetTransferredBy(),
		CreatedAt:     time.Now(),
	}

	if req.Notes != nil {
		outMovement.Notes = req.Notes
		inMovement.Notes = req.Notes
	}

	if err := tx.Create(&outMovement).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("Failed to create outbound movement record"),
		}, err
	}

	if err := tx.Create(&inMovement).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: strPtr("Failed to create inbound movement record"),
		}, err
	}

	tx.Commit()

	protoOutMovement := s.movementToProto(outMovement)
	protoInMovement := s.movementToProto(inMovement)

	return &proto.TransferStockResponse{
		StockMovements:   []*proto.StockMovement{protoOutMovement, protoInMovement},
		SourceStock:      s.stockToProto(fromStock),
		DestinationStock: s.stockToProto(toStock),
		Success:          true,
		Message:          strPtr("Stock transferred successfully"),
	}, nil
}

// -- Stock Movement --
func (s *InventoryHandler) ListStockMovements(ctx context.Context, req *proto.ListStockMovementsRequest) (*proto.ListStockMovementsResponse, error) {
	var stockMovements []StockMovement
	var total int64

	query := s.db.Model(&StockMovement{})

	if req.ProductId != nil && *req.ProductId != 0 {
		query = query.Where("product_id = ?", *req.ProductId)
	}

	if req.WarehouseId != nil && *req.WarehouseId != 0 {
		query = query.Where("warehouse_id = ?", *req.WarehouseId)
	}

	if req.MovementType != nil && *req.MovementType != proto.MovementType_MOVEMENT_TYPE_UNSPECIFIED {
		query = query.Where("movement_type = ?", int32(*req.MovementType))
	}

	if req.DateRange != nil {
		if req.DateRange.StartDate != "" {
			startDate, err := time.Parse("2006-01-02", req.DateRange.StartDate)
			if err == nil {
				query = query.Where("created_at >= ?", startDate)
			}
		}
		if req.DateRange.EndDate != "" {
			endDate, err := time.Parse("2006-01-02", req.DateRange.EndDate)
			if err == nil {
				endDate = endDate.Add(24 * time.Hour)
				query = query.Where("created_at < ?", endDate)
			}
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListStockMovementsResponse{
			Success: false,
			Message: strPtr("Failed to count stock movements"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize

	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&stockMovements).Error; err != nil {
		return &proto.ListStockMovementsResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	protoMovements := make([]*proto.StockMovement, len(stockMovements))
	for i, movement := range stockMovements {
		protoMovements[i] = s.movementToProto(movement)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListStockMovementsResponse{
		Success:        true,
		StockMovements: protoMovements,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// -- Warehouse --
func (s *InventoryHandler) CreateWarehouse(ctx context.Context, req *proto.CreateWarehouseRequest) (*proto.CreateWarehouseResponse, error) {
	var warehouse Warehouse
	if req.GetWarehouseCode() == "" || req.GetWarehouseName() == "" {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: strPtr("Warehouse code and name required"),
		}, nil
	}

	warehouse = Warehouse{
		WarehouseCode: req.GetWarehouseCode(),
		WarehouseName: req.GetWarehouseName(),
		Location:      strPtr(req.GetLocation()),
	}
	managerId := req.GetManagerId()
	warehouse.ManagerID = &managerId

	if err := s.db.Create(&warehouse).Error; err != nil {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: strPtr("error creating Product"),
		}, err
	}

	_ = s.redis.Del(ctx, WAREHOUSE_CACHE_KEY)

	return &proto.CreateWarehouseResponse{
		Success:   true,
		Warehouse: s.warehouseToProto(warehouse),
	}, nil
}
func (s *InventoryHandler) GetWarehouse(ctx context.Context, req *proto.GetWarehouseRequest) (*proto.GetWarehouseResponse, error) {
	var warehouse Warehouse

	if req.GetWarehouseCode() == "" {
		return &proto.GetWarehouseResponse{
			Success: false,
			Message: strPtr("warehouse_coderequired"),
		}, nil
	}

	if err := s.db.Where("warehouse_code = ?", req.GetWarehouseCode()).First(&warehouse).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetWarehouseResponse{
				Success: false,
				Message: strPtr("Warehouse not found"),
			}, nil
		}
		return &proto.GetWarehouseResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	return &proto.GetWarehouseResponse{
		Success:   true,
		Warehouse: s.warehouseToProto(warehouse),
	}, nil
}

func (s *InventoryHandler) ListWarehouse(ctx context.Context, req *proto.ListWarehousesRequest) (*proto.ListWarehousesResponse, error) {
	var warehouse []Warehouse
	var total int64

	query := s.db.Model(&Warehouse{})

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.WarehouseCode != nil {
		query = query.Where("warehouse_code = ?", req.GetWarehouseCode())
	}
	if req.WarehouseName != nil {
		query = query.Where("warehouse_name = ?", req.GetWarehouseName())
	}
	if req.SearchTerm != nil {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		query = query.Where(
			"warehouse_code ILIKE ? OR warehouse_name ILIKE ?",
			searchTerm, searchTerm, searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListWarehousesResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&warehouse).Error; err != nil {
		return &proto.ListWarehousesResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoWarehouse := make([]*proto.Warehouse, len(warehouse))
	for i, wh := range warehouse {
		protoWarehouse[i] = s.warehouseToProto(wh)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListWarehousesResponse{
		Success:    true,
		Warehouses: protoWarehouse,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// -- Suppliers --

func (s *InventoryHandler) CreateSupplier(ctx context.Context, req *proto.CreateSupplierRequest) (*proto.CreateSupplierResponse, error) {
	var supplier Supplier
	if req.GetSupplierCode() == "" || req.GetSupplierName() == "" {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: strPtr("Supplier Code and Name Must be Provided"),
		}, nil
	}

	supplier = Supplier{
		SupplierCode:  req.GetSupplierCode(),
		SupplierName:  req.GetSupplierName(),
		ContactPerson: req.ContactPerson,
		Phone:         req.Phone,
		Email:         strPtr(req.GetEmail()),
		Address:       strPtr(req.GetAddress()),
	}

	if err := s.db.Create(&supplier).Error; err != nil {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: strPtr("Error while creating Supplier"),
		}, err
	}

	return &proto.CreateSupplierResponse{
		Success:  true,
		Supplier: s.supplierToProto(supplier),
	}, nil
}

func (s *InventoryHandler) GetSupplier(ctx context.Context, req *proto.GetSupplierRequest) (*proto.GetSupplierResponse, error) {
	var supplier Supplier

	if req.GetId() == 0 {
		return &proto.GetSupplierResponse{
			Success: false,
			Message: strPtr("Supplier ID needed"),
		}, nil
	}

	if err := s.db.Where("id = ?", req.GetId()).First(&supplier).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetSupplierResponse{
				Success: false,
				Message: strPtr("Supplier not found"),
			}, nil
		}
		return &proto.GetSupplierResponse{
			Success: false,
			Message: strPtr("Database error"),
		}, err
	}

	return &proto.GetSupplierResponse{
		Success:  true,
		Supplier: s.supplierToProto(supplier),
	}, nil
}

func (s *InventoryHandler) ListSupplier(ctx context.Context, req *proto.ListSuppliersRequest) (*proto.ListSuppliersResponse, error) {
	var suppliers []Supplier
	var total int64

	query := s.db.Model(&Warehouse{})

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.SupplierCode != nil {
		query = query.Where("supplier_code = ?", req.GetSupplierCode())
	}
	if req.SupplierName != nil {
		query = query.Where("supplier_name = ?", req.GetSupplierName())
	}
	if req.SearchTerm != nil {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		query = query.Where(
			"supplier_code ILIKE ? OR supplier_name ILIKE ? OR contact_person ILIKE ? OR phone ILIKE ? OR email ILIKE ? OR address ILIKE ?",
			searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListSuppliersResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&suppliers).Error; err != nil {
		return &proto.ListSuppliersResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoSupplier := make([]*proto.Supplier, len(suppliers))
	for i, spl := range suppliers {
		protoSupplier[i] = s.supplierToProto(spl)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListSuppliersResponse{
		Success:   true,
		Suppliers: protoSupplier,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// -- Product Type --

func (s *InventoryHandler) CreateProductType(ctx context.Context, req *proto.CreateProductTypeRequest) (*proto.CreateProductTypeResponse, error) {
	var productType ProductType
	if req.GetProductTypeName() == "" {
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: strPtr("Product Type Name neeeded"),
		}, nil
	}

	productType = ProductType{
		ProductTypeName: req.GetProductTypeName(),
		Description:     strPtr(req.GetDescription()),
	}

	if err := s.db.Create(&productType).Error; err != nil {
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: strPtr("Failed to Create Product Type"),
		}, err
	}

	return &proto.CreateProductTypeResponse{
		Success:     true,
		ProductType: s.productTypeToProto(productType),
	}, nil
}

func (s *InventoryHandler) ListProductType(ctx context.Context, req *proto.ListProductTypesRequest) (*proto.ListProductTypesResponse, error) {
	var productTypes []ProductType
	var total int64

	query := s.db.Model(&Warehouse{})

	if req.SearchTerm != nil {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		query = query.Where(
			"product_type_name ILIKE ?",
			searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductTypesResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if token := req.GetPagination().GetPageToken(); token != "" {
		if n, err := strconv.Atoi(token); err == nil && n > 0 {
			pageNumber = n
		}
	}

	offset := (pageNumber - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&productTypes).Error; err != nil {
		return &proto.ListProductTypesResponse{
			Success: false,
			Message: strPtr("database error"),
		}, err
	}

	protoProductType := make([]*proto.ProductType, len(productTypes))
	for i, ptype := range productTypes {
		protoProductType[i] = s.productTypeToProto(ptype)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListProductTypesResponse{
		Success:      true,
		ProductTypes: protoProductType,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}
