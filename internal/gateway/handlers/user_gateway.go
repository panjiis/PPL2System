package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	proto "syntra-system/proto/protogen/user"

	"github.com/gin-gonic/gin"
)

type UserHTTPHandler struct {
	userClient proto.UserServiceClient
}

func NewUserHTTPHandler(userClient proto.UserServiceClient) *UserHTTPHandler {
	return &UserHTTPHandler{
		userClient: userClient,
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Username  string `json:"username" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6"`
	Firstname string `json:"firstname" binding:"required"`
	Lastname  string `json:"lastname" binding:"required"`
	RoleID    int32  `json:"role_id" binding:"required"`
}

type UpdateUserRequest struct {
	Email     *string `json:"email,omitempty"`
	Firstname *string `json:"firstname,omitempty"`
	Lastname  *string `json:"lastname,omitempty"`
	RoleID    *int32  `json:"role_id,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

type CreateEmployeeRequest struct {
	EmployeeName   string  `json:"employee_name" binding:"required"`
	Position       *string `json:"position,omitempty"`
	Phone          *string `json:"phone,omitempty"`
	Email          *string `json:"email,omitempty"`
	Address        *string `json:"address,omitempty"`
	HireDate       *string `json:"hire_date,omitempty"`
	BaseSalary     string  `json:"base_salary" binding:"required"`
	CommissionRate string  `json:"commission_rate" binding:"required"`
	CommissionType int32   `json:"commission_type" binding:"required"`
}

type UpdateEmployeeRequest struct {
	EmployeeName   *string `json:"employee_name,omitempty"`
	Position       *string `json:"position,omitempty"`
	Phone          *string `json:"phone,omitempty"`
	Email          *string `json:"email,omitempty"`
	Address        *string `json:"address,omitempty"`
	BaseSalary     *string `json:"base_salary,omitempty"`
	CommissionRate *string `json:"commission_rate,omitempty"`
	CommissionType *int32  `json:"commission_type,omitempty"`
	IsActive       *bool   `json:"is_active,omitempty"`
}

type CreateRoleRequest struct {
	RoleName    string  `json:"role_name" binding:"required"`
	AccessLevel int32   `json:"access_level" binding:"required"`
	Permissions *string `json:"permissions,omitempty"`
}

type ListUsersQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=10"`
	IsActive *bool  `form:"is_active,omitempty"`
	RoleID   *int32 `form:"role_id,omitempty"`
}

type ListEmployeesQuery struct {
	Page     int     `form:"page,default=1"`
	PageSize int     `form:"page_size,default=10"`
	IsActive *bool   `form:"is_active,omitempty"`
	Position *string `form:"position,omitempty"`
}

type ListRolesQuery struct {
	Page     int `form:"page,default=1"`
	PageSize int `form:"page_size,default=10"`
}

type ListStoreQuery struct {
	Page     int `form:"page,default=1"`
	PageSize int `form:"page_size,default=10"`
}

type CreateStoreRequest struct {
	Name                  string  `json:"name" binding:"required"`
	ImageURL              *string `json:"image_url,omitempty"`
	StorePreferences      *string `json:"store_preferences,omitempty"`      // JSON string
	ManagementPreferences *string `json:"management_preferences,omitempty"` // JSON string
	Address               *string `json:"address,omitempty"`
	Phone                 *string `json:"phone,omitempty"`
	City                  *string `json:"city,omitempty"`
	Country               *string `json:"country,omitempty"`
	PostalCode            *string `json:"postal_code,omitempty"`
	IsActive              *bool   `json:"is_active,omitempty"`
}

type UpdateStoreRequest struct {
	Name                  *string `json:"name,omitempty"`
	ImageURL              *string `json:"image_url,omitempty"`
	StorePreferences      *string `json:"store_preferences,omitempty"`
	ManagementPreferences *string `json:"management_preferences,omitempty"`
	Address               *string `json:"address,omitempty"`
	Phone                 *string `json:"phone,omitempty"`
	City                  *string `json:"city,omitempty"`
	Country               *string `json:"country,omitempty"`
	PostalCode            *string `json:"postal_code,omitempty"`
	IsActive              *bool   `json:"is_active,omitempty"`
}

type StoreResponse struct {
	ID                    int64     `json:"id"`
	Name                  string    `json:"name"`
	ImageURL              *string   `json:"image_url,omitempty"`
	StorePreferences      *string   `json:"store_preferences,omitempty"`
	ManagementPreferences *string   `json:"management_preferences,omitempty"`
	Address               *string   `json:"address,omitempty"`
	Phone                 *string   `json:"phone,omitempty"`
	City                  *string   `json:"city,omitempty"`
	Country               *string   `json:"country,omitempty"`
	PostalCode            *string   `json:"postal_code,omitempty"`
	IsActive              bool      `json:"is_active"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

func successResponse(message string, data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
}

func errorResponse(message string) APIResponse {
	return APIResponse{
		Success: false,
		Message: message,
	}
}

func successWithMetaResponse(message string, data interface{}, meta interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Message: message,
		Data:    data,
		Meta:    meta,
	}
}

// --- Authentication ---

func (h *UserHTTPHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.Authenticate(ctx, &proto.AuthenticateRequest{
		Username: req.Username,
		Password: req.Password,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Authentication service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusUnauthorized, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse(resp.Message, map[string]interface{}{
		"token":      resp.Token,
		"expires_at": resp.ExpiresAt,
		"user":       resp.User,
	}))
}

func (h *UserHTTPHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.CreateUser(ctx, &proto.CreateUserRequest{
		Username:  req.Username,
		Email:     req.Email,
		Password:  req.Password,
		Firstname: req.Firstname,
		Lastname:  req.Lastname,
		RoleId:    req.RoleID,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("User service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusCreated, successResponse(resp.Message, map[string]interface{}{
		"token":      resp.Token,
		"expires_at": resp.ExpiredAt,
		"user":       resp.User,
	}))
}

// --- User Management ---
func (h *UserHTTPHandler) GetUser(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid user ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.userClient.GetUser(ctx, &proto.GetUserRequest{
		Id: userID,
	})

	if err != nil {
		c.JSON(http.StatusNotFound, errorResponse("User not found"))
		return
	}

	c.JSON(http.StatusOK, successResponse("User retrieved successfully", resp.User))
}

func (h *UserHTTPHandler) UpdateUser(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid user ID"))
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.UpdateUser(ctx, &proto.UpdateUserRequest{
		Id:        userID,
		Email:     req.Email,
		Firstname: req.Firstname,
		Lastname:  req.Lastname,
		RoleId:    req.RoleID,
		IsActive:  req.IsActive,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("User service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse(resp.Message, resp.User))
}

func (h *UserHTTPHandler) ListUsers(c *gin.Context) {
	var query ListUsersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.ListUsers(ctx, &proto.ListUsersRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
		IsActive: query.IsActive,
		RoleId:   query.RoleID,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("User service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse(resp.Message, resp.Users, resp.Pagination))
}

// --- Employee Management ---
func (h *UserHTTPHandler) CreateEmployee(c *gin.Context) {
	var req CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.CreateEmployee(ctx, &proto.CreateEmployeeRequest{
		EmployeeName:   req.EmployeeName,
		Position:       req.Position,
		Phone:          req.Phone,
		Email:          req.Email,
		Address:        req.Address,
		HireDate:       req.HireDate,
		BaseSalary:     req.BaseSalary,
		CommissionRate: req.CommissionRate,
		CommissionType: proto.CommissionType(req.CommissionType),
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Employee service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusCreated, successResponse(resp.Message, resp.Employee))
}

func (h *UserHTTPHandler) GetEmployee(c *gin.Context) {
	idParam := c.Param("id")
	employeeID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid employee ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.userClient.GetEmployee(ctx, &proto.GetEmployeeRequest{
		Id: employeeID,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Employee service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusNotFound, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse(resp.Message, resp.Employee))
}

func (h *UserHTTPHandler) UpdateEmployee(c *gin.Context) {
	idParam := c.Param("id")
	employeeID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid employee ID"))
		return
	}

	var req UpdateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var commissionType *proto.CommissionType
	if req.CommissionType != nil {
		ct := proto.CommissionType(*req.CommissionType)
		commissionType = &ct
	}

	resp, err := h.userClient.UpdateEmployee(ctx, &proto.UpdateEmployeeRequest{
		Id:             employeeID,
		EmployeeName:   req.EmployeeName,
		Position:       req.Position,
		Phone:          req.Phone,
		Email:          req.Email,
		Address:        req.Address,
		BaseSalary:     req.BaseSalary,
		CommissionRate: req.CommissionRate,
		CommissionType: commissionType,
		IsActive:       req.IsActive,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Employee service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse(resp.Message, resp.Employee))
}

func (h *UserHTTPHandler) ListEmployees(c *gin.Context) {
	var query ListEmployeesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.ListEmployees(ctx, &proto.ListEmployeesRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
		IsActive: query.IsActive,
		Position: query.Position,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Employee service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse(resp.Message, resp.Employees, resp.Pagination))
}

// --- Role Management ---
func (h *UserHTTPHandler) CreateRole(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.CreateRole(ctx, &proto.CreateRoleRequest{
		RoleName:    req.RoleName,
		AccessLevel: req.AccessLevel,
		Permissions: req.Permissions,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Role service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusCreated, successResponse(resp.Message, resp.Role))
}

func (h *UserHTTPHandler) ListRoles(c *gin.Context) {
	var query ListRolesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.ListRoles(ctx, &proto.ListRolesRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Role service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse(resp.Message, resp.Roles, resp.Pagination))
}

func (h *UserHTTPHandler) UpdateRole(c *gin.Context) {
	var req proto.UpdateRoleRequest
	roleIDParam := c.Param("id")
	roleID, err := strconv.ParseInt(roleIDParam, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, "Invalid role ID")
		return
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	req.Id = int32(roleID)

	resp, err := h.userClient.UpdateRole(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, "Failed to update warehouse: "+err.Error())
		return
	}
	if !resp.Success {
		c.JSON(http.StatusBadRequest, resp.Message)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"role": resp.Role,
	})
}

func (h *UserHTTPHandler) GetRole(c *gin.Context) {
	idParam := c.Param("id")
	roleID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid role ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.userClient.GetRole(ctx, &proto.GetRoleRequest{Id: int32(roleID)})

	if err != nil {
		c.JSON(http.StatusNotFound, errorResponse("Role not found"))
		return
	}

	c.JSON(http.StatusOK, successResponse("Role retrieved successfully", resp))
}

// -- Store Management --
func (h *UserHTTPHandler) CreateStore(c *gin.Context) {
	var req CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.CreateStore(ctx, &proto.CreateStoreRequest{
		Name:                  req.Name,
		ImageUrl:              req.ImageURL,
		StorePreferences:      req.StorePreferences,
		ManagementPreferences: req.ManagementPreferences,
		Address:               req.Address,
		Phone:                 req.Phone,
		City:                  req.City,
		Country:               req.Country,
		PostalCode:            req.PostalCode,
		IsActive:              req.IsActive != nil && *req.IsActive,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Store Management service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusCreated, successResponse(resp.Message, resp.Store))
}

func (h *UserHTTPHandler) ListStores(c *gin.Context) {
	var query ListStoreQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.ListStores(ctx, &proto.ListStoreRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Role service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(resp.Message))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse(resp.Message, resp.Stores, resp.Pagination))
}

func (h *UserHTTPHandler) UpdateStore(c *gin.Context) {
	var req proto.UpdateStoreRequest
	storeIDParam := c.Param("id")
	storeID, err := strconv.ParseInt(storeIDParam, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, "Invalid store ID")
		return
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	req.Id = int64(storeID)

	resp, err := h.userClient.UpdateStore(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, "Failed to update warehouse: "+err.Error())
		return
	}
	if !resp.Success {
		c.JSON(http.StatusBadRequest, resp.Message)
		return
	}
	c.JSON(http.StatusOK, successResponse(resp.Message, resp.Store))
}

func (h *UserHTTPHandler) GetStore(c *gin.Context) {
	idParam := c.Param("id")
	storeID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid store ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.userClient.GetStore(ctx, &proto.GetStoreRequest{Id: int64(storeID)})

	if err != nil {
		c.JSON(http.StatusNotFound, errorResponse("Store not found"))
		return
	}

	c.JSON(http.StatusOK, successResponse(resp.Message, resp.Store))
}
