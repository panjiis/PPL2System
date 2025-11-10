package handler

import (
	"context"
	"strconv"

	"gorm.io/gorm"

	invenFunction "syntra-system/internal/services/inventory"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/inventory"
)

// -- Inventory Products --

func (s *InventoryHandler) CreateProduct(ctx context.Context, req *proto.CreateProductRequest) (*proto.CreateProductResponse, error) {
	var product InventoryProduct
	if req.GetProductTypeId() == 0 || req.GetProductName() == "" {
		return &proto.CreateProductResponse{
			Success: false,
			Message: lib.StrPtr("Product Type and Product Name is Required"),
		}, nil
	}

	productCode, err := invenFunction.GenerateProductCode(s.db, req.GetProductTypeId())
	if err != nil {
		return &proto.CreateProductResponse{
			Success: false,
			Message: lib.StrPtr("error generating Product Code"),
		}, err
	}

	product = InventoryProduct{
		ProductCode:   productCode,
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
			Message: lib.StrPtr("error creating Product"),
		}, err
	}

	_ = s.redis.Del(ctx, PRODUCTS_CACHE_KEY)

	return &proto.CreateProductResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}

func (s *InventoryHandler) UpdateProduct(ctx context.Context, req *proto.UpdateProductRequest) (*proto.UpdateProductResponse, error) {
	var product InventoryProduct

	if req.GetProductCode() == "" {
		return &proto.UpdateProductResponse{
			Success: false,
			Message: lib.StrPtr("Id must be provided"),
		}, nil
	}

	if err := s.db.Where("product_code = ?", req.GetProductCode()).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.UpdateProductResponse{
				Success: false,
				Message: lib.StrPtr("Products not found"),
			}, err
		}
		return &proto.UpdateProductResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
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

	if err := s.db.Save(&product).Error; err != nil {
		return &proto.UpdateProductResponse{
			Success: false,
			Message: lib.StrPtr("error updating products"),
		}, err
	}

	s.InvalidateInventoryCaches(ctx)

	return &proto.UpdateProductResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}

func (s *InventoryHandler) ListProducts(ctx context.Context, req *proto.ListProductsRequest) (*proto.ListProductsResponse, error) {
	var products []InventoryProduct
	var total int64

	query := s.db.Model(&InventoryProduct{}).Preload("ProductType").Preload("Supplier").Preload("Stocks")

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
	if err := query.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
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

func (s *InventoryHandler) GetProduct(ctx context.Context, req *proto.GetProductByCodeRequest) (*proto.GetProductByCodeResponse, error) {
	var product InventoryProduct

	if req.GetProductCode() == "" {
		return &proto.GetProductByCodeResponse{
			Success: false,
			Message: lib.StrPtr("Id must be provided"),
		}, nil
	}

	if err := s.db.Preload("Supplier").Preload("ProductType").Preload("Stocks").Where("product_code = ?", req.GetProductCode()).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetProductByCodeResponse{
				Success: false,
				Message: lib.StrPtr("Products not Found"),
			}, err
		}
		return &proto.GetProductByCodeResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	return &proto.GetProductByCodeResponse{
		Success: true,
		Product: s.inventoryProductsToProto(product),
	}, nil
}
