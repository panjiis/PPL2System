package handler

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	sysutils "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/user"
)

func (s *UserHandler) CreateUser(ctx context.Context, req *proto.CreateUserRequest) (*proto.CreateUserResponse, error) {
	if req.GetUsername() == "" || req.GetEmail() == "" || req.GetPassword() == "" {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "username, email, and password are required",
		}, nil
	}

	var existingUser User
	if err := s.db.Where("username = ? OR email = ?", req.GetUsername(), req.GetEmail()).First(&existingUser).Error; err == nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "username or email already exists",
		}, nil
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "database error while checking existing user",
		}, err
	}

	var role Role
	if err := s.db.First(&role, req.GetRoleId()).Error; err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "invalid role specified",
		}, nil
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(req.GetPassword()), bcrypt.DefaultCost)
	if err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "error hashing password",
		}, err
	}

	newUser := User{
		Username:  req.GetUsername(),
		Email:     req.GetEmail(),
		Password:  string(pwHash),
		Firstname: req.GetFirstname(),
		Lastname:  req.GetLastname(),
		RoleID:    req.GetRoleId(),
		IsActive:  true,
	}

	if err := s.db.Create(&newUser).Error; err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "error creating user",
		}, err
	}

	s.db.First(&newUser.Role, newUser.RoleID)

	token, exp, err := sysutils.GenerateToken(newUser.ID, newUser.Username, 24*time.Hour)
	if err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "error generating token",
		}, err
	}

	s.InvalidateUserCaches(ctx)

	return &proto.CreateUserResponse{
		Success:   true,
		Message:   "user registered successfully",
		Token:     token,
		ExpiredAt: timestamppb.New(exp),
		User:      s.userToProto(newUser),
	}, nil
}

func (s *UserHandler) Authenticate(ctx context.Context, req *proto.AuthenticateRequest) (*proto.AuthenticateResponse, error) {
	if req.GetUsername() == "" || req.GetPassword() == "" {
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "username and password are required",
		}, nil
	}

	var user User
	if err := s.db.Preload("Role").Where("username = ? AND is_active = ?", req.GetUsername(), true).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.AuthenticateResponse{
				Success: false,
				Message: "invalid username or password",
			}, nil
		}
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.GetPassword())); err != nil {
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "invalid username or password",
		}, nil
	}

	token, exp, err := sysutils.GenerateToken(user.ID, user.Username, 24*time.Hour)
	if err != nil {
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "error generating token",
		}, err
	}

	now := time.Now()
	user.LastLogin = &now
	s.db.Save(&user)

	s.InvalidateUserCaches(ctx, user.ID)

	return &proto.AuthenticateResponse{
		Success:   true,
		Message:   "login successful",
		Token:     token,
		ExpiresAt: timestamppb.New(exp),
		User:      s.userToProto(user),
	}, nil
}
