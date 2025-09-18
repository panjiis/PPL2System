package handler

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"

	sysutils "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/user"

	"gorm.io/gorm"
)

const (
	USER_CACHE_PREFIX       = "user:"
	USER_EMPLOYEE_CACHE_KEY = "user:employee"

	CACHE_TTL_SHORT  = 5 * time.Minute
	CACHE_TTL_MEDIUM = 30 * time.Minute
	CACHE_TTL_LONG   = 2 * time.Hour
)

type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: %v", value)
	}

	return json.Unmarshal(bytes, a)
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	return json.Marshal(a)
}

type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Username  string `gorm:"uniqueIndex"`
	Email     string
	Password  string
	Firstname string
	Lastname  string
	RoleID    int32
	Role      Role `gorm:"foreignKey:RoleID"`
	IsActive  bool
	LastLogin *timestamppb.Timestamp
	CreatedAt *timestamppb.Timestamp `gorm:"autoCreateTime"`
	UpdatedAt *timestamppb.Timestamp `gorm:"autoUpdateTime"`
}

type Role struct {
	ID          int32 `gorm:"primaryKey:autoIncrement"`
	RoleName    string
	AccessLevel int32
	Permissions string
	CreatedAt   *timestamppb.Timestamp `gorm:"autoCreateTime"`
	UpdatedAt   *timestamppb.Timestamp `gorm:"autoUpdateTime"`
}

type Employee struct {
	ID             int32 `gorm:"primaryKey:autoIncrement"`
	EmployeeName   string
	Postition      string
	Phone          string
	Email          string
	Address        string `gorm:"text"`
	HireDate       string
	BaseSalary     string
	CommissionRate string
	CommissionType int32
	CreatedAt      *timestamppb.Timestamp `gorm:"autoCreateTime"`
	UpdatedAt      *timestamppb.Timestamp `gorm:"autoUpdateTime"`
}

type UserHandler struct {
	proto.UnimplementedUserServiceServer
	db    *gorm.DB
	redis *redis.Client
}

func NewUserHandler(db *gorm.DB, redisClient *redis.Client) *UserHandler {
	return &UserHandler{
		db:    db,
		redis: redisClient,
	}
}

func (s *UserHandler) InvalidateUserCaches(ctx context.Context, ID ...int64) {
	s.redis.Del(ctx, USER_EMPLOYEE_CACHE_KEY)

	for _, k := range ID {
		cacheKey := fmt.Sprintf("%s%d", USER_CACHE_PREFIX, k)
		s.redis.Del(ctx, cacheKey)
	}
}

func (s *UserHandler) Register(ctx context.Context, req *proto.CreateUserRequest) *proto.CreateUserResponse {
	var existingUser User
	if err := s.db.Where("username = ? OR email = ?", req.Username, req.Email).First(&existingUser).Error; err == nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "Username or email already exists",
		}
	} else if err != gorm.ErrRecordNotFound {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "Database error",
		}
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "Error hashing password",
		}
	}

	newUser := User{
		Username:  req.Username,
		Email:     req.Email,
		Password:  string(pwHash),
		Firstname: req.Firstname,
		Lastname:  req.Lastname,
		RoleID:    req.RoleId,
		IsActive:  true,
	}

	if err := s.db.Create(&newUser).Error; err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "Error creating user",
		}
	}

	token, exp, err := sysutils.GenerateToken(newUser.ID, newUser.Username, 24*time.Hour)
	if err != nil {
		return &proto.CreateUserResponse{
			Success: false,
			Message: "Error generating token",
		}
	}

	s.InvalidateUserCaches(ctx)

	return &proto.CreateUserResponse{
		Success:   true,
		Message:   "User registered successfully",
		Token:     token,
		ExpiredAt: timestamppb.New(exp),
		User: &proto.User{
			Id:        newUser.ID,
			Username:  newUser.Username,
			Firstname: newUser.Firstname,
			Lastname:  newUser.Lastname,
			RoleId:    newUser.RoleID,
			IsActive:  newUser.IsActive,
			LastLogin: newUser.LastLogin,
			Role: &proto.Role{
				Id:          newUser.Role.ID,
				RoleName:    newUser.Role.RoleName,
				AccessLevel: newUser.Role.AccessLevel,
				Permissions: &newUser.Role.Permissions,
			},
		},
	}
}

func (s *UserHandler) Login(ctx context.Context, req *proto.AuthenticateRequest) *proto.AuthenticateResponse {
	var user User
	if err := s.db.Preload("Role").Where("username = ?", req.Username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.AuthenticateResponse{
				Success: false,
				Message: "Invalid username or password",
			}
		}
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "Database error",
		}
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "Invalid username or password",
		}
	}

	token, exp, err := sysutils.GenerateToken(user.ID, user.Username, 24*time.Hour)
	if err != nil {
		return &proto.AuthenticateResponse{
			Success: false,
			Message: "Error generating token",
		}
	}

	user.LastLogin = timestamppb.New(time.Now())
	s.db.Save(&user)

	s.InvalidateUserCaches(ctx, user.ID)

	return &proto.AuthenticateResponse{
		Success:   true,
		Message:   "Login successful",
		Token:     token,
		ExpiresAt: timestamppb.New(exp),
		User: &proto.User{
			Id:        user.ID,
			Username:  user.Username,
			Firstname: user.Firstname,
			Lastname:  user.Lastname,
			RoleId:    user.RoleID,
			IsActive:  user.IsActive,
			LastLogin: user.LastLogin,
			Role: &proto.Role{
				Id:          user.Role.ID,
				RoleName:    user.Role.RoleName,
				AccessLevel: user.Role.AccessLevel,
				Permissions: &user.Role.Permissions,
			},
		},
	}
}
