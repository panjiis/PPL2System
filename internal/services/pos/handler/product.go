package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	posFunc "syntra-system/internal/services/pos"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"
)

func (s *POSHandler) CreateProduct(ctx context.Context, req *proto.Product) (*proto.GetProductResponse, error) {

	newProduct := &Product{
		ProductName:             req.GetProductName(),
		ProductPrice:            req.GetProductPrice(),
		CostPrice:               req.GetCostPrice(),
		CommissionEligible:      req.GetCommissionEligible(),
		RequiresServiceEmployee: req.GetRequiresServiceEmployee(),
		IsActive:                req.GetIsActive(),
	}

	if req.ProductGroupId != nil {
		id := int32(*req.ProductGroupId)
		newProduct.ProductGroupId = &id
	}

	if req.ImageUrl != nil {
		newProduct.ImageUrl = req.ImageUrl
	}

	if req.GetRequiresServiceEmployee() {
		if req.ProductGroupId == nil {
			return nil, status.Errorf(codes.InvalidArgument, "product_group_id is required for service products")
		}

		serviceCode, err := posFunc.GenerateServiceCode(s.db, *newProduct.ProductGroupId)
		if err != nil {
			log.Printf("Error generating service code: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to generate service code: %v", err)
		}
		newProduct.ProductCode = serviceCode
		log.Printf("Auto-generated service code: %s", serviceCode)
	} else {
		if req.GetProductCode() == "" {
			return nil, status.Errorf(codes.InvalidArgument, "product_code is required for non-service products")
		}
		newProduct.ProductCode = req.GetProductCode()
	}

	if err := s.db.Create(newProduct).Error; err != nil {
		log.Printf("Error creating product: %v", err)
		if err := new(gorm.DB).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
				return nil, status.Errorf(codes.AlreadyExists, "product with code '%s' already exists", newProduct.ProductCode)
			}
		}
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}

	log.Printf("Successfully created product with code: %s", newProduct.ProductCode)

	return &proto.GetProductResponse{
		Success: true,
		Product: s.productToProto(*newProduct),
	}, nil
}

func (s *POSHandler) UpdateProduct(ctx context.Context, req *proto.UpdateProductRequest) (*proto.GetProductResponse, error) {
	log.Printf("Received UpdateProduct request for : %v", req)

	var existingProduct Product
	if err := s.db.Where("product_code = ?", req.GetProductCode()).First(&existingProduct).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "product with code %v not found", req.GetProductCode())
		}
		return nil, status.Errorf(codes.Internal, "failed to find product: %v", err)
	}

	if req.ProductName != nil {
		existingProduct.ProductName = req.GetProductName()
	}
	if req.ProductPrice != nil {
		existingProduct.ProductPrice = req.GetProductPrice()
	}
	if req.CostPrice != nil {
		existingProduct.CostPrice = req.GetCostPrice()
	}
	existingProduct.ProductGroupId = req.ProductGroupId
	if req.ImageUrl != nil {
		existingProduct.ImageUrl = req.ImageUrl
	}
	if req.Color != nil {
		existingProduct.Color = req.Color
	}
	if req.CommissionEligible != nil {
		existingProduct.CommissionEligible = req.GetCommissionEligible()
	}
	if req.RequiresServiceEmployee != nil {
		existingProduct.RequiresServiceEmployee = req.GetRequiresServiceEmployee()
	}
	if req.IsActive != nil {
		existingProduct.IsActive = req.GetIsActive()
	}

	if err := s.db.Save(&existingProduct).Error; err != nil {
		log.Printf("Error updating product: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update product: %v", err)
	}

	log.Printf("Successfully updated product with ID: %s", existingProduct.ProductCode)

	return &proto.GetProductResponse{
		Success: true,
		Product: s.productToProto(existingProduct),
	}, nil
}

func (s *POSHandler) GetProduct(crx context.Context, req *proto.GetProductRequest) (*proto.GetProductResponse, error) {
	var product Product

	if req.GetProductCode() == "" {
		return &proto.GetProductResponse{
			Success: false,
			Message: lib.StrPtr("Product_code must be provided"),
		}, nil
	}

	if err := s.db.Where("product_code = ?", req.GetProductCode()).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetProductResponse{
				Success: false,
				Message: lib.StrPtr("Product not found"),
			}, err
		} else {

			return &proto.GetProductResponse{
				Success: false,
				Message: lib.StrPtr("database error"),
			}, err
		}
	}

	return &proto.GetProductResponse{
		Success: true,
		Product: s.productToProto(product),
	}, nil
}

func (s *POSHandler) ListProducts(ctx context.Context, req *proto.ListProductsRequest) (*proto.ListProductsResponse, error) {
	var products []Product
	var total int64

	query := s.db.Model(&Product{}).Preload("ProductGroup")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.ProductGroupId != nil {
		query = query.Where("product_group_id = ?", req.GetProductGroupId())
	}
	if req.SearchTerm != nil {
		searchTerm := "%" + req.GetSearchTerm() + "%"
		query = query.Where(
			"product_code ILIKE ? OR product_name ILIKE ?",
			searchTerm, searchTerm,
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	pageSize := int(req.GetPagination().GetPageSize())
	pageNumber := 1

	if pageSize > 0 {
		if token := req.GetPagination().GetPageToken(); token != "" {
			if n, err := strconv.Atoi(token); err == nil && n > 0 {
				pageNumber = n
			}
		}

		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	if err := query.Find(&products).Error; err != nil {
		return &proto.ListProductsResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	protoProducts := make([]*proto.Product, len(products))
	for i, prod := range products {
		protoProducts[i] = s.productToProto(prod)
	}

	nextPageToken := ""
	if pageSize > 0 && int64(pageNumber*pageSize) < total {
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

// -- Product Groups --

func (s *POSHandler) CreateProductGroup(ctx context.Context, req *proto.CreateProductGroupRequest) (*proto.CreateProductGroupResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request cannot be nil")
	}
	if req.ProductGroupName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "product_group_name is required")
	}

	newProductGroup := &ProductGroup{
		ProductGroupName: req.GetProductGroupName(),
		CommissionRate:   req.GetCommissionRate(),
		IsActive:         req.GetIsActive(),
	}

	if req.ParentGroupId != nil {
		parentID := req.GetParentGroupId()
		if parentID <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "parent_group_id must be a positive integer")
		}

		var parent ProductGroup
		if err := s.db.First(&parent, parentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, status.Errorf(codes.NotFound, "parent product group with ID %d not found", parentID)
			}
			return nil, status.Errorf(codes.Internal, "failed to check parent product group: %v", err)
		}
		newProductGroup.ParentGroupId = &parentID
	}

	if req.Color != nil {
		newProductGroup.Color = lib.StrPtr(req.GetColor())
	}
	if req.ImageUrl != nil {
		newProductGroup.ImageUrl = lib.StrPtr(req.GetImageUrl())
	}

	if err := s.db.Create(newProductGroup).Error; err != nil {
		log.Printf("Error creating product group: %v", err)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, status.Errorf(codes.AlreadyExists, "product group with name '%s' already exists", newProductGroup.ProductGroupName)
		}
		return nil, status.Errorf(codes.Internal, "failed to create product group: %v", err)
	}

	return &proto.CreateProductGroupResponse{
		Success:      true,
		ProductGroup: s.productGroupToProto(*newProductGroup),
	}, nil
}

func (s *POSHandler) UpdateProductGroup(ctx context.Context, req *proto.UpdateProductGroupRequest) (*proto.UpdateProductGroupResponse, error) {
	if req == nil || req.Id <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "valid product group ID is required")
	}

	var existing ProductGroup
	if err := s.db.First(&existing, req.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "product group with ID %d not found", req.Id)
		}
		return nil, status.Errorf(codes.Internal, "failed to find product group: %v", err)
	}

	if req.ProductGroupName != nil {
		existing.ProductGroupName = req.GetProductGroupName()
	}
	if req.CommissionRate != nil {
		existing.CommissionRate = req.GetCommissionRate()
	}
	if req.Color != nil {
		existing.Color = lib.StrPtr(req.GetColor())
	}
	if req.ImageUrl != nil {
		existing.ImageUrl = lib.StrPtr(req.GetImageUrl())
	}
	if req.IsActive != nil {
		existing.IsActive = req.GetIsActive()
	}

	if req.ParentGroupId != nil {
		parentID := req.GetParentGroupId()
		if parentID <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "parent_group_id must be a positive integer")
		}

		if parentID == req.Id {
			return nil, status.Errorf(codes.InvalidArgument, "product group cannot be its own parent")
		}

		var parent ProductGroup
		if err := s.db.First(&parent, parentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, status.Errorf(codes.NotFound, "parent product group with ID %d not found", parentID)
			}
			return nil, status.Errorf(codes.Internal, "failed to verify parent: %v", err)
		}
		existing.ParentGroupId = &parentID
	}

	if err := s.db.Save(&existing).Error; err != nil {
		log.Printf("Error updating product group: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update product group: %v", err)
	}

	return &proto.UpdateProductGroupResponse{
		Success:      true,
		ProductGroup: s.productGroupToProto(existing),
	}, nil
}

func (s *POSHandler) GetProductGroup(ctx context.Context, req *proto.GetProductGroupRequest) (*proto.GetProductGroupResponse, error) {
	if req == nil || req.Id <= 0 {
		return &proto.GetProductGroupResponse{
			Success: false,
			Message: lib.StrPtr("valid ID must be provided"),
		}, nil
	}

	var pg ProductGroup
	if err := s.db.First(&pg, req.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &proto.GetProductGroupResponse{
				Success: false,
				Message: lib.StrPtr("product group not found"),
			}, nil
		}
		log.Printf("Database error fetching product group: %v", err)
		return &proto.GetProductGroupResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, nil
	}

	return &proto.GetProductGroupResponse{
		Success:      true,
		ProductGroup: s.productGroupToProto(pg),
	}, nil
}

func (s *POSHandler) ListProductGroups(ctx context.Context, req *proto.ListProductGroupsRequest) (*proto.ListProductGroupsResponse, error) {
	var productGroups []ProductGroup
	var total int64

	query := s.db.Model(&ProductGroup{}).Preload("Products")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	} else if req.ParentGroupId != nil {
		query = query.Where("parent_group = ?", req.GetParentGroupId())
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListProductGroupsResponse{
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
	if err := query.Offset(offset).Limit(pageSize).Find(&productGroups).Error; err != nil {
		return &proto.ListProductGroupsResponse{
			Success: false,
			Message: lib.StrPtr("database error"),
		}, err
	}

	protoProductGroups := make([]*proto.ProductGroup, len(productGroups))
	for i, pg := range productGroups {
		protoProductGroups[i] = s.productGroupToProto(pg)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListProductGroupsResponse{
		Success:       true,
		ProductGroups: protoProductGroups,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}
