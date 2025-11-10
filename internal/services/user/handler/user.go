package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"gorm.io/gorm"

	lib "syntra-system/internal/services/user"
	proto "syntra-system/proto/protogen/user"
)

// --- User Management ---
func (s *UserHandler) GetUser(ctx context.Context, req *proto.GetUserRequest) (*proto.GetUserResponse, error) {
	var user User
	if err := s.db.Preload("Role").First(&user, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetUserResponse{}, fmt.Errorf("user not found")
		}
		return &proto.GetUserResponse{}, err
	}

	return &proto.GetUserResponse{
		User: s.userToProto(user),
	}, nil
}

func (s *UserHandler) UpdateUser(ctx context.Context, req *proto.UpdateUserRequest) (*proto.UpdateUserResponse, error) {
	var user User
	if err := s.db.Preload("Role").First(&user, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.UpdateUserResponse{
				Success: false,
				Message: "user not found",
			}, nil
		}
		return &proto.UpdateUserResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	updates := make(map[string]interface{})

	if req.Email != nil {
		updates["email"] = req.GetEmail()
	}
	if req.Firstname != nil {
		updates["firstname"] = req.GetFirstname()
	}
	if req.Lastname != nil {
		updates["lastname"] = req.GetLastname()
	}
	if req.RoleId != nil {
		var role Role
		if err := s.db.First(&role, req.GetRoleId()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &proto.UpdateUserResponse{
					Success: false,
					Message: "Role not found",
				}, nil
			}
			return &proto.UpdateUserResponse{
				Success: false,
				Message: "invalid role specified",
			}, err
		}
		updates["role_id"] = req.GetRoleId()
	}
	if req.IsActive != nil {
		updates["is_active"] = req.GetIsActive()
	}

	if len(updates) > 0 {
		result := s.db.Model(&user).Select(lib.GetUpdateColumns(updates)).Updates(updates)
		if result.Error != nil {
			log.Printf("Database update error: %v", result.Error)
			return &proto.UpdateUserResponse{
				Success: false,
				Message: "error updating user",
			}, result.Error
		}
	}

	var updatedUser User
	if err := s.db.Preload("Role").First(&updatedUser, user.ID).Error; err != nil {
		return &proto.UpdateUserResponse{
			Success: false,
			Message: "error fetching updated user",
		}, err
	}

	s.InvalidateUserCaches(ctx, user.ID)

	return &proto.UpdateUserResponse{
		Success: true,
		Message: "user updated successfully",
		User:    s.userToProto(updatedUser),
	}, nil
}

func (s *UserHandler) ListUsers(ctx context.Context, req *proto.ListUsersRequest) (*proto.ListUsersResponse, error) {
	var users []User
	var total int64

	query := s.db.Model(&User{}).Preload("Role")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.RoleId != nil {
		query = query.Where("role_id = ?", req.GetRoleId())
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListUsersResponse{
			Success: false,
			Message: "database error",
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
	if err := query.Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return &proto.ListUsersResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	protoUsers := make([]*proto.User, len(users))
	for i, user := range users {
		protoUsers[i] = s.userToProto(user)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListUsersResponse{
		Success: true,
		Message: "users retrieved successfully",
		Users:   protoUsers,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}
