package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proto "syntra-system/proto/protogen/commissions"
)

func (c *CommissionHandler) GetCommissionSummary(ctx context.Context, req *proto.GetCommissionSummaryRequest) (*proto.GetCommissionSummaryResponse, error) {
	if req.GetEmployeeId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Employee ID is required")
	}
	if req.GetDateRange().GetStartDate() == "" || req.GetDateRange().GetEndDate() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Date range with start and end date is required")
	}

	employeeID := req.GetEmployeeId()
	startDate := req.GetDateRange().GetStartDate()
	endDate := req.GetDateRange().GetEndDate()

	cacheKey := fmt.Sprintf("commission_summary:%d:%s:%s", employeeID, startDate, endDate)
	val, err := c.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var summary proto.CommissionSummary
		if err := json.Unmarshal([]byte(val), &summary); err == nil {
			return &proto.GetCommissionSummaryResponse{
				Summary: &summary,
			}, nil
		}
	}

	employee, err := c.GetEmployeeDetails(ctx, employeeID)
	if err != nil {
		return nil, err
	}

	var aggResult struct {
		TotalSales       string
		TotalEarned      string
		TotalPaid        string
		CalculationCount int32
	}
	err = c.db.WithContext(ctx).Model(&CommissionCalculation{}).Select("COALESCE(SUM(total_sales), 0) as total_sales, "+
		"COALESCE(SUM(total_commission), 0) as total_earned, "+
		"COALESCE(SUM(CASE WHEN status = ? THEN total_commission ELSE 0 END), 0) as total_paid, "+
		"COUNT(*) as calculation_count", int32(proto.CommissionStatus_COMMISSION_STATUS_PAID)).Where("employee_id = ? AND calculation_period_start >= ? AND calculation_period_end <= ?", employeeID, startDate, endDate).Scan(&aggResult).Error

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to aggregate commission data: %v", err)
	}

	var recentCalcsGorm []CommissionCalculation
	if err := c.db.WithContext(ctx).Where("employee_id = ? AND calculation_period_start >= ? AND calculation_period_end <= ?", employeeID, startDate, endDate).Order("created_at desc").Limit(5).Find(&recentCalcsGorm).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get recent calculations: %v", err)
	}

	totalSales, _ := decimal.NewFromString(aggResult.TotalSales)
	totalEarned, _ := decimal.NewFromString(aggResult.TotalEarned)
	totalPaid, _ := decimal.NewFromString(aggResult.TotalPaid)
	pending := totalEarned.Sub(totalPaid)

	avgRate := decimal.Zero
	if totalSales.GreaterThan(decimal.Zero) {
		avgRate = totalEarned.Div(totalSales).Mul(decimal.NewFromInt(100))
	}

	var recentCalcsProto []*proto.CommissionCalculation
	for _, calc := range recentCalcsGorm {
		recentCalcsProto = append(recentCalcsProto, c.commissionCalculationToProto(calc))
	}

	summary := &proto.CommissionSummary{
		EmployeeId:            employeeID,
		EmployeeName:          employee.EmployeeName, // Use NATS data
		Period:                req.GetDateRange(),
		TotalSales:            totalSales.StringFixed(2),
		TotalCommissionEarned: totalEarned.StringFixed(2),
		TotalCommissionPaid:   totalPaid.StringFixed(2),
		CommissionPending:     pending.StringFixed(2),
		AverageCommissionRate: avgRate.StringFixed(2),
		CalculationCount:      aggResult.CalculationCount,
		RecentCalculations:    recentCalcsProto,
	}

	jsonData, err := json.Marshal(summary)
	if err == nil {
		c.redis.Set(ctx, cacheKey, jsonData, 2*time.Hour)
	}

	return &proto.GetCommissionSummaryResponse{
		Success: true,
		Summary: summary,
	}, nil
}

func (c *CommissionHandler) GetCommissionReport(ctx context.Context, req *proto.GetCommissionReportRequest) (*proto.GetCommissionReportResponse, error) {
	if req.GetDateRange() == nil || req.GetDateRange().GetStartDate() == "" || req.GetDateRange().GetEndDate() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Date range is required")
	}
	startDate := req.GetDateRange().GetStartDate()
	endDate := req.GetDateRange().GetEndDate()

	var (
		page  = 1
		limit = 20
	)
	if p := req.GetPagination(); p != nil {
		if p.GetPageSize() > 0 {
			limit = int(p.GetPageSize())
		}
		if pNum, err := strconv.Atoi(p.GetPageToken()); err == nil && pNum > 0 {
			page = pNum
		}
	}
	offset := (page - 1) * limit

	baseQuery := c.db.WithContext(ctx).Model(&CommissionCalculation{}).Where("calculation_period_start >= ? AND calculation_period_end <= ?", startDate, endDate)

	if req.GetEmployeeId() > 0 {
		baseQuery = baseQuery.Where("employee_id = ?", req.GetEmployeeId())
	}
	if req.GetStatus() != proto.CommissionStatus_COMMISSION_STATUS_UNSPECIFIED {
		baseQuery = baseQuery.Where("status = ?", req.GetStatus())
	}

	var overallTotals struct {
		Calculated string
		Paid       string
	}
	err := baseQuery.Select("COALESCE(SUM(total_commission), 0) as calculated, "+
		"COALESCE(SUM(CASE WHEN status = ? THEN total_commission ELSE 0 END), 0) as paid", int32(proto.CommissionStatus_COMMISSION_STATUS_PAID)).Scan(&overallTotals).Error
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get overall totals: %v", err)
	}
	totalCalculated, _ := decimal.NewFromString(overallTotals.Calculated)
	totalPaid, _ := decimal.NewFromString(overallTotals.Paid)
	totalPending := totalCalculated.Sub(totalPaid)

	var totalEmployees int64
	if err := baseQuery.Distinct("employee_id").Count(&totalEmployees).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count employees: %v", err)
	}

	var employeeSummariesData []struct {
		EmployeeID            int64
		TotalSales            string
		TotalCommissionEarned string
		TotalCommissionPaid   string
		CalculationCount      int32
	}
	err = baseQuery.Select("employee_id, "+
		"COALESCE(SUM(total_sales), 0) as total_sales, "+
		"COALESCE(SUM(total_commission), 0) as total_commission_earned, "+
		"COALESCE(SUM(CASE WHEN status = ? THEN total_commission ELSE 0 END), 0) as total_commission_paid, "+
		"COUNT(*) as calculation_count", int32(proto.CommissionStatus_COMMISSION_STATUS_PAID)).Group("employee_id").Order("employee_id").Offset(offset).Limit(limit).Scan(&employeeSummariesData).Error
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get per-employee summaries: %v", err)
	}

	employeeIDs := make([]int64, 0, len(employeeSummariesData))
	for _, summary := range employeeSummariesData {
		employeeIDs = append(employeeIDs, summary.EmployeeID)
	}

	employeeNameMap, err := c.GetEmployeeNamesBatch(ctx, employeeIDs)
	if err != nil {
		employeeNameMap = make(map[int64]string)
	}

	var summariesProto []*proto.CommissionSummary
	for _, data := range employeeSummariesData {
		totalSales, _ := decimal.NewFromString(data.TotalSales)
		totalEarned, _ := decimal.NewFromString(data.TotalCommissionEarned)
		totalPaid, _ := decimal.NewFromString(data.TotalCommissionPaid)
		pending := totalEarned.Sub(totalPaid)
		avgRate := decimal.Zero
		if totalSales.GreaterThan(decimal.Zero) {
			avgRate = totalEarned.Div(totalSales).Mul(decimal.NewFromInt(100))
		}

		summary := &proto.CommissionSummary{
			EmployeeId:            data.EmployeeID,
			EmployeeName:          employeeNameMap[data.EmployeeID],
			Period:                req.GetDateRange(),
			TotalSales:            data.TotalSales,
			TotalCommissionEarned: data.TotalCommissionEarned,
			TotalCommissionPaid:   data.TotalCommissionPaid,
			CommissionPending:     pending.StringFixed(2),
			AverageCommissionRate: avgRate.StringFixed(2),
			CalculationCount:      data.CalculationCount,
		}
		summariesProto = append(summariesProto, summary)
	}

	nextPageToken := ""
	if int64(offset+limit) < totalEmployees {
		nextPageToken = strconv.Itoa(page + 1)
	}

	return &proto.GetCommissionReportResponse{
		Success:                    true,
		EmployeeSummaries:          summariesProto,
		TotalCommissionsCalculated: totalCalculated.StringFixed(2),
		TotalCommissionsPaid:       totalPaid.StringFixed(2),
		TotalCommissionsPending:    totalPending.StringFixed(2),
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(totalEmployees),
		},
	}, nil
}
