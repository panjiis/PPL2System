package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/inventory"
	"time"

	"gorm.io/gorm"
)

func (s *InventoryHandler) ListStocks(ctx context.Context, req *proto.ListStockRequest) (*proto.ListStockResponse, error) {
	var stocks []Stock
	s.db = s.db.Debug()
	query := s.db.Model(&Stock{})

	pageSize := int32(50)
	offset := int32(0)

	if req.SearchTerm != nil && req.GetSearchTerm() != "" {
		searchTerm := fmt.Sprintf("%%%s%%", *req.SearchTerm)
		query = query.Joins("LEFT JOIN inventory_products ON inventory_products.product_code = stocks.product_code").
			Where(
				"inventory_products.product_code ILIKE ? OR inventory_products.product_name ILIKE ? OR inventory_products.unit_of_measure ILIKE ?",
				searchTerm, searchTerm, searchTerm,
			)
	}

	if req.ProductCode != nil && req.GetProductCode() != "" {
		query = query.Where("product_code = ?", req.GetProductCode())
	}

	if req.WarehouseId != nil && req.GetWarehouseId() != 0 {
		query = query.Where("warehouse_id = ?", req.GetWarehouseId())
	}

	if req.Pagination != nil {
		if req.Pagination.GetPageSize() > 0 {
			pageSize = req.Pagination.GetPageSize()
		}
		if req.Pagination.GetPageToken() != "" {
			if n, err := fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset); err != nil || n != 1 {
				return &proto.ListStockResponse{
					Success: false,
					Message: lib.StrPtr("Invalid page token"),
				}, fmt.Errorf("invalid page token: %s", req.Pagination.GetPageToken())
			}
		}
	}

	var totalCount int64
	countQuery := s.db.Model(&Stock{})
	if req.SearchTerm != nil && req.GetSearchTerm() != "" {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		countQuery = countQuery.Joins("LEFT JOIN inventory_products ON inventory_products.product_code = stocks.product_code").
			Where(
				"inventory_products.product_code ILIKE ? OR inventory_products.product_name ILIKE ? OR inventory_products.unit_of_measure ILIKE ?",
				searchTerm, searchTerm, searchTerm,
			)
	}

	if err := countQuery.Count(&totalCount).Error; err != nil {
		log.Printf("Count error: %v", err)
		return &proto.ListStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to count records"),
		}, err
	}

	log.Printf("Total count: %d", totalCount)

	if req.SearchTerm != nil && req.GetSearchTerm() != "" {
		query = query.Select("DISTINCT stocks.*")
	}

	if err := query.
		Preload("Product").
		Preload("Warehouse").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&stocks).Error; err != nil {
		log.Printf("Query error: %v", err)
		return &proto.ListStockResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	log.Printf("Fetched stocks: %d", len(stocks))

	protoStocks := make([]*proto.Stock, 0, len(stocks))
	for _, stock := range stocks {
		protoStock := s.stockToProto(stock)
		if protoStock != nil {
			protoStocks = append(protoStocks, protoStock)
		} else {
			log.Printf("Warning: stockToProto returned nil for stock ID: %d", stock.ID)
		}
	}

	log.Printf("Proto stocks: %d", len(protoStocks))

	nextPageToken := ""
	if int32(len(stocks)) == pageSize && int64(offset+pageSize) < totalCount {
		nextPageToken = fmt.Sprintf("%d", offset+pageSize)
	}

	return &proto.ListStockResponse{
		Stock: protoStocks,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(totalCount),
		},
		Success: true,
	}, nil
}

func (s *InventoryHandler) ReserveStock(ctx context.Context, req *proto.ReserveStockRequest) (*proto.ReserveStockResponse, error) {
	if req.GetProductCode() == "" {
		return &proto.ReserveStockResponse{
			Success: false,
			Message: lib.StrPtr("product_code required"),
		}, nil
	}
	if req.GetWarehouseId() == 0 {
		return &proto.ReserveStockResponse{
			Success: false,
			Message: lib.StrPtr("warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.ReserveStockResponse{
			Success: false,
			Message: lib.StrPtr("quantity must be greater than 0"),
		}, nil
	}

	var stock Stock

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("product_code = ? AND warehouse_id = ?", req.GetProductCode(), req.GetWarehouseId()).
		First(&stock).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.ReserveStockResponse{
				Success: false,
				Message: lib.StrPtr("Stock not found for this product and warehouse"),
			}, nil
		}
		return &proto.ReserveStockResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if stock.AvailableQuantity < req.GetQuantity() {
		tx.Rollback()
		return &proto.ReserveStockResponse{
			Success: false,
			Message: lib.StrPtr(fmt.Sprintf("Insufficient stock. Available: %d, Requested: %d",
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
			Message: lib.StrPtr("Failed to update stock"),
		}, err
	}

	referenceId := req.GetReferenceId()

	employee, err := s.GetManagerDetails(ctx, req.GetReservedBy())
	if err != nil {
		return nil, err
	}

	movement := StockMovement{
		ProductCode:   req.GetProductCode(),
		WarehouseID:   req.GetWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_ADJUSTMENT),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_ADJUSTMENT),
		ReferenceID:   &referenceId,
		CreatedBy:     req.GetReservedBy(),
		CreatedAt:     time.Now(),

		ManagerName: employee.ManagerName,
	}

	if err := tx.Create(&movement).Error; err != nil {
		tx.Rollback()
		return &proto.ReserveStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to create stock movement record"),
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
	if req.GetProductCode() == "" {
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: lib.StrPtr("product_code required"),
		}, nil
	}
	if req.GetWarehouseId() == 0 {
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: lib.StrPtr("warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: lib.StrPtr("quantity must be greater than 0"),
		}, nil
	}

	var stock Stock

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("product_code = ? AND warehouse_id = ?", req.GetProductCode(), req.GetWarehouseId()).
		First(&stock).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.ReleaseStockResponse{
				Success: false,
				Message: lib.StrPtr("Stock not found for this product and warehouse"),
			}, nil
		}
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if stock.ReservedQuantity < req.GetQuantity() {
		tx.Rollback()
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: lib.StrPtr(fmt.Sprintf("Insufficient reserved stock. Reserved: %d, Requested: %d",
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
			Message: lib.StrPtr("Failed to update stock"),
		}, err
	}

	referenceId := req.GetReferenceId()

	employee, err := s.GetManagerDetails(ctx, req.GetReleasedBy())
	if err != nil {
		return nil, err
	}

	movement := StockMovement{
		ProductCode:   req.GetProductCode(),
		WarehouseID:   req.GetWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_ADJUSTMENT),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_ADJUSTMENT),
		ReferenceID:   &referenceId,
		CreatedBy:     req.GetReleasedBy(),
		CreatedAt:     time.Now(),

		ManagerName: employee.ManagerName,
	}

	if err := tx.Create(&movement).Error; err != nil {
		tx.Rollback()
		return &proto.ReleaseStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to create stock movement record"),
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
	if req.GetProductCode() == "" {
		return &proto.UpdateStockResponse{
			Success: false,
			Message: lib.StrPtr("product_code required"),
		}, nil
	}
	if req.GetWarehouseId() == 0 {
		return &proto.UpdateStockResponse{
			Success: false,
			Message: lib.StrPtr("warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.UpdateStockResponse{
			Success: false,
			Message: lib.StrPtr("quantity must be greater than 0"),
		}, nil
	}

	var stock Stock

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	result := tx.Where("product_code = ? AND warehouse_id = ?", req.GetProductCode(), req.GetWarehouseId()).First(&stock)

	if result.Error == gorm.ErrRecordNotFound {
		stock = Stock{
			ProductCode:       req.GetProductCode(),
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
			Message: lib.StrPtr("Database error"),
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
				Message: lib.StrPtr(fmt.Sprintf("Insufficient stock. Available: %d, Requested: %d",
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
				Message: lib.StrPtr("Adjustment would result in negative stock"),
			}, nil
		}
	default:
		tx.Rollback()
		return &proto.UpdateStockResponse{
			Success: false,
			Message: lib.StrPtr("Invalid movement type"),
		}, nil
	}

	stock.UpdatedAt = time.Now()

	if err := tx.Save(&stock).Error; err != nil {
		tx.Rollback()
		return &proto.UpdateStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to update stock"),
		}, err
	}

	employee, err := s.GetManagerDetails(ctx, req.GetCreatedBy())
	if err != nil {
		return nil, err
	}

	movement := StockMovement{
		ProductCode:   req.GetProductCode(),
		WarehouseID:   req.GetWarehouseId(),
		MovementType:  int32(req.GetMovementType()),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(req.GetReferenceType()),
		CreatedBy:     req.GetCreatedBy(),
		CreatedAt:     time.Now(),

		ManagerName: employee.ManagerName,
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
			Message: lib.StrPtr("Failed to create stock movement record"),
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
			Message: lib.StrPtr("Failed to count records"),
		}, err
	}

	if err := query.Offset(int(offset)).Limit(int(pageSize)).Find(&stocks).Error; err != nil {
		return &proto.ListLowStockResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
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
	if req.GetProductCode() == "" {
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("product_code required"),
		}, nil
	}
	if req.GetFromWarehouseId() == 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("from_warehouse_id required"),
		}, nil
	}
	if req.GetToWarehouseId() == 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("to_warehouse_id required"),
		}, nil
	}
	if req.GetQuantity() <= 0 {
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("quantity must be greater than 0"),
		}, nil
	}
	if req.GetFromWarehouseId() == req.GetToWarehouseId() {
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("cannot transfer to the same warehouse"),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var fromStock, toStock Stock

	if err := tx.Where("product_code = ? AND warehouse_id = ?",
		req.GetProductCode(), req.GetFromWarehouseId()).First(&fromStock).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.TransferStockResponse{
				Success: false,
				Message: lib.StrPtr("Source stock not found"),
			}, nil
		}
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	if fromStock.AvailableQuantity < req.GetQuantity() {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr(fmt.Sprintf("Insufficient stock in source warehouse. Available: %d, Requested: %d",
				fromStock.AvailableQuantity, req.GetQuantity())),
		}, nil
	}

	result := tx.Where("product_code = ? AND warehouse_id = ?",
		req.GetProductCode(), req.GetToWarehouseId()).First(&toStock)

	if result.Error == gorm.ErrRecordNotFound {
		toStock = Stock{
			ProductCode:       req.GetProductCode(),
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
			Message: lib.StrPtr("Database error"),
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
			Message: lib.StrPtr("Failed to update source stock"),
		}, err
	}

	if err := tx.Save(&toStock).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to update destination stock"),
		}, err
	}

	transferRefId := fmt.Sprintf("TRANSFER_%d_%d_%d", req.GetProductCode(), req.GetFromWarehouseId(), time.Now().Unix())

	employee, err := s.GetManagerDetails(ctx, req.GetTransferredBy())
	if err != nil {
		return nil, err
	}

	outMovement := StockMovement{
		ProductCode:   req.GetProductCode(),
		WarehouseID:   req.GetFromWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_TRANSFER),
		Quantity:      -req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_TRANSFER),
		ReferenceID:   &transferRefId,
		CreatedBy:     req.GetTransferredBy(),
		CreatedAt:     time.Now(),

		ManagerName: employee.ManagerName,
	}

	inMovement := StockMovement{
		ProductCode:   req.GetProductCode(),
		WarehouseID:   req.GetToWarehouseId(),
		MovementType:  int32(proto.MovementType_MOVEMENT_TYPE_TRANSFER),
		Quantity:      req.GetQuantity(),
		ReferenceType: int32(proto.ReferenceType_REFERENCE_TYPE_TRANSFER),
		ReferenceID:   &transferRefId,
		CreatedBy:     req.GetTransferredBy(),
		CreatedAt:     time.Now(),

		ManagerName: employee.ManagerName,
	}

	if req.Notes != nil {
		outMovement.Notes = req.Notes
		inMovement.Notes = req.Notes
	}

	if err := tx.Create(&outMovement).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to create outbound movement record"),
		}, err
	}

	if err := tx.Create(&inMovement).Error; err != nil {
		tx.Rollback()
		return &proto.TransferStockResponse{
			Success: false,
			Message: lib.StrPtr("Failed to create inbound movement record"),
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
		Message:          lib.StrPtr("Stock transferred successfully"),
	}, nil
}

// -- Stock Movement --
func (s *InventoryHandler) ListStockMovements(ctx context.Context, req *proto.ListStockMovementsRequest) (*proto.ListStockMovementsResponse, error) {
	var stockMovements []StockMovement
	var total int64

	query := s.db.Model(&StockMovement{})

	if req.ProductCode != nil && *req.ProductCode != "" {
		query = query.Where("product_code = ?", *req.ProductCode)
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
			Message: lib.StrPtr("Failed to count stock movements"),
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
			Message: lib.StrPtr("Database error"),
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
