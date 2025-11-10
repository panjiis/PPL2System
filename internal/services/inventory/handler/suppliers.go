package handler

import (
	"context"
	"strconv"

	"gorm.io/gorm"

	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/inventory"
)

func (s *InventoryHandler) CreateSupplier(ctx context.Context, req *proto.CreateSupplierRequest) (*proto.CreateSupplierResponse, error) {
	var supplier Supplier
	if req.GetSupplierCode() == "" || req.GetSupplierName() == "" {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Supplier Code and Name Must be Provided"),
		}, nil
	}

	supplier = Supplier{
		SupplierCode:  req.GetSupplierCode(),
		SupplierName:  req.GetSupplierName(),
		ContactPerson: req.ContactPerson,
		Phone:         req.Phone,
		Email:         lib.StrPtr(req.GetEmail()),
		Address:       lib.StrPtr(req.GetAddress()),
	}

	if err := s.db.Create(&supplier).Error; err != nil {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Error while creating Supplier"),
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
			Message: lib.StrPtr("Supplier ID needed"),
		}, nil
	}

	if err := s.db.Where("id = ?", req.GetId()).First(&supplier).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetSupplierResponse{
				Success: false,
				Message: lib.StrPtr("Supplier not found"),
			}, nil
		}
		return &proto.GetSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	return &proto.GetSupplierResponse{
		Success:  true,
		Supplier: s.supplierToProto(supplier),
	}, nil
}

func (s *InventoryHandler) ListSuppliers(ctx context.Context, req *proto.ListSuppliersRequest) (*proto.ListSuppliersResponse, error) {
	var suppliers []Supplier
	var total int64

	query := s.db.Model(&Supplier{})

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
	if err := query.Offset(offset).Limit(pageSize).Find(&suppliers).Error; err != nil {
		return &proto.ListSuppliersResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
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

func (s *InventoryHandler) UpdateSupplier(ctx context.Context, req *proto.UpdateSupplierRequest) (*proto.CreateSupplierResponse, error) {
	var supplier Supplier

	if req.GetSupplierCode() == "" {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Supplier code required"),
		}, nil
	}

	if err := s.db.Where("supplier_code = ?", req.GetSupplierCode()).First(&supplier).Error; err != nil {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Warehouse not found"),
		}, err
	}

	if req.SupplierName != nil {
		supplier.SupplierName = req.GetSupplierName()
	}
	if req.ContactPerson != nil {
		supplier.ContactPerson = lib.StrPtr(req.GetContactPerson())
	}
	if req.Phone != nil {
		Phone := req.GetPhone()
		supplier.Phone = &Phone
	}
	if req.Email != nil {
		Email := req.GetEmail()
		supplier.Email = &Email
	}
	if req.Address != nil {
		Address := req.GetAddress()
		supplier.Address = &Address
	}

	if err := s.db.Save(&supplier).Error; err != nil {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("error updating warehouse"),
		}, err
	}

	s.redis.Del(ctx, WAREHOUSE_CACHE_KEY)

	return &proto.CreateSupplierResponse{
		Success:  true,
		Supplier: s.supplierToProto(supplier),
	}, nil
}

func (s *InventoryHandler) UpdateSupplierStatus(ctx context.Context, req *proto.UpdateSupplierStatusRequest) (*proto.CreateSupplierResponse, error) {
	var supplier Supplier

	if req.GetSupplierCode() == "" {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Supplier code required"),
		}, nil
	}

	if err := s.db.Where("supplier_code = ?", req.GetSupplierCode()).First(&supplier).Error; err != nil {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("Warehouse not found"),
		}, err
	}

	supplier.IsActive = req.GetIsActive()

	if err := s.db.Save(&supplier).Error; err != nil {
		return &proto.CreateSupplierResponse{
			Success: false,
			Message: lib.StrPtr("error updating warehouse"),
		}, err
	}

	s.redis.Del(ctx, WAREHOUSE_CACHE_KEY)

	return &proto.CreateSupplierResponse{
		Success:  true,
		Supplier: s.supplierToProto(supplier),
	}, nil
}
