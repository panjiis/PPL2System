package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/pos"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var wibLoc, _ = time.LoadLocation("Asia/Jakarta")

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
	ProductCode       string `json:"product_code" binding:"required"`
	Quantity          int32  `json:"quantity" binding:"required,min=1"`
	ServingEmployeeID *int64 `json:"serving_employee_id,omitempty"`
}

type ApplyDiscountRequest struct {
	CartID     string   `json:"cart_id" binding:"required"`
	DiscountID int32    `json:"discount_id" binding:"required"`
	ItemIDs    []string `json:"item_ids,omitempty"`
}

type ValidateDiscountRequest struct {
	DiscountID  int32   `json:"discount_id" binding:"required"`
	ProductCode *string `json:"product_code,omitempty"`
	Quantity    *int32  `json:"quantity,omitempty"`
}

type CreateOrderItemRequest struct {
	ProductCode       string `json:"product_code" binding:"required"`
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
	Page           int     `json:"page,default=1"`
	PageSize       int     `json:"page_size,default=0"`
	IsActive       *bool   `json:"is_active,omitempty"`
	ProductGroupID *int32  `json:"product_group_id,omitempty"`
	SearchTerm     *string `json:"search,omitempty"`
}

type ListProductGroupsQuery struct {
	Page          int    `json:"page,default=1"`
	PageSize      int    `json:"page_size,default=0"`
	IsActive      *bool  `json:"is_active,omitempty"`
	ParentGroupID *int32 `json:"parent_group_id,omitempty"`
}

type ListDiscountsQuery struct {
	Page        int     `json:"page,default=1"`
	PageSize    int     `json:"page_size,default=0"`
	IsActive    *bool   `json:"is_active,omitempty"`
	ProductCode *string `json:"product_code,omitempty"`
	SearchTerm  *int    `json:"search,omitempty"`
}

type ListOrdersQuery struct {
	Page         int                 `json:"page,default=1"`
	PageSize     int                 `json:"page_size,default=20"`
	CashierID    *int64              `json:"cashier_id,omitempty"`
	DocumentType *proto.DocumentType `json:"document_type,omitempty"`
	PaidStatus   *proto.PaidStatus   `json:"paid_status,omitempty"`
	StartDate    string              `json:"start_date,omitempty"`
	EndDate      string              `json:"end_date,omitempty"`
}

type PaymentTypes struct {
	ID                int32  `json:"id"`
	PaymentName       string `json:"payment_name"`
	ProcessingFeeRate string `json:"processing_fee_rate"`
	IsActive          bool   `json:"is_active"`
}

type Product struct {
	ProductCode             string  `json:"product_code"`
	ProductName             string  `json:"product_name"`
	ProductPrice            string  `json:"product_price"`
	CostPrice               string  `json:"cost_price"`
	ImageUrl                *string `json:"image_url,omitempty"`
	Color                   *string `json:"color,omitempty"`
	ProductGroupID          *int32  `json:"product_group_id,omitempty"`
	CommissionEligible      bool    `json:"commission_eligible"`
	RequiresServiceEmployee bool    `json:"requires_service_employee"`
	IsActive                bool    `json:"is_active"`
}

// --- Product Handlers ---

func (h *POSHTTPHandler) CreateProduct(c *gin.Context) {
	var req Product
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.posClient.CreateProduct(ctx, &proto.Product{
		ProductCode:             req.ProductCode,
		ProductName:             req.ProductName,
		ProductPrice:            req.ProductPrice,
		CostPrice:               req.CostPrice,
		ImageUrl:                req.ImageUrl,
		ProductGroupId:          req.ProductGroupID,
		CommissionEligible:      req.CommissionEligible,
		RequiresServiceEmployee: req.RequiresServiceEmployee,
		IsActive:                req.IsActive,
	})

	if err != nil || !resp.Success {
		msg := "Failed to create payment type"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusCreated, successResponse("Product created successfully", resp.Product))
}

func (h *POSHTTPHandler) UpdateProduct(c *gin.Context) {
	productCode := c.Param("code")
	log.Printf("Received code param: '%s'", productCode)
	if productCode == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Product Code required"))
		return
	}

	var req struct {
		ProductCode             *string `json:"product_code"`
		ProductName             *string `json:"product_name"`
		ProductPrice            *string `json:"product_price"`
		CostPrice               *string `json:"cost_price"`
		ImageUrl                *string `json:"image_url"`
		Color                   *string `json:"color"`
		ProductGroupId          *int32  `json:"product_group_id"`
		CommissionEligible      *bool   `json:"commission_eligible"`
		RequiresServiceEmployee *bool   `json:"requires_service_employee"`
		IsActive                *bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid request body"))
		return
	}

	resp, err := h.posClient.UpdateProduct(c.Request.Context(), &proto.UpdateProductRequest{
		ProductCode:             productCode,
		ProductName:             req.ProductName,
		ProductPrice:            req.ProductPrice,
		CostPrice:               req.CostPrice,
		Color:                   req.Color,
		ImageUrl:                req.ImageUrl,
		ProductGroupId:          req.ProductGroupId,
		CommissionEligible:      req.CommissionEligible,
		RequiresServiceEmployee: req.RequiresServiceEmployee,
		IsActive:                req.IsActive,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("failed to update product: "+err.Error()))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(*resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse("Product updated successfully", resp.Product))
}

func (h *POSHTTPHandler) GetProduct(c *gin.Context) {
	code := c.Param("code")
	log.Printf("Received code param: '%s'", code)
	if code == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Product code required"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.posClient.GetProduct(ctx, &proto.GetProductRequest{
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

func (s *POSHTTPHandler) CreateProductGroupHandler(c *gin.Context) {
	var req proto.CreateProductGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.posClient.CreateProductGroup(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, successResponse("Product groups created successfully", resp.ProductGroup))
}

func (s *POSHTTPHandler) UpdateProductGroupHandler(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req proto.UpdateProductGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Id = int32(id)

	resp, err := s.posClient.UpdateProductGroup(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, successResponse("Product groups updated successfully", resp.ProductGroup))
}

func (s *POSHTTPHandler) GetProductGroupHandler(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req proto.GetProductGroupRequest
	req.Id = int32(id)

	resp, err := s.posClient.GetProductGroup(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp.Success {
		if resp.Message != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": *resp.Message})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Product Group not found"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

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

func (h *POSHTTPHandler) CreatePaymentTypes(c *gin.Context) {
	var req PaymentTypes
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format"))
		return
	}

	if rate, err := strconv.ParseFloat(req.ProcessingFeeRate, 64); err != nil || rate < 0 || rate > 100 {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid processing fee rate"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.posClient.CreatePaymentTypes(ctx, &proto.PaymentType{
		Id:                req.ID,
		PaymentName:       req.PaymentName,
		ProcessingFeeRate: req.ProcessingFeeRate,
		IsActive:          req.IsActive,
	})

	if err != nil || !resp.Success {
		msg := "Failed to create payment type"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusBadRequest, errorResponse(msg))
		return
	}

	c.JSON(http.StatusCreated, successResponse("Payment Types created successfully", resp.PaymentTypes))
}

func (h *POSHTTPHandler) UpdatePaymentType(c *gin.Context) {
	id := c.Param("id")
	paymentTypeID, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid payment type id"))
		return
	}

	var req struct {
		PaymentName       *string `json:"payment_name"`
		ProcessingFeeRate *string `json:"processing_fee_rate"`
		IsActive          *bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid request body"))
		return
	}

	if rate, err := strconv.ParseFloat(*req.ProcessingFeeRate, 64); err != nil || rate < 0 || rate > 100 {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid processing fee rate"))
		return
	}

	resp, err := h.posClient.UpdatePaymentTypes(c.Request.Context(), &proto.UpdatePaymentTypeRequest{
		Id:                int32(paymentTypeID),
		PaymentName:       req.PaymentName,
		ProcessingFeeRate: req.ProcessingFeeRate,
		IsActive:          req.IsActive,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("failed to update payment type: "+err.Error()))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(*resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse("Payment Types updated successfully", resp.PaymentTypes))
}

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

	var discountType *proto.DiscountType
	if query.SearchTerm != nil {
		switch *query.SearchTerm {
		case 1:
			dt := proto.DiscountType_DISCOUNT_TYPE_PERCENTAGE
			discountType = &dt
		case 2:
			dt := proto.DiscountType_DISCOUNT_TYPE_FIXED_AMOUNT
			discountType = &dt
		case 3:
			dt := proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y
			discountType = &dt
		}
	}

	resp, err := h.posClient.ListDiscounts(ctx, &proto.ListDiscountsRequest{
		Page:         int32(query.Page),
		PageSize:     int32(query.PageSize),
		IsActive:     query.IsActive,
		ProductCode:  query.ProductCode,
		Search:       lib.IntToInt32Ptr(query.SearchTerm),
		DiscountType: discountType,
	})

	if err != nil || !resp.Success {
		msg := "Failed to list discounts"
		if resp != nil && resp.Message != nil {
			msg = *resp.Message
		}
		c.JSON(http.StatusInternalServerError, errorResponse(msg))
		return
	}

    for _, discount := range resp.Discounts {
		discount.ValidFrom = lib.LocalToWIBTimestamp(discount.GetValidFrom())
        discount.ValidUntil = lib.LocalToWIBTimestamp(discount.GetValidUntil())
    }

	c.JSON(http.StatusOK, successWithMetaResponse("Discounts retrieved successfully", resp.Discounts, gin.H{
		"total":     resp.Total,
		"page":      resp.Page,
		"page_size": resp.PageSize,
	}))
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
		DiscountId:  req.DiscountID,
		ProductCode: req.ProductCode,
		Quantity:    req.Quantity,
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

	c.JSON(http.StatusOK, successResponse("Discount validation completed", gin.H{
		"is_valid":                   resp.IsValid,
		"reason":                     resp.Reason,
		"calculated_discount_amount": resp.CalculatedDiscountAmount,
	}))
}

func (h *POSHTTPHandler) CreateDiscount(c *gin.Context) {
	var req lib.CreateDiscountRequest
	var maxUsage *int64
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	var discountType proto.DiscountType
	switch req.DiscountType {
	case 1:
		discountType = proto.DiscountType_DISCOUNT_TYPE_PERCENTAGE
	case 2:
		discountType = proto.DiscountType_DISCOUNT_TYPE_FIXED_AMOUNT
	case 3:
		discountType = proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y
	default:
		c.JSON(http.StatusBadRequest, errorResponse("Invalid discount type"))
		return
	}

	if discountType == proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y {
		if req.BuyQuantity == nil || req.GetQuantity == nil {
			c.JSON(http.StatusBadRequest, errorResponse("buy_quantity and get_quantity are required for BUY_X_GET_Y discount"))
			return
		}
		if *req.BuyQuantity <= 0 || *req.GetQuantity <= 0 {
			c.JSON(http.StatusBadRequest, errorResponse("buy_quantity and get_quantity must be positive integers"))
			return
		}
		if req.DiscountValue != nil {
			c.JSON(http.StatusBadRequest, errorResponse("discount_value must not be set for BUY_X_GET_Y discount"))
			return
		}
	}

	var validFrom, validUntil *timestamppb.Timestamp
	if req.ValidFrom != nil {
		validFrom = timestamppb.New(*req.ValidFrom)
	}
	if req.ValidUntil != nil {
		validUntil = timestamppb.New(*req.ValidUntil)
	}
	if req.MaxUsagePerTransaction != nil {
		val, err := strconv.ParseInt(*req.MaxUsagePerTransaction, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("Invalid max_usage_per_transaction: must be an integer"))
			return
		}
		maxUsage = &val
	}

	if req.ProductCode == nil {
		req.ProductCode = nil
	}
	if req.ProductGroupId == nil {
		req.ProductGroupId = nil
	}

	grpcReq := &proto.CreateDiscountRequest{
		DiscountName:           req.DiscountName,
		DiscountType:           discountType,
		ProductCode:            req.ProductCode,
		ProductGroupId:         req.ProductGroupId,
		MinQuantity:            req.MinQuantity,
		MaxUsagePerTransaction: maxUsage,
		ValidFrom:              validFrom,
		ValidUntil:             validUntil,
		IsActive:               req.IsActive,
	}

	if discountType == proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y {
		req.DiscountValue = lib.StrPtr(fmt.Sprintf("%d:%d", *req.BuyQuantity, *req.GetQuantity))
	}

	log.Print(*req.DiscountValue)

	if req.DiscountValue != nil {
		grpcReq.DiscountValue = *req.DiscountValue
	}
	grpcReq.BuyQuantity = req.BuyQuantity
	grpcReq.GetQuantity = req.GetQuantity

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.CreateDiscount(ctx, grpcReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Discount service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(*resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse("Discount created successfully", resp.Discount))
}

func (h *POSHTTPHandler) GetDiscount(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid discount ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.GetDiscount(ctx, &proto.GetDiscountRequest{
		Id: id,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Discount service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusNotFound, errorResponse(*resp.Message))
		return
	}

	if resp.Discount.GetValidFrom() != nil {
        resp.Discount.ValidFrom = lib.LocalToWIBTimestamp(resp.Discount.GetValidFrom())
    }
    if resp.Discount.GetValidUntil() != nil {
        resp.Discount.ValidUntil = lib.LocalToWIBTimestamp(resp.Discount.GetValidUntil())
    }

	c.JSON(http.StatusOK, successResponse("Discount retrieved successfully", resp.Discount))
}

func (h *POSHTTPHandler) UpdateDiscount(c *gin.Context) {
	var maxUsage *int64
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid discount ID"))
		return
	}

	var req lib.UpdateDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	var discountType *proto.DiscountType

	if req.DiscountType != nil {
		var dt proto.DiscountType
		switch *req.DiscountType {
		case 1:
			dt = proto.DiscountType_DISCOUNT_TYPE_PERCENTAGE
		case 2:
			dt = proto.DiscountType_DISCOUNT_TYPE_FIXED_AMOUNT
		case 3:
			dt = proto.DiscountType_DISCOUNT_TYPE_BUY_X_GET_Y
		default:
			c.JSON(http.StatusBadRequest, errorResponse("Invalid discount type"))
			return
		}
		discountType = &dt
	}

	if req.DiscountType != nil && *req.DiscountType == 3 {
		if req.DiscountValue != nil {
			c.JSON(http.StatusBadRequest, errorResponse("discount_value must not be set for BUY_X_GET_Y discount"))
			return
		}

		if req.BuyQuantity != nil && *req.BuyQuantity <= 0 {
			c.JSON(http.StatusBadRequest, errorResponse("buy_quantity must be positive"))
			return
		}

		if req.GetQuantity != nil && *req.GetQuantity <= 0 {
			c.JSON(http.StatusBadRequest, errorResponse("get_quantity must be positive"))
			return
		}

	} else if req.DiscountType != nil {
		if req.BuyQuantity != nil || req.GetQuantity != nil {
			c.JSON(http.StatusBadRequest, errorResponse("buy_quantity and get_quantity must not be set for non BUY_X_GET_Y discounts"))
			return
		}
	}

	var validFrom, validUntil *timestamppb.Timestamp
	if req.ValidFrom != nil {
		validFrom = timestamppb.New(*req.ValidFrom)
	}
	if req.ValidUntil != nil {
		validUntil = timestamppb.New(*req.ValidUntil)
	}
	if req.MaxUsagePerTransaction != nil {
		val, err := strconv.ParseInt(*req.MaxUsagePerTransaction, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("Invalid max_usage_per_transaction: must be an integer"))
			return
		}
		maxUsage = &val
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grpcReq := &proto.UpdateDiscountRequest{
		Id:                     id,
		DiscountName:           req.DiscountName,
		DiscountType:           discountType,
		DiscountValue:          req.DiscountValue,
		BuyQuantity:            req.BuyQuantity,
		GetQuantity:            req.GetQuantity,
		ProductCode:            req.ProductCode,
		ProductGroupId:         req.ProductGroupId,
		MinQuantity:            req.MinQuantity,
		MaxUsagePerTransaction: maxUsage,
		ValidFrom:              validFrom,
		ValidUntil:             validUntil,
		IsActive:               req.IsActive,
	}

	resp, err := h.posClient.UpdateDiscount(ctx, grpcReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Discount service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(*resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse("Discount updated successfully", resp.Discount))
}

func (h *POSHTTPHandler) DeleteDiscount(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid discount ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.posClient.DeleteDiscount(ctx, &proto.DeleteDiscountRequest{
		Id: id,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Discount service error"))
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, errorResponse(*resp.Message))
		return
	}

	c.JSON(http.StatusOK, successResponse("Discount deleted successfully", nil))
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
		ProductCode:       req.ProductCode,
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
			ProductCode:       item.ProductCode,
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
