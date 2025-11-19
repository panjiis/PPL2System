package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	// Pastikan path import ini benar
	proto "syntra-system/proto/protogen/analytics"
)

// AnalyticsHTTPHandler menampung klien gRPC
type AnalyticsHTTPHandler struct {
	analyticsClient proto.AnalyticsServiceClient
}

// NewAnalyticsHTTPHandler adalah constructor
func NewAnalyticsHTTPHandler(analyticsClient proto.AnalyticsServiceClient) *AnalyticsHTTPHandler {
	return &AnalyticsHTTPHandler{
		analyticsClient: analyticsClient,
	}
}

// Untuk: GET /api/v1/dashboard
type GetDashboardQuery struct {
	Date string `form:"date" binding:"required"`
}

// Untuk: GET /api/v1/reports/sales
type GetSalesReportQuery struct {
	StartDate               string `form:"start_date" binding:"required"`
	EndDate                 string `form:"end_date" binding:"required"`
	CashierID               *int64 `form:"cashier_id"` // Pointer untuk opsional
	ProductGroupID          *int32 `form:"product_group_id"`
	IncludeDailyBreakdown   bool   `form:"include_daily_breakdown,default=false"` // 'bool' akan otomatis false jika tidak ada
	IncludeProductBreakdown bool   `form:"include_product_breakdown,default=false"`
}

// Untuk: POST /api/v1/reports/daily-summary/generate
type GenerateSummaryBody struct {
	Date      string `json:"date" binding:"required"`
	CashierID *int64 `json:"cashier_id"`
}

// Untuk: GET /api/v1/products/sales
type GetProductSalesQuery struct {
	StartDate      string  `form:"start_date" binding:"required"`
	EndDate        string  `form:"end_date" binding:"required"`
	ProductCode    *string `form:"product_code"`
	ProductGroupID *int32  `form:"product_group_id"`
	PageSize       int     `form:"page_size,default=20"`
	PageToken      string  `form:"page_token"` // Page token biasanya string
}

// Untuk: GET /api/v1/reports/daily-summary
type GetDailySummaryQuery struct {
	Date      string `form:"date" binding:"required"`
	CashierID *int64 `form:"cashier_id"`
}

// Untuk: GET /api/v1/products/top-selling
type GetTopSellingProductsQuery struct {
	StartDate      string `form:"start_date" binding:"required"`
	EndDate        string `form:"end_date" binding:"required"`
	Limit          int32  `form:"limit,default=5"` // Tipe data proto adalah int32
	ProductGroupID *int32 `form:"product_group_id"`
}

// Untuk: GET /api/v1/employees/performance
type GetEmployeePerformanceQuery struct {
	StartDate  string `form:"start_date" binding:"required"`
	EndDate    string `form:"end_date" binding:"required"`
	EmployeeID *int64 `form:"employee_id"`
	PageSize   int    `form:"page_size,default=20"`
	PageToken  string `form:"page_token"`
}

// Untuk: GET /api/v1/performance/report
type GetPerformanceReportQuery struct {
	StartDate  string `form:"start_date" binding:"required"`
	EndDate    string `form:"end_date" binding:"required"`
	EmployeeID *int64 `form:"employee_id"`
}

// Untuk: GET /api/v1/customers/analytics
type GetCustomerAnalyticsQuery struct {
	StartDate      string `form:"start_date" binding:"required"`
	EndDate        string `form:"end_date" binding:"required"`
	ProductGroupID *int32 `form:"product_group_id"`
	PageSize       int    `form:"page_size,default=20"`
	PageToken      string `form:"page_token"`
}

// Untuk: GET /api/v1/customers/peak-hours
type GetPeakHoursQuery struct {
	StartDate string `form:"start_date" binding:"required"`
	EndDate   string `form:"end_date" binding:"required"`
}

// Untuk: POST /api/v1/products/sales/generate
type GenerateProductSalesSummaryBody struct {
	Date           string  `json:"date" binding:"required"`
	ProductCode    *string `json:"product_code"`     // Pointer untuk opsional
	ProductGroupID *int32  `json:"product_group_id"` // Pointer untuk opsional
}

type GenerateCustomerAnalyticsBody struct {
	Date           string `json:"date" binding:"required"`
	ProductGroupID *int32 `json:"product_group_id"` // Pointer untuk opsional
}

type GenerateEmployeePerformanceBody struct {
	CalculationID int64 `json:"calculation_id" binding:"required"`
}

// Menggantikan: http_get_dashboard_data
// Endpoint: GET /api/v1/dashboard
func (h *AnalyticsHTTPHandler) GetDashboardData(c *gin.Context) {
	var query GetDashboardQuery
	// Bind query param
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	// Siapkan request gRPC
	req := &proto.GetDashboardDataRequest{
		Date: query.Date,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Set timeout
	defer cancel()

	// Panggil gRPC ke server Python
	resp, err := h.analyticsClient.GetDashboardData(ctx, req)

	// Handle error gRPC (fungsi ini sudah termasuk c.Abort())
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// Kembalikan respons sukses
	// di api_server.py: return {"dashboard_data": result_dict}
	c.JSON(http.StatusOK, successResponse("Dashboard data retrieved successfully", resp.Dashboard))
}

// Menggantikan: http_get_sales_report
// Endpoint: GET /api/v1/reports/sales
func (h *AnalyticsHTTPHandler) GetSalesReport(c *gin.Context) {
	var query GetSalesReportQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	// Siapkan request gRPC
	req := &proto.GetSalesReportRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		CashierId:               query.CashierID, // Otomatis nil jika tidak ada
		ProductGroupId:          query.ProductGroupID,
		IncludeDailyBreakdown:   &query.IncludeDailyBreakdown, // Gunakan pointer jika field proto 'opsional'
		IncludeProductBreakdown: &query.IncludeProductBreakdown,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Timeout lebih lama untuk laporan
	defer cancel()

	// Panggil gRPC
	resp, err := h.analyticsClient.GetSalesReport(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// Kembalikan respons sukses
	// di api_server.py: return report_data
	c.JSON(http.StatusOK, successResponse("Sales report retrieved successfully", resp.SalesReport))
}

// Menggantikan: http_generate_daily_summary
// Endpoint: POST /api/v1/reports/daily-summary/generate
func (h *AnalyticsHTTPHandler) GenerateDailySummary(c *gin.Context) {
	var body GenerateSummaryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Body request tidak valid: "+err.Error()))
		return
	}

	req := &proto.GenerateDailySummaryRequest{
		Date:      body.Date,
		CashierId: body.CashierID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Timeout lebih lama untuk generate
	defer cancel()

	resp, err := h.analyticsClient.GenerateDailySummary(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// Kembalikan respons sukses
	// di api_server.py: return { "success": True, "message": ..., "generated_summaries": ... }
	c.JSON(http.StatusOK, successResponse(*resp.Message, resp.GeneratedSummaries))
}

// Menggantikan: http_get_daily_summary
// Endpoint: GET /api/v1/reports/daily-summary
func (h *AnalyticsHTTPHandler) GetDailySummary(c *gin.Context) {
	var query GetDailySummaryQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	req := &proto.GetDailySummaryRequest{
		Date:      query.Date,
		CashierId: query.CashierID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetDailySummary(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"daily_summaries": summaries}
	c.JSON(http.StatusOK, successResponse("Daily summaries retrieved", resp.DailySummaries))
}

// Menggantikan: http_get_product_sales
// Endpoint: GET /api/v1/products/sales
func (h *AnalyticsHTTPHandler) GetProductSales(c *gin.Context) {
	var query GetProductSalesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	req := &proto.GetProductSalesRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		ProductCode:    query.ProductCode,
		ProductGroupId: query.ProductGroupID,
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: query.PageToken,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Printf("before")
	resp, err := h.analyticsClient.GetProductSales(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}
	log.Printf("after")

	// di api_server.py: return {"product_sales": ..., "pagination": ...}
	c.JSON(http.StatusOK, successWithMetaResponse("Product sales retrieved", resp.ProductSales, resp.Pagination))
}

// Menggantikan: http_get_top_selling_products
// Endpoint: GET /api/v1/products/top-selling
func (h *AnalyticsHTTPHandler) GetTopSellingProducts(c *gin.Context) {
	var query GetTopSellingProductsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	req := &proto.GetTopSellingProductsRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		Limit:          query.Limit,
		ProductGroupId: query.ProductGroupID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetTopSellingProducts(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"top_selling_products": results}
	c.JSON(http.StatusOK, successResponse("Top selling products retrieved", resp.TopProducts))
}

// Menggantikan: http_get_employee_performance
// Endpoint: GET /api/v1/employees/performance
func (h *AnalyticsHTTPHandler) GetEmployeePerformance(c *gin.Context) {
	var query GetEmployeePerformanceQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	req := &proto.GetEmployeePerformanceRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		EmployeeId: query.EmployeeID,
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: query.PageToken,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetEmployeePerformance(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"performances": ..., "pagination": ...}
	c.JSON(http.StatusOK, successWithMetaResponse("Employee performances retrieved", resp.Performances, resp.Pagination))
}

// Menggantikan: http_get_performance_report
// Endpoint: GET /api/v1/performance/report
func (h *AnalyticsHTTPHandler) GetPerformanceReport(c *gin.Context) {
	var query GetPerformanceReportQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	req := &proto.GetPerformanceReportRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		EmployeeId: query.EmployeeID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetPerformanceReport(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"performance_report": result_dict}
	c.JSON(http.StatusOK, successResponse("Performance report retrieved", resp.PerformanceReport))
}

// Menggantikan: http_get_customer_analytics
// Endpoint: GET /api/v1/customers/analytics
func (h *AnalyticsHTTPHandler) GetCustomerAnalytics(c *gin.Context) {
	var query GetCustomerAnalyticsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
		return
	}

	req := &proto.GetCustomerAnalyticsRequest{
		DateRange: &proto.DateRange{
			StartDate: query.StartDate,
			EndDate:   query.EndDate,
		},
		ProductGroupId: query.ProductGroupID,
		Pagination: &proto.PaginationRequest{
			PageSize:  int32(query.PageSize),
			PageToken: query.PageToken,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetCustomerAnalytics(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"analytics": ..., "pagination": ...}
	c.JSON(http.StatusOK, successWithMetaResponse("Customer analytics retrieved", resp.Analytics, resp.Pagination))
}

// Menggantikan: http_get_peak_hours
// Endpoint: GET /api/v1/customers/peak-hours
func (h *AnalyticsHTTPHandler) GetPeakHours(c *gin.Context) {
	// var query GetPeakHoursQuery
	// if err := c.ShouldBindQuery(&query); err != nil {
	// 	c.JSON(http.StatusBadRequest, errorResponse("Parameter query tidak valid: "+err.Error()))
	// 	return
	// }

	req := &proto.GetPeakHoursRequest{}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetPeakHours(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"peak_hours": results}
	c.JSON(http.StatusOK, successResponse("Peak hours retrieved", resp.PeakDataByWeek))
}

// Menggantikan: http_get_real_time_metrics
// Endpoint: GET /api/v1/real-time-metrics
func (h *AnalyticsHTTPHandler) GetRealTimeMetrics(c *gin.Context) {
	// Tidak ada query param, langsung buat request
	req := &proto.GetRealTimeMetricsRequest{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.analyticsClient.GetRealTimeMetrics(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// di api_server.py: return {"metrics": ...}
	c.JSON(http.StatusOK, successResponse("Real-time metrics retrieved", resp.Metrics))
}

// Endpoint: POST /api/v1/products/sales/generate
func (h *AnalyticsHTTPHandler) GenerateProductSalesSummary(c *gin.Context) {
	var body GenerateProductSalesSummaryBody
	// Validasi JSON body
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Body request tidak valid: "+err.Error()))
		return
	}

	// Siapkan request gRPC sesuai proto
	req := &proto.GenerateProductSalesSummaryRequest{
		Date:           body.Date,
		ProductCode:    body.ProductCode,
		ProductGroupId: body.ProductGroupID,
	}

	log.Printf("")

	// Set timeout yang lebih lama untuk proses ETL/Generate
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Panggil gRPC ke server Python
	resp, err := h.analyticsClient.GenerateProductSalesSummary(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// Kembalikan respons sukses
	c.JSON(http.StatusOK, successResponse(*resp.Message, resp.GeneratedSummaries))
}

func (h *AnalyticsHTTPHandler) GenerateCustomerAnalytics(c *gin.Context) {
	var body GenerateCustomerAnalyticsBody
	// Validasi JSON body
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Body request tidak valid: "+err.Error()))
		return
	}

	// Siapkan request gRPC sesuai proto
	req := &proto.GenerateCustomerAnalyticsRequest{
		Date:           body.Date,
		ProductGroupId: body.ProductGroupID,
	}

	// Set timeout yang lebih lama untuk proses ETL/Generate
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Panggil gRPC ke server Python
	resp, err := h.analyticsClient.GenerateCustomerAnalytics(ctx, req)
	if err != nil {
		handleGRPCError(c, err) // Fungsi error handling Anda yang sudah ada
		return
	}

	// Kembalikan respons sukses
	// (Menggunakan GeneratedAnalytics sesuai definisi proto kita)
	c.JSON(http.StatusOK, successResponse(*resp.Message, resp.GeneratedAnalytics))
}

func (h *AnalyticsHTTPHandler) GenerateEmployeePerformance(c *gin.Context) {
	var body GenerateEmployeePerformanceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Body request tidak valid: "+err.Error()))
		return
	}

	req := &proto.GenerateEmployeePerformanceRequest{
		CalculationId: body.CalculationID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Timeout ETL
	defer cancel()

	resp, err := h.analyticsClient.GenerateEmployeePerformance(ctx, req)
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(*resp.Message, resp.GeneratedPerformance))
}
