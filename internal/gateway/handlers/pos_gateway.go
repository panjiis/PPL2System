package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	proto "syntra-system/proto/protogen/pos"

	"github.com/gin-gonic/gin"
)

type POSHTTPHandler struct {
	posClient proto.POSServiceClient
}

func NewPOSHTTPHandler(posClient proto.POSServiceClient) *POSHTTPHandler {
	return &POSHTTPHandler{
		posClient: posClient,
	}
}

// Request structs
type CreateCartRequest struct {
	CashierID int64 `json:"cashier_id" binding:"required"`
}

type AddItemToCartRequest struct {
	CartID            string `json:"cart_id" binding:"required"`
	ProductID         int32  `json:"product_id" binding:"required"`
	Quantity          int32  `json:"quantity" binding:"required,min=1"`
	ServingEmployeeID *int64 `json:"serving_employee_id,omitempty"`
}

type ApplyDiscountRequest struct {
	CartID     string   `json:"cart_id" binding:"required"`
	DiscountID int32    `json:"discount_id" binding:"required"`
	ItemIDs    []string `json:"item_ids,omitempty"`
}

type ValidateDiscountRequest struct {
	DiscountID int32  `json:"discount_id" binding:"required"`
	ProductID  *int32 `json:"product_id,omitempty"`
	Quantity   *int32 `json:"quantity,omitempty"`
}

type CreateOrderItemRequest struct {
	ProductID         int32  `json:"product_id" binding:"required"`
	Quantity          int32  `json:"quantity" binding:"required,min=1"`
	ServingEmployeeID *int64 `json:"serving_employee_id,omitempty"`
	DiscountID        *int32 `json:"discount_id,omitempty"`
}

type CreateOrderRequest struct {
	DocumentNumber string                   `json:"document_number" binding:"required"`
	CashierID      int64                    `json:"cashier_id" binding:"required"`
	DocumentType   int32                    `json:"document_type" binding:"required"`
	OrderItems     []CreateOrderItemRequest `json:"order_items" binding:"required,min=1"`
	AdditionalInfo *string                  `json:"additional_info,omitempty"`
	Notes          *string                  `json:"notes,omitempty"`
}

type CreateOrderFromCartRequest struct {
	CartID         string  `json:"cart_id" binding:"required"`
	DocumentNumber string  `json:"document_number" binding:"required"`
	AdditionalInfo *string `json:"additional_info,omitempty"`
	Notes          *string `json:"notes,omitempty"`
}

type ProcessPaymentRequest struct {
	OrderID       int64  `json:"order_id" binding:"required"`
	PaymentTypeID int32  `json:"payment_type_id" binding:"required"`
	PaidAmount    string `json:"paid_amount" binding:"required"`
}

type VoidOrderRequest struct {
	ID       int64  `json:"id" binding:"required"`
	VoidedBy int64  `json:"voided_by" binding:"required"`
	Reason   string `json:"reason" binding:"required"`
}

type ReturnOrderRequest struct {
	OriginalOrderID int64   `json:"original_order_id" binding:"required"`
	ProcessedBy     int64   `json:"processed_by" binding:"required"`
	ItemIDs         []int64 `json:"item_ids" binding:"required,min=1"`
	Reason          *string `json:"reason,omitempty"`
}

// Query structs
type ListProductsQuery struct {
	Page           int     `form:"page,default=1"`
	PageSize       int     `form:"page_size,default=10"`
	IsActive       *bool   `form:"is_active,omitempty"`
	ProductGroupID *int32  `form:"product_group_id,omitempty"`
	SearchTerm     *string `form:"search,omitempty"`
}

type ListProductGroupsQuery struct {
	Page          int    `form:"page,default=1"`
	PageSize      int    `form:"page_size,default=10"`
	IsActive      *bool  `form:"is_active,omitempty"`
	ParentGroupID *int32 `form:"parent_group_id,omitempty"`
}

type ListDiscountsQuery struct {
	Page       int     `form:"page,default=1"`
	PageSize   int     `form:"page_size,default=10"`
	IsActive   *bool   `form:"is_active,omitempty"`
	ProductID  *int32  `form:"product_id,omitempty"`
	SearchTerm *string `form:"search,omitempty"`
}

type ListOrdersQuery struct {
	Page         int                 `form:"page,default=1"`
	PageSize     int                 `form:"page_size,default=20"`
	CashierID    *int64              `form:"cashier_id,omitempty"`
	DocumentType *proto.DocumentType `form:"document_type,omitempty"`
	PaidStatus   *proto.PaidStatus   `form:"paid_status,omitempty"`
	StartDate    string              `form:"start_date,omitempty"`
	EndDate      string              `form:"end_date,omitempty"`
}

// --- Product Handlers ---

func (h *POSHTTPHandler) GetProduct(c *gin.Context) {
	idParam := c.Param("id")
	productID, err := strconv.ParseInt(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid product ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.GetProduct(ctx, &proto.GetProductRequest{
		Id: int32(productID),
	})

	if err != nil || !resp.Success {
		msg := "Product not found"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusNotFound, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Product retrieved successfully", resp.Product))
}

func (h *POSHTTPHandler) GetProductByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Product code required"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.GetProductByCode(ctx, &proto.GetProductByCodeRequest{
		ProductCode: code,
	})

	if err != nil || !resp.Success {
		msg := "Product not found"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusNotFound, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Product retrieved successfully", resp.Product))
}

func (h *POSHTTPHandler) ListProducts(c *gin.Context) {
	var query ListProductsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.ListProducts(ctx, &proto.ListProductsRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
		IsActive:       query.IsActive,
		ProductGroupId: query.ProductGroupID,
		SearchTerm:     query.SearchTerm,
	})

	if err != nil || !resp.Success {
		msg := "Failed to list products"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse("Products retrieved successfully", resp.Products, resp.Pagination))
}

// --- Product Group Handlers ---

func (h *POSHTTPHandler) ListProductGroups(c *gin.Context) {
	var query ListProductGroupsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.ListProductGroups(ctx, &proto.ListProductGroupsRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
		IsActive:      query.IsActive,
		ParentGroupId: query.ParentGroupID,
	})

	if err != nil || !resp.Success {
		msg := "Failed to list product groups"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse("Product groups retrieved successfully", resp.ProductGroups, resp.Pagination))
}

// --- Payment Handlers ---

func (h *POSHTTPHandler) ListPaymentTypes(c *gin.Context) {
	isActiveStr := c.Query("is_active")
	var isActive *bool
	if isActiveStr != "" {
		active, err := strconv.ParseBool(isActiveStr)
		if err == nil {
			isActive = &active
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.ListPaymentTypes(ctx, &proto.ListPaymentTypesRequest{
		IsActive: isActive,
	})

	if err != nil || !resp.Success {
		msg := "Failed to list payment types"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Payment types retrieved successfully", resp.PaymentTypes))
}

func (h *POSHTTPHandler) ProcessPayment(c *gin.Context) {
	var req ProcessPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.ProcessPayment(ctx, &proto.ProcessPaymentRequest{
		OrderId:       req.OrderID,
		PaymentTypeId: req.PaymentTypeID,
		PaidAmount:    req.PaidAmount,
	})

	if err != nil || !resp.Success {
		msg := "Payment processing failed"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Payment processed successfully", map[string]interface{}{
		"order_document": resp.OrderDocument,
		"change_amount":  resp.ChangeAmount,
	}))
}

// --- Discount Handlers ---

func (h *POSHTTPHandler) ListDiscounts(c *gin.Context) {
	var query ListDiscountsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.ListDiscounts(ctx, &proto.ListDiscountsRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
		IsActive:   query.IsActive,
		ProductId:  query.ProductID,
		SearchTerm: query.SearchTerm,
	})

	if err != nil || !resp.Success {
		msg := "Failed to list discounts"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse("Discounts retrieved successfully", resp.Discounts, resp.Pagination))
}

func (h *POSHTTPHandler) ValidateDiscount(c *gin.Context) {
	var req ValidateDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.ValidateDiscount(ctx, &proto.ValidateDiscountRequest{
		DiscountId: req.DiscountID,
		ProductId:  req.ProductID,
		Quantity:   req.Quantity,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Discount validation service error"))
		return
	}

	if !resp.Success {
		msg := "Discount validation failed"
		if resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Discount validation completed", map[string]interface{}{
		"is_valid":                   resp.IsValid,
		"reason":                     resp.Reason,
		"calculated_discount_amount": resp.CalculatedDiscountAmount,
	}))
}

// --- Cart Handlers ---

func (h *POSHTTPHandler) CreateCart(c *gin.Context) {
	var req CreateCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.CreateCart(ctx, &proto.CreateCartRequest{
		CashierId: req.CashierID,
	})

	if err != nil || !resp.Success {
		msg := "Failed to create cart"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

	c.JSON(http.StatusCreated, successResponse("Cart created successfully", resp.Cart))
}

func (h *POSHTTPHandler) GetCart(c *gin.Context) {
	cartID := c.Param("id")
	if cartID == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Cart ID required"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.GetCart(ctx, &proto.GetCartRequest{
		CartId: cartID,
	})

	if err != nil || !resp.Success {
		msg := "Cart not found"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusNotFound, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Cart retrieved successfully", resp.Cart))
}

func (h *POSHTTPHandler) AddItemToCart(c *gin.Context) {
	var req AddItemToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.AddItemToCart(ctx, &proto.AddItemToCartRequest{
		CartId:            req.CartID,
		ProductId:         req.ProductID,
		Quantity:          req.Quantity,
		ServingEmployeeId: req.ServingEmployeeID,
	})

	if err != nil || !resp.Success {
		msg := "Failed to add item to cart"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Item added to cart successfully", resp.Cart))
}

func (h *POSHTTPHandler) RemoveItemFromCart(c *gin.Context) {
	cartID := c.Param("cart_id")
	itemID := c.Param("item_id")

	if cartID == "" || itemID == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Cart ID and Item ID required"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.RemoveItemFromCart(ctx, &proto.RemoveItemFromCartRequest{
		CartId: cartID,
		ItemId: itemID,
	})

	if err != nil || !resp.Success {
		msg := "Failed to remove item from cart"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Item removed from cart successfully", resp.Cart))
}

func (h *POSHTTPHandler) ApplyDiscount(c *gin.Context) {
	var req ApplyDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.ApplyDiscount(ctx, &proto.ApplyDiscountRequest{
		CartId:     req.CartID,
		DiscountId: req.DiscountID,
		ItemIds:    req.ItemIDs,
	})

	if err != nil || !resp.Success {
		msg := "Failed to apply discount"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Discount applied successfully", resp.Cart))
}

// --- Order Handlers ---

func (h *POSHTTPHandler) CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	orderItems := make([]*proto.CreateOrderItemRequest, len(req.OrderItems))
	for i, item := range req.OrderItems {
		orderItems[i] = &proto.CreateOrderItemRequest{
			ProductId:         item.ProductID,
			Quantity:          item.Quantity,
			ServingEmployeeId: item.ServingEmployeeID,
			DiscountId:        item.DiscountID,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.posClient.CreateOrder(ctx, &proto.CreateOrderRequest{
		DocumentNumber: req.DocumentNumber,
		CashierId:      req.CashierID,
		DocumentType:   proto.DocumentType(req.DocumentType),
		OrderItems:     orderItems,
		AdditionalInfo: req.AdditionalInfo,
		Notes:          req.Notes,
	})

	if err != nil || !resp.Success {
		msg := "Failed to create order"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusCreated, successResponse("Order created successfully", resp.OrderDocument))
}

func (h *POSHTTPHandler) CreateOrderFromCart(c *gin.Context) {
	var req CreateOrderFromCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.posClient.CreateOrderFromCart(ctx, &proto.CreateOrderFromCartRequest{
		CartId:         req.CartID,
		DocumentNumber: req.DocumentNumber,
		AdditionalInfo: req.AdditionalInfo,
		Notes:          req.Notes,
	})

	if err != nil || !resp.Success {
		msg := "Failed to create order from cart"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusCreated, successResponse("Order created from cart successfully", resp.OrderDocument))
}

func (h *POSHTTPHandler) GetOrder(c *gin.Context) {
	idParam := c.Param("id")
	orderID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid order ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.GetOrder(ctx, &proto.GetOrderRequest{
		Id: orderID,
	})

	if err != nil || !resp.Success {
		msg := "Order not found"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusNotFound, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Order retrieved successfully", resp.OrderDocument))
}

func (h *POSHTTPHandler) ListOrders(c *gin.Context) {
	var query ListOrdersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &proto.ListOrdersRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
		CashierId:    query.CashierID,
		DocumentType: query.DocumentType,
		PaidStatus:   query.PaidStatus,
	}

	if query.StartDate != "" || query.EndDate != "" {
		req.DateRange = &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		}
	}

	resp, err := h.posClient.ListOrders(ctx, req)

	if err != nil || !resp.Success {
		msg := "Failed to list orders"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse("Orders retrieved successfully", resp.OrderDocuments, resp.Pagination))
}

func (h *POSHTTPHandler) VoidOrder(c *gin.Context) {
	var req VoidOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.VoidOrder(ctx, &proto.VoidOrderRequest{
		Id:       req.ID,
		VoidedBy: req.VoidedBy,
		Reason:   req.Reason,
	})

	if err != nil || !resp.Success {
		msg := "Failed to void order"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Order voided successfully", resp.OrderDocument))
}

func (h *POSHTTPHandler) ReturnOrder(c *gin.Context) {
	var req ReturnOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.posClient.ReturnOrder(ctx, &proto.ReturnOrderRequest{
		OriginalOrderId: req.OriginalOrderID,
		ProcessedBy:     req.ProcessedBy,
		ItemIds:         req.ItemIDs,
		Reason:          req.Reason,
	})

	if err != nil || !resp.Success {
		msg := "Failed to process return"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusOK, successResponse("Return processed successfully", resp.ReturnDocument))
}
