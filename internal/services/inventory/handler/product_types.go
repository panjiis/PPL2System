package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/inventory"

	"gorm.io/gorm"
)

func (s *InventoryHandler) CreateProductType(ctx context.Context, req *proto.CreateProductTypeRequest) (*proto.CreateProductTypeResponse, error) {
	var productType ProductType
	if req.GetProductTypeName() == "" || req.GetProductTypeCode() == "" {
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("Product Type Name neeeded"),
		}, nil
	}

	productType = ProductType{
		ProductTypeName: req.GetProductTypeName(),
		ProductTypeCode: req.GetProductTypeCode(),
		Description:     lib.StrPtr(req.GetDescription()),
	}

	if err := s.db.Create(&productType).Error; err != nil {
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("Failed to Create Product Type"),
		}, err
	}

	return &proto.CreateProductTypeResponse{
		Success:     true,
		ProductType: s.productTypeToProto(productType),
	}, nil
}

func (s *InventoryHandler) ListProductTypes(ctx context.Context, req *proto.ListProductTypesRequest) (*proto.ListProductTypesResponse, error) {
	var productTypes []ProductType
	var total int64

	query := s.db.Model(&ProductType{})

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
	if err := query.Offset(offset).Limit(pageSize).Find(&productTypes).Error; err != nil {
		return &proto.ListProductTypesResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
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

func (s *InventoryHandler) UpdateProductType(ctx context.Context, req *proto.UpdateProductTypeRequest) (*proto.CreateProductTypeResponse, error) {
	if req.GetId() == 0 {
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("Product type ID is required"),
		}, nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var productType ProductType
	if err := tx.Where("id = ?", req.GetId()).First(&productType).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return &proto.CreateProductTypeResponse{
				Success: false,
				Message: lib.StrPtr("Product type not found"),
			}, nil
		}
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("Database error"),
		}, err
	}

	oldCode := productType.ProductTypeCode

	// Update mutable fields
	if req.ProductTypeName != nil {
		productType.ProductTypeName = *req.ProductTypeName
	}
	if req.Description != nil {
		productType.Description = req.Description
	}

	codeChanged := false
	if req.ProductTypeCode != nil && *req.ProductTypeCode != oldCode {
		productType.ProductTypeCode = *req.ProductTypeCode
		codeChanged = true
	}

	if err := tx.Save(&productType).Error; err != nil {
		tx.Rollback()
		return &proto.CreateProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("Failed to update product type"),
		}, err
	}

	// Only proceed if ProductTypeCode changed
	if codeChanged {
		var products []InventoryProduct
		if err := tx.Where("product_type_id = ?", productType.ID).Find(&products).Error; err != nil {
			tx.Rollback()
			return &proto.CreateProductTypeResponse{
				Success: false,
				Message: lib.StrPtr("Failed to load products"),
			}, err
		}

		for _, prod := range products {
			// Parse existing code: "FOOD-0001" → prefix="FOOD", suffix="0001"
			parts := strings.Split(prod.ProductCode, "-")
			if len(parts) < 2 {
				// Fallback: keep original if format is unexpected
				continue
			}

			// Rebuild with new prefix
			newCode := productType.ProductTypeCode + "-" + parts[len(parts)-1]

			// Update product
			if err := tx.Model(&prod).Update("product_code", newCode).Error; err != nil {
				tx.Rollback()
				return &proto.CreateProductTypeResponse{
					Success: false,
					Message: lib.StrPtr(fmt.Sprintf("Failed to update product code from %s to %s", prod.ProductCode, newCode)),
				}, err
			}

			// Update stocks
			if err := tx.Model(&Stock{}).Where("product_code = ?", prod.ProductCode).Update("product_code", newCode).Error; err != nil {
				tx.Rollback()
				return &proto.CreateProductTypeResponse{
					Success: false,
					Message: lib.StrPtr(fmt.Sprintf("Failed to update stock for %s", prod.ProductCode)),
				}, err
			}

			// Update stock movements
			if err := tx.Model(&StockMovement{}).Where("product_code = ?", prod.ProductCode).Update("product_code", newCode).Error; err != nil {
				tx.Rollback()
				return &proto.CreateProductTypeResponse{
					Success: false,
					Message: lib.StrPtr(fmt.Sprintf("Failed to update stock movements for %s", prod.ProductCode)),
				}, err
			}
		}
	}

	tx.Commit()

	// Invalidate caches
	s.InvalidateInventoryCaches(ctx)

	return &proto.CreateProductTypeResponse{
		Success:     true,
		ProductType: s.productTypeToProto(productType),
	}, nil
}

func (s *InventoryHandler) ListProductsByProductType(ctx context.Context, req *proto.ListProductsByProductTypeRequest) (*proto.ListProductsByProductTypeResponse, error) {
	if req.GetProductTypeId() == 0 {
		return &proto.ListProductsByProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("product_type_id is required"),
		}, nil
	}

	var products []InventoryProduct
	var total int64

	query := s.db.Model(&InventoryProduct{}).
		Where("product_type_id = ?", req.GetProductTypeId()).
		Preload("ProductType").
		Preload("Supplier").
		Preload("Stocks")

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductsByProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("database error during count"),
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
		return &proto.ListProductsByProductTypeResponse{
			Success: false,
			Message: lib.StrPtr("database error fetching products"),
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

	return &proto.ListProductsByProductTypeResponse{
		Success:  true,
		Products: protoProducts,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

func (s *InventoryHandler) GetProductType(ctx context.Context, req *proto.GetProductTypeRequest) (*proto.ProductType, error) {
	var productType ProductType

	if err := s.db.Where("id = ?", req.GetId()).First(&productType).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.ProductType{
				Id: 0,
			}, nil
		}
		return &proto.ProductType{
			Id: 0,
		}, err
	}

	return s.productTypeToProto(productType), nil
}
