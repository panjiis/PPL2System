package handler

import (
	"context"
	"errors"
	"strconv"
	proto "syntra-system/proto/protogen/user"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func (s *UserHandler) CreateStore(ctx context.Context, req *proto.CreateStoreRequest) (*proto.CreateStoreResponse, error) {
	store := Store{
		Name:                  req.GetName(),
		ImageURL:              req.ImageUrl,
		StorePreferences:      req.StorePreferences,
		ManagementPreferences: req.ManagementPreferences,
		Address:               req.Address,
		Phone:                 req.Phone,
		City:                  req.City,
		Country:               req.Country,
		PostalCode:            req.PostalCode,
		IsActive:              req.GetIsActive(),
	}

	if err := s.db.Create(&store).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create store: %v", err)
	}

	return &proto.CreateStoreResponse{
		Success: true,
		Message: "Store created successfully",
		Store:   s.storeToProto(store),
	}, nil
}

func (s *UserHandler) GetStore(ctx context.Context, req *proto.GetStoreRequest) (*proto.GetStoreResponse, error) {
	var store Store
	if err := s.db.First(&store, req.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "store not found")
		}
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	return &proto.GetStoreResponse{
		Success: true,
		Message: "Store retrieved successfully",
		Store:   s.storeToProto(store),
	}, nil
}

func (s *UserHandler) UpdateStore(ctx context.Context, req *proto.UpdateStoreRequest) (*proto.UpdateStoreResponse, error) {
	var store Store
	if err := s.db.First(&store, req.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "store not found")
		}
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	if req.Name != nil {
		store.Name = *req.Name
	}
	if req.ImageUrl != nil {
		store.ImageURL = req.ImageUrl
	}
	if req.StorePreferences != nil {
		store.StorePreferences = req.StorePreferences
	}
	if req.ManagementPreferences != nil {
		store.ManagementPreferences = req.ManagementPreferences
	}
	if req.Address != nil {
		store.Address = req.Address
	}
	if req.Phone != nil {
		store.Phone = req.Phone
	}
	if req.City != nil {
		store.City = req.City
	}
	if req.Country != nil {
		store.Country = req.Country
	}
	if req.PostalCode != nil {
		store.PostalCode = req.PostalCode
	}
	if req.IsActive != nil {
		store.IsActive = *req.IsActive
	}

	if err := s.db.Save(&store).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update store: %v", err)
	}

	return &proto.UpdateStoreResponse{
		Success: true,
		Message: "Store updated successfully",
		Store:   s.storeToProto(store),
	}, nil
}

func (s *UserHandler) ListStore(ctx context.Context, req *proto.ListStoreRequest) (*proto.ListStoreResponse, error) {
	var stores []Store
	query := s.db.Model(&Store{})

	var totalCount int64
	query.Count(&totalCount)

	pageSize := req.Pagination.GetPageSize()
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := 0
	if token := req.Pagination.GetPageToken(); token != "" {
		if i, err := strconv.Atoi(token); err == nil {
			offset = i
		}
	}

	query.Limit(int(pageSize)).Offset(offset).Find(&stores)

	var nextPageToken string
	if int64(offset+int(pageSize)) < totalCount {
		nextPageToken = strconv.Itoa(offset + int(pageSize))
	}

	protoStores := make([]*proto.Store, len(stores))
	for i, store := range stores {
		protoStores[i] = s.storeToProto(store)
	}

	return &proto.ListStoreResponse{
		Success: true,
		Message: "Stores retrieved successfully",
		Stores:  protoStores,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(totalCount),
		},
	}, nil
}
