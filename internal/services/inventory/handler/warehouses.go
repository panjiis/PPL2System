package handler

import (
	"context"
	"log"
	"strconv"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/inventory"

	"gorm.io/gorm"
)

func (s *InventoryHandler) CreateWarehouse(ctx context.Context, req *proto.CreateWarehouseRequest) (*proto.CreateWarehouseResponse, error) {
	var warehouse Warehouse
	if req.GetWarehouseCode() == "" || req.GetWarehouseName() == "" {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("Warehouse code and name required"),
		}, nil
	}

	warehouse = Warehouse{
		WarehouseCode: req.GetWarehouseCode(),
		WarehouseName: req.GetWarehouseName(),
		Location:      lib.StrPtr(req.GetLocation()),
	}
	managerId := req.GetManagerId()
	warehouse.ManagerID = &managerId

	if err := s.db.Create(&warehouse).Error; err != nil {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("error creating Product"),
		}, err
	}

	employee, err := s.GetManagerDetails(ctx, req.GetManagerId())
	if err != nil {
		return nil, err
	}

	warehouse.ManagerName = employee.ManagerName

	_ = s.redis.Del(ctx, WAREHOUSE_CACHE_KEY)

	return &proto.CreateWarehouseResponse{
		Success:   true,
		Warehouse: s.warehouseToProto(warehouse),
	}, nil
}

func (s *InventoryHandler) UpdateWarehouse(ctx context.Context, req *proto.UpdateWarehouseRequest) (*proto.CreateWarehouseResponse, error) {
	var warehouse Warehouse

	if req.GetWarehouseCode() == "" {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("Warehouse code required"),
		}, nil
	}

	if err := s.db.Where("warehouse_code = ?", req.GetWarehouseCode()).First(&warehouse).Error; err != nil {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("Warehouse not found"),
		}, err
	}

	if req.WarehouseName != nil {
		warehouse.WarehouseName = req.GetWarehouseName()
	}
	if req.Location != nil {
		warehouse.Location = lib.StrPtr(req.GetLocation())
	}
	if req.ManagerId != nil {
		managerId := req.GetManagerId()
		warehouse.ManagerID = &managerId
	}

	if err := s.db.Save(&warehouse).Error; err != nil {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("error updating warehouse"),
		}, err
	}

	employee, err := s.GetManagerDetails(ctx, req.GetManagerId())
	if err != nil {
		return nil, err
	}

	warehouse.ManagerName = employee.ManagerName

	s.redis.Del(ctx, WAREHOUSE_CACHE_KEY)

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
			Message: lib.StrPtr("warehouse_coderequired"),
		}, nil
	}

	if err := s.db.Where("warehouse_code = ?", req.GetWarehouseCode()).First(&warehouse).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetWarehouseResponse{
				Success: false,
				Message: lib.StrPtr("Warehouse not found"),
			}, nil
		}
		return &proto.GetWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	return &proto.GetWarehouseResponse{
		Success:   true,
		Warehouse: s.warehouseToProto(warehouse),
	}, nil
}

func (s *InventoryHandler) ListWarehouses(ctx context.Context, req *proto.ListWarehousesRequest) (*proto.ListWarehousesResponse, error) {
	var warehouses []Warehouse
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
			searchTerm, searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListWarehousesResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
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
	if err := query.Offset(offset).Limit(pageSize).Find(&warehouses).Error; err != nil {
		return &proto.ListWarehousesResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	// Batch process manager names
	managerIDs := make(map[int64]bool)
	var uniqueManagerIDs []int64

	for _, wh := range warehouses {
		if wh.ManagerID != nil && *wh.ManagerID > 0 {
			if !managerIDs[*wh.ManagerID] {
				managerIDs[*wh.ManagerID] = true
				uniqueManagerIDs = append(uniqueManagerIDs, *wh.ManagerID)
			}
		}
	}

	managerNameMap := make(map[int64]string)
	for _, managerID := range uniqueManagerIDs {
		manager, err := s.GetManagerDetails(ctx, managerID)
		if err == nil {
			managerNameMap[managerID] = manager.ManagerName
		} else {
			log.Printf("Failed to get manager for ID %d: %v", managerID, err)
			managerNameMap[managerID] = "Manager not found"
		}
	}

	protoWarehouses := make([]*proto.Warehouse, len(warehouses))
	for i, wh := range warehouses {
		warehouseWithManager := wh
		if wh.ManagerID != nil && *wh.ManagerID > 0 {
			warehouseWithManager.ManagerName = managerNameMap[*wh.ManagerID]
		}

		protoWH := s.warehouseToProto(warehouseWithManager)

		protoWarehouses[i] = protoWH
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListWarehousesResponse{
		Success:    true,
		Warehouses: protoWarehouses,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

func (s *InventoryHandler) UpdateWarehouseStatus(ctx context.Context, req *proto.UpdateWarehouseStatusRequest) (*proto.CreateWarehouseResponse, error) {
	var warehouse Warehouse

	if req.GetWarehouseCode() == "" {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("Supplier code required"),
		}, nil
	}

	if err := s.db.Where("warehouse_code = ?", req.GetWarehouseCode()).First(&warehouse).Error; err != nil {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("Warehouse not found"),
		}, err
	}

	warehouse.IsActive = req.GetIsActive()

	if err := s.db.Save(&warehouse).Error; err != nil {
		return &proto.CreateWarehouseResponse{
			Success: false,
			Message: lib.StrPtr("error updating warehouse"),
		}, err
	}

	s.redis.Del(ctx, WAREHOUSE_CACHE_KEY)

	return &proto.CreateWarehouseResponse{
		Success:   true,
		Warehouse: s.warehouseToProto(warehouse),
	}, nil
}
