package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proto "syntra-system/proto/protogen/commissions"
)

type CommissionsHTTPHandler struct {
	commissionClient proto.CommissionServiceClient
}

func NewCommissionsHTTPHandler(commissionClient proto.CommissionServiceClient) *CommissionsHTTPHandler {
	return &CommissionsHTTPHandler{
		commissionClient: commissionClient,
	}
}

func handleGRPCError(c *gin.Context, err error) {
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.InvalidArgument:
				c.JSON(http.StatusBadRequest, errorResponse(s.Message()))
			case codes.NotFound:
				c.JSON(http.StatusNotFound, errorResponse(s.Message()))
			case codes.FailedPrecondition:
				c.JSON(http.StatusBadRequest, errorResponse(s.Message()))
			case codes.AlreadyExists:
				c.JSON(http.StatusConflict, errorResponse(s.Message()))
			case codes.DeadlineExceeded:
				c.JSON(http.StatusGatewayTimeout, errorResponse("Request Timeout: "+s.Message()))
			default:
				c.JSON(http.StatusInternalServerError, errorResponse("Internal Service Error: "+s.Message()))
			}
		} else {
			c.JSON(http.StatusInternalServerError, errorResponse("Unknown Service Error"))
		}
		c.Abort() 
	}
}

// --- Request & Query Structs for Binding ---

type CalculateCommissionRequest struct {
	EmployeeID      int64  `json:"employee_id" binding:"required"`
	PeriodStart     string `json:"period_start" binding:"required"`
	PeriodEnd       string `json:"period_end" binding:"required"`
	CalculatedBy    int64  `json:"calculated_by" binding:"required"`
	SaveCalculation *bool  `json:"save_calculation"`
}

type RecalculateCommissionRequest struct {
	RecalculatedBy int64   `json:"recalculated_by" binding:"required"`
	Notes          *string `json:"notes"`
}

type BulkCalculateRequest struct {
	EmployeeIDs  []int64 `json:"employee_ids" binding:"required"`
	PeriodStart  string  `json:"period_start" binding:"required"`
	PeriodEnd    string  `json:"period_end" binding:"required"`
	CalculatedBy int64   `json:"calculated_by" binding:"required"`
}

type ListCalculationsQuery struct {
	Page       int    `form:"page,default=1"`
	PageSize   int    `form:"page_size,default=0"`
	EmployeeID *int64 `form:"employee_id"`
	Status     *int32 `form:"status"`
	StartDate  string `form:"start_date"`
	EndDate    string `form:"end_date"`
}

type ApproveRequest struct {
	ApprovedBy    int64   `json:"approved_by" binding:"required"`
	ApprovalNotes *string `json:"approval_notes"`
}

type RejectRequest struct {
	RejectedBy      int64  `json:"rejected_by" binding:"required"`
	RejectionReason string `json:"rejection_reason" binding:"required"`
}

type BulkApproveRequest struct {
	CommissionCalculationIDs []int64 `json:"commission_calculation_ids" binding:"required"`
	ApprovedBy               int64   `json:"approved_by" binding:"required"`
	ApprovalNotes            *string `json:"approval_notes"`
}

type PayCommissionRequest struct {
	PaymentTypeID   int32   `json:"payment_type_id" binding:"required"`
	ReferenceNumber *string `json:"reference_number"`
	PaidBy          int64   `json:"paid_by" binding:"required"`
	Notes           *string `json:"notes"`
	PaymentDate     *string `json:"payment_date"`
}

type ReportQuery struct {
	Page       int    `form:"page,default=1"`
	PageSize   int    `form:"page_size,default=0"`
	EmployeeID *int64 `form:"employee_id"`
	Status     *int32 `form:"status"`
	StartDate  string `form:"start_date" binding:"required"`
	EndDate    string `form:"end_date" binding:"required"`
}

// --- Commission Calculation Handlers ---

func (h *CommissionsHTTPHandler) CalculateCommission(c *gin.Context) {
	var req CalculateCommissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.commissionClient.CalculateCommission(ctx, &proto.CalculateCommissionRequest{
		EmployeeId:      req.EmployeeID,
		PeriodStart:     req.PeriodStart,
		PeriodEnd:       req.PeriodEnd,
		CalculatedBy:    req.CalculatedBy,
		SaveCalculation: req.SaveCalculation,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Commission calculated successfully", resp))
}

func (h *CommissionsHTTPHandler) RecalculateCommission(c *gin.Context) {
	calcID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid calculation ID"))
		return
	}

	var req RecalculateCommissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.commissionClient.RecalculateCommission(ctx, &proto.RecalculateCommissionRequest{
		CommissionCalculationId: calcID,
		RecalculatedBy:          req.RecalculatedBy,
		Notes:                   req.Notes,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Commission recalculated successfully", resp))
}

func (h *CommissionsHTTPHandler) BulkCalculateCommissions(c *gin.Context) {
	var req BulkCalculateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for bulk operations
	defer cancel()

	resp, err := h.commissionClient.BulkCalculateCommissions(ctx, &proto.BulkCalculateCommissionsRequest{
		EmployeeIds:  req.EmployeeIDs,
		PeriodStart:  req.PeriodStart,
		PeriodEnd:    req.PeriodEnd,
		CalculatedBy: req.CalculatedBy,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Bulk calculation processed", resp))
}

// --- Commission Management Handlers ---

func (h *CommissionsHTTPHandler) GetCommissionCalculation(c *gin.Context) {
	calcID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid calculation ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.commissionClient.GetCommissionCalculation(ctx, &proto.GetCommissionCalculationRequest{Id: calcID})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Calculation retrieved successfully", resp.CommissionCalculation))
}

func (h *CommissionsHTTPHandler) ListCommissionCalculations(c *gin.Context) {
	var query ListCalculationsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters: "+err.Error()))
		return
	}

	grpcReq := &proto.ListCommissionCalculationsRequest{
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
	}
	if query.EmployeeID != nil {
		grpcReq.EmployeeId = query.EmployeeID
	}
	if query.Status != nil {
		statusEnum := proto.CommissionStatus(*query.Status)
		grpcReq.Status = &statusEnum
	}
	if query.StartDate != "" && query.EndDate != "" {
		grpcReq.CalculationPeriod = &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.commissionClient.ListCommissionCalculations(ctx, grpcReq)

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successWithMetaResponse("Calculations retrieved successfully", resp.CommissionCalculations, resp.Pagination))
}

func (h *CommissionsHTTPHandler) ApproveCommission(c *gin.Context) {
	calcID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid calculation ID"))
		return
	}

	var req ApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.commissionClient.ApproveCommission(ctx, &proto.ApproveCommissionRequest{
		CommissionCalculationId: calcID,
		ApprovedBy:              req.ApprovedBy,
		ApprovalNotes:           req.ApprovalNotes,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Commission approved successfully", resp.CommissionCalculation))
}

func (h *CommissionsHTTPHandler) RejectCommission(c *gin.Context) {
	calcID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid calculation ID"))
		return
	}

	var req RejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.commissionClient.RejectCommission(ctx, &proto.RejectCommissionRequest{
		CommissionCalculationId: calcID,
		RejectedBy:              req.RejectedBy,
		RejectionReason:         req.RejectionReason,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Commission rejected successfully", resp.CommissionCalculation))
}

func (h *CommissionsHTTPHandler) BulkApproveCommissions(c *gin.Context) {
	var req BulkApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := h.commissionClient.BulkApproveCommissions(ctx, &proto.BulkApproveCommissionsRequest{
		CommissionCalculationIds: req.CommissionCalculationIDs,
		ApprovedBy:               req.ApprovedBy,
		ApprovalNotes:            req.ApprovalNotes,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Bulk approval processed", resp))
}

// --- Commission Payment Handlers ---

func (h *CommissionsHTTPHandler) PayCommission(c *gin.Context) {
	calcID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid calculation ID"))
		return
	}

	var req PayCommissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid request format: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.commissionClient.PayCommission(ctx, &proto.PayCommissionRequest{
		CommissionCalculationId: calcID,
		PaymentTypeId:           req.PaymentTypeID,
		ReferenceNumber:         req.ReferenceNumber,
		PaidBy:                  req.PaidBy,
		Notes:                   req.Notes,
		PaymentDate:             req.PaymentDate,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Commission paid successfully", resp))
}

func (h *CommissionsHTTPHandler) GetCommissionPayment(c *gin.Context) {
	calcID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid calculation ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.commissionClient.GetCommissionPayment(ctx, &proto.GetCommissionPaymentRequest{
		CommissionCalculationId: calcID,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Payment retrieved successfully", resp.CommissionPayment))
}

// --- Commission Reporting Handlers ---

func (h *CommissionsHTTPHandler) GetCommissionSummary(c *gin.Context) {
	empID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid employee ID"))
		return
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Query parameters 'start_date' and 'end_date' are required"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("Calling commission service for employee %d", empID)
	resp, err := h.commissionClient.GetCommissionSummary(ctx, &proto.GetCommissionSummaryRequest{
		EmployeeId: empID,
		DateRange: &proto.DateRange{
			StartDate: startDate,
			EndDate:   endDate,
		},
	})

	if err != nil {
		log.Printf("Commission service error: %v", err)
	}

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Summary retrieved successfully", resp.Summary))
}

func (h *CommissionsHTTPHandler) GetCommissionReport(c *gin.Context) {
	var query ReportQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid query parameters: "+err.Error()))
		return
	}

	grpcReq := &proto.GetCommissionReportRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: strconv.Itoa(query.Page),
		},
	}
	if query.EmployeeID != nil {
		grpcReq.EmployeeId = query.EmployeeID
	}
	if query.Status != nil {
		statusEnum := proto.CommissionStatus(*query.Status)
		grpcReq.Status = &statusEnum
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := h.commissionClient.GetCommissionReport(ctx, grpcReq)

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Report retrieved successfully", resp))
}

// --- Commission Settings Handlers ---

func (h *CommissionsHTTPHandler) GetCommissionSettings(c *gin.Context) {
	empID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Invalid employee ID"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.commissionClient.GetCommissionSettings(ctx, &proto.GetCommissionSettingsRequest{
		EmployeeId: empID,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse("Settings retrieved successfully", resp))
}
