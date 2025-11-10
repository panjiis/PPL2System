package handler

import (
	"context"
	"strconv"
	proto "syntra-system/proto/protogen/user"

	"gorm.io/gorm"
)

func (s *UserHandler) CreateRole(ctx context.Context, req *proto.CreateRoleRequest) (*proto.CreateRoleResponse, error) {
	if req.GetRoleName() == "" {
		return &proto.CreateRoleResponse{
			Success: false,
			Message: "role name is required",
		}, nil
	}

	var existingRole Role
	if err := s.db.Where("role_name = ?", req.GetRoleName()).First(&existingRole).Error; err == nil {
		return &proto.CreateRoleResponse{
			Success: false,
			Message: "role name already exists",
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateRoleResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	newRole := Role{
		RoleName:    req.GetRoleName(),
		AccessLevel: req.GetAccessLevel(),
		Permissions: req.GetPermissions(),
	}

	if err := s.db.Create(&newRole).Error; err != nil {
		return &proto.CreateRoleResponse{
			Success: false,
			Message: "error creating role",
		}, err
	}

	_ = s.redis.Del(ctx, ROLE_CACHE_KEY)

	return &proto.CreateRoleResponse{
		Success: true,
		Message: "role created successfully",
		Role:    s.roleToProto(newRole),
	}, nil
}

func (s *UserHandler) ListRoles(ctx context.Context, req *proto.ListRolesRequest) (*proto.ListRolesResponse, error) {
	var roles []Role
	var total int64

	query := s.db.Model(&Role{})

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListRolesResponse{
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
	if err := query.Offset(offset).Limit(pageSize).Find(&roles).Error; err != nil {
		return &proto.ListRolesResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	protoRoles := make([]*proto.Role, len(roles))
	for i, role := range roles {
		protoRoles[i] = s.roleToProto(role)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListRolesResponse{
		Success: true,
		Message: "roles retrieved successfully",
		Roles:   protoRoles,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}

func (s *UserHandler) UpdateRole(ctx context.Context, req *proto.UpdateRoleRequest) (*proto.CreateRoleResponse, error) {
	var role Role
	if err := s.db.First(&role, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.CreateRoleResponse{
				Success: false,
				Message: "role not found",
			}, nil
		}
		return &proto.CreateRoleResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	role.RoleName = req.GetRoleName()
	role.AccessLevel = req.GetAccessLevel()
	role.Permissions = req.GetPermissions()

	if err := s.db.Save(&role).Error; err != nil {
		return &proto.CreateRoleResponse{
			Success: false,
			Message: "error updating role",
		}, err
	}

	_ = s.redis.Del(ctx, ROLE_CACHE_KEY)

	return &proto.CreateRoleResponse{
		Success: true,
		Message: "role updated successfully",
		Role:    s.roleToProto(role),
	}, nil
}

func (s *UserHandler) GetRole(ctx context.Context, req *proto.GetRoleRequest) (*proto.Role, error) {
	var role Role
	if err := s.db.First(&role, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.Role{}, nil
		}
		return &proto.Role{}, err
	}

	return s.roleToProto(role), nil
}
