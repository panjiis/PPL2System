package handler

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	sysutils "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/user"
)

const (
	USER_CACHE_PREFIX       = "user:"
	USER_EMPLOYEE_CACHE_KEY = "user:employee"
	ROLE_CACHE_KEY          = "roles:list"
	CACHE_TTL_SHORT         = 5 * time.Minute
	CACHE_TTL_MEDIUM        = 30 * time.Minute
	CACHE_TTL_LONG          = 2 * time.Hour
)

// --- Helpers ---
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

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

func timeNowOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// --- GORM Models ---
type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Username  string `gorm:"uniqueIndex;not null"`
	Email     string `gorm:"uniqueIndex;not null"`
	Password  string `gorm:"not null"`
	Firstname string `gorm:"not null"`
	Lastname  string `gorm:"not null"`
	RoleID    int32  `gorm:"not null"`
	Role      Role   `gorm:"foreignKey:RoleID"`
	IsActive  bool   `gorm:"default:true"`
	LastLogin *time.Time
	CreatedAt *time.Time `gorm:"autoCreateTime"`
	UpdatedAt *time.Time `gorm:"autoUpdateTime"`
}

type Role struct {
	ID          int32      `gorm:"primaryKey;autoIncrement"`
	RoleName    string     `gorm:"uniqueIndex;not null"`
	AccessLevel int32      `gorm:"not null"`
	Permissions string     `gorm:"type:text"`
	CreatedAt   *time.Time `gorm:"autoCreateTime"`
	UpdatedAt   *time.Time `gorm:"autoUpdateTime"`
}

type Employee struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	EmployeeName   string `gorm:"not null"`
	Position       string `gorm:"column:position"`
	Phone          string
	Email          string
	Address        string `gorm:"type:text"`
	HireDate       string
	BaseSalary     string     `gorm:"not null"`
	CommissionRate string     `gorm:"not null"`
	CommissionType int32      `gorm:"not null"`
	IsActive       bool       `gorm:"default:true"`
	CreatedAt      *time.Time `gorm:"autoCreateTime"`
	UpdatedAt      *time.Time `gorm:"autoUpdateTime"`

	CommissionTiers []CommissionTier `gorm:"foreignKey:EmployeeID"`
}

type CommissionTier struct {
	ID             int32  `gorm:"primaryKey;autoIncrement"`
	EmployeeID     int64  `gorm:"not null"`
	MinSalesAmount string `gorm:"not null"`
	MaxSalesAmount string
	CommissionRate string     `gorm:"not null"`
	CreatedAt      *time.Time `gorm:"autoCreateTime"`
	UpdatedAt      *time.Time `gorm:"autoUpdateTime"`
}

// --- Handler ---
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

func (s *UserHandler) InvalidateUserCaches(ctx context.Context, userIDs ...int64) {
	_ = s.redis.Del(ctx, USER_EMPLOYEE_CACHE_KEY, ROLE_CACHE_KEY)

	for _, id := range userIDs {
		cacheKey := fmt.Sprintf("%s%d", USER_CACHE_PREFIX, id)
		_ = s.redis.Del(ctx, cacheKey)
	}
}

// --- Conversion Helpers ---
func (s *UserHandler) roleToProto(role Role) *proto.Role {
	return &proto.Role{
		Id:          role.ID,
		RoleName:    role.RoleName,
		AccessLevel: role.AccessLevel,
		Permissions: strPtr(role.Permissions),
		CreatedAt:   timestamppb.New(timeNowOrZero(role.CreatedAt)),
		UpdatedAt:   timestamppb.New(timeNowOrZero(role.UpdatedAt)),
	}
}

func (s *UserHandler) userToProto(user User) *proto.User {
	var roleProto *proto.Role
	if user.Role.ID != 0 {
		roleProto = s.roleToProto(user.Role)
	}

	var lastLoginProto *timestamppb.Timestamp
	if user.LastLogin != nil {
		lastLoginProto = timestamppb.New(*user.LastLogin)
	}

	return &proto.User{
		Id:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Password:  "",
		Firstname: user.Firstname,
		Lastname:  user.Lastname,
		RoleId:    user.RoleID,
		IsActive:  user.IsActive,
		LastLogin: lastLoginProto,
		CreatedAt: timestamppb.New(timeNowOrZero(user.CreatedAt)),
		UpdatedAt: timestamppb.New(timeNowOrZero(user.UpdatedAt)),
		Role:      roleProto,
	}
}

func (s *UserHandler) employeeToProto(employee Employee) *proto.Employee {
	var commissionTiers []*proto.CommissionTier
	for _, tier := range employee.CommissionTiers {
		commissionTiers = append(commissionTiers, &proto.CommissionTier{
			Id:             tier.ID,
			EmployeeId:     tier.EmployeeID,
			MinSalesAmount: tier.MinSalesAmount,
			MaxSalesAmount: strPtr(tier.MaxSalesAmount),
			CommissionRate: tier.CommissionRate,
			CreatedAt:      timestamppb.New(timeNowOrZero(tier.CreatedAt)),
			UpdatedAt:      timestamppb.New(timeNowOrZero(tier.UpdatedAt)),
		})
	}

	return &proto.Employee{
		Id:              employee.ID,
		EmployeeName:    employee.EmployeeName,
		Position:        strPtr(employee.Position),
		Phone:           strPtr(employee.Phone),
		Email:           strPtr(employee.Email),
		Address:         strPtr(employee.Address),
		HireDate:        strPtr(employee.HireDate),
		BaseSalary:      employee.BaseSalary,
		CommissionRate:  employee.CommissionRate,
		CommissionType:  proto.CommissionType(employee.CommissionType),
		IsActive:        employee.IsActive,
		CreatedAt:       timestamppb.New(timeNowOrZero(employee.CreatedAt)),
		UpdatedAt:       timestamppb.New(timeNowOrZero(employee.UpdatedAt)),
		CommissionTiers: commissionTiers,
	}
}

// --- Authentication & Registration ---
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

	if req.Email != nil {
		user.Email = req.GetEmail()
	}
	if req.Firstname != nil {
		user.Firstname = req.GetFirstname()
	}
	if req.Lastname != nil {
		user.Lastname = req.GetLastname()
	}
	if req.RoleId != nil {
		var role Role
		if err := s.db.First(&role, req.GetRoleId()).Error; err != nil {
			return &proto.UpdateUserResponse{
				Success: false,
				Message: "invalid role specified",
			}, nil
		}
		user.RoleID = req.GetRoleId()
	}
	if req.IsActive != nil {
		user.IsActive = req.GetIsActive()
	}

	if err := s.db.Save(&user).Error; err != nil {
		return &proto.UpdateUserResponse{
			Success: false,
			Message: "error updating user",
		}, err
	}

	s.db.First(&user.Role, user.RoleID)

	s.InvalidateUserCaches(ctx, user.ID)

	return &proto.UpdateUserResponse{
		Success: true,
		Message: "user updated successfully",
		User:    s.userToProto(user),
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

// --- Role Management ---
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

// --- Employee Management ---
func (s *UserHandler) CreateEmployee(ctx context.Context, req *proto.CreateEmployeeRequest) (*proto.CreateEmployeeResponse, error) {
	if req.GetEmployeeName() == "" || req.GetBaseSalary() == "" || req.GetCommissionRate() == "" {
		return &proto.CreateEmployeeResponse{
			Success: false,
			Message: "employee name, base salary, and commission rate are required",
		}, nil
	}

	newEmployee := Employee{
		EmployeeName:   req.GetEmployeeName(),
		BaseSalary:     req.GetBaseSalary(),
		CommissionRate: req.GetCommissionRate(),
		CommissionType: int32(req.GetCommissionType()),
		IsActive:       true,
	}

	if req.Position != nil {
		newEmployee.Position = req.GetPosition()
	}
	if req.Phone != nil {
		newEmployee.Phone = req.GetPhone()
	}
	if req.Email != nil {
		newEmployee.Email = req.GetEmail()
	}
	if req.Address != nil {
		newEmployee.Address = req.GetAddress()
	}
	if req.HireDate != nil {
		newEmployee.HireDate = req.GetHireDate()
	}

	if err := s.db.Create(&newEmployee).Error; err != nil {
		return &proto.CreateEmployeeResponse{
			Success: false,
			Message: "error creating employee",
		}, err
	}

	s.InvalidateUserCaches(ctx)

	return &proto.CreateEmployeeResponse{
		Success:  true,
		Message:  "employee created successfully",
		Employee: s.employeeToProto(newEmployee),
	}, nil
}

func (s *UserHandler) GetEmployee(ctx context.Context, req *proto.GetEmployeeRequest) (*proto.GetEmployeeResponse, error) {
	var employee Employee
	if err := s.db.Preload("CommissionTiers").First(&employee, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.GetEmployeeResponse{
				Success: false,
				Message: "employee not found",
			}, nil
		}
		return &proto.GetEmployeeResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	return &proto.GetEmployeeResponse{
		Success:  true,
		Message:  "employee retrieved successfully",
		Employee: s.employeeToProto(employee),
	}, nil
}

func (s *UserHandler) UpdateEmployee(ctx context.Context, req *proto.UpdateEmployeeRequest) (*proto.UpdateEmployeeResponse, error) {
	var employee Employee
	if err := s.db.Preload("CommissionTiers").First(&employee, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &proto.UpdateEmployeeResponse{
				Success: false,
				Message: "employee not found",
			}, nil
		}
		return &proto.UpdateEmployeeResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	if req.EmployeeName != nil {
		employee.EmployeeName = req.GetEmployeeName()
	}
	if req.Position != nil {
		employee.Position = req.GetPosition()
	}
	if req.Phone != nil {
		employee.Phone = req.GetPhone()
	}
	if req.Email != nil {
		employee.Email = req.GetEmail()
	}
	if req.Address != nil {
		employee.Address = req.GetAddress()
	}
	if req.BaseSalary != nil {
		employee.BaseSalary = req.GetBaseSalary()
	}
	if req.CommissionRate != nil {
		employee.CommissionRate = req.GetCommissionRate()
	}
	if req.CommissionType != nil {
		employee.CommissionType = int32(req.GetCommissionType())
	}
	if req.IsActive != nil {
		employee.IsActive = req.GetIsActive()
	}

	if err := s.db.Save(&employee).Error; err != nil {
		return &proto.UpdateEmployeeResponse{
			Success: false,
			Message: "error updating employee",
		}, err
	}

	s.InvalidateUserCaches(ctx)

	return &proto.UpdateEmployeeResponse{
		Success:  true,
		Message:  "employee updated successfully",
		Employee: s.employeeToProto(employee),
	}, nil
}

func (s *UserHandler) ListEmployees(ctx context.Context, req *proto.ListEmployeesRequest) (*proto.ListEmployeesResponse, error) {
	var employees []Employee
	var total int64

	query := s.db.Model(&Employee{}).Preload("CommissionTiers")

	if req.IsActive != nil {
		query = query.Where("is_active = ?", req.GetIsActive())
	}
	if req.Position != nil && req.GetPosition() != "" {
		query = query.Where("position ILIKE ?", "%"+req.GetPosition()+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return &proto.ListEmployeesResponse{
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
	if err := query.Offset(offset).Limit(pageSize).Find(&employees).Error; err != nil {
		return &proto.ListEmployeesResponse{
			Success: false,
			Message: "database error",
		}, err
	}

	protoEmployees := make([]*proto.Employee, len(employees))
	for i, emp := range employees {
		protoEmployees[i] = s.employeeToProto(emp)
	}

	nextPageToken := ""
	if int64(pageNumber*pageSize) < total {
		nextPageToken = strconv.Itoa(pageNumber + 1)
	}

	return &proto.ListEmployeesResponse{
		Success:   true,
		Message:   "employees retrieved successfully",
		Employees: protoEmployees,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		},
	}, nil
}
