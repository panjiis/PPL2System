package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	lib "syntra-system/internal/utils"
	proto "syntra-system/proto/protogen/commissions"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Localization for WIB (UTC+7)
var wib, _ = time.LoadLocation("Asia/Jakarta")

func (c *CommissionHandler) GetCommissionCalculation(ctx context.Context, req *proto.GetCommissionCalculationRequest) (*proto.GetCommissionCalculationResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}

	cacheKey := fmt.Sprintf("%s%d", COMMISSION_CALCULATION_CACHE_PREFIX, req.GetId())

	val, err := c.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var cachedCalc CommissionCalculation
		if err := json.Unmarshal([]byte(val), &cachedCalc); err == nil {
			return &proto.GetCommissionCalculationResponse{
				CommissionCalculation: c.commissionCalculationToProto(cachedCalc),
			}, nil
		}
	} else if err != redis.Nil {
		fmt.Printf("Redis error on GET: %v. Falling back to DB.\n", err)
	}

	var dbCalc CommissionCalculation
	if err := c.db.WithContext(ctx).Preload("CommissionDetails").Preload("CommissionPayment").First(&dbCalc, req.GetId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Commission calculation with ID %d not found", req.GetId())
		}
		return nil, status.Errorf(codes.Internal, "Failed to get commission calculation from DB: %v", err)
	}

	jsonData, err := json.Marshal(&dbCalc)
	if err == nil {
		if err := c.redis.Set(ctx, cacheKey, jsonData, 24*time.Hour).Err(); err != nil {
			fmt.Printf("Failed to set cache for key %s: %v\n", cacheKey, err)
		}
	}

	return &proto.GetCommissionCalculationResponse{
		Success:               true,
		CommissionCalculation: c.commissionCalculationToProto(dbCalc),
	}, nil
}

func (c *CommissionHandler) ListCommissionCalculations(ctx context.Context, req *proto.ListCommissionCalculationsRequest) (*proto.ListCommissionCalculationsResponse, error) {
	var (
		page   = 1
		limit  = -1
		offset = 0
	)

	if p := req.GetPagination(); p != nil {
		if p.GetPageSize() > 0 {
			limit = int(p.GetPageSize())
			if pagenum, err := strconv.Atoi(p.GetPageToken()); err == nil && pagenum > 0 {
				page = pagenum
			}
			offset = (page - 1) * limit
		}
	}

	baseQuery := c.db.WithContext(ctx).Model(&CommissionCalculation{})

	if req.GetEmployeeId() > 0 {
		baseQuery = baseQuery.Where("employee_id = ?", req.GetEmployeeId())
	}
	if req.GetStatus() != proto.CommissionStatus_COMMISSION_STATUS_UNSPECIFIED {
		baseQuery = baseQuery.Where("status = ?", req.GetStatus())
	}
	if period := req.GetCalculationPeriod(); period != nil && period.GetStartDate() != "" && period.GetEndDate() != "" {
		baseQuery = baseQuery.Where("calculation_period_start >= ? AND calculation_period_end <= ?", period.GetStartDate(), period.GetEndDate())
	}

	var totalCount int64
	if err := baseQuery.Count(&totalCount).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to count calculations: %v", err)
	}

	query := baseQuery.Order("created_at DESC").Preload("CommissionPayment")
	if limit != -1 {
		query = query.Offset(offset).Limit(limit)
	}

	var calculations []CommissionCalculation
	if err := query.Find(&calculations).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to retrieve calculations: %v", err)
	}

	employeeIDs := make([]int64, 0, len(calculations))
	for _, calc := range calculations {
		employeeIDs = append(employeeIDs, calc.EmployeeID)
	}

	employeeNameMap, err := c.GetEmployeeNamesBatch(ctx, employeeIDs)
	if err != nil {
		employeeNameMap = make(map[int64]string)
	}

	// --- Manager names ---
	managerIDsSet := make(map[int64]bool)
	for _, calc := range calculations {
		if calc.CalculatedBy > 0 {
			managerIDsSet[calc.CalculatedBy] = true
		}
		if calc.ApprovedBy != nil && *calc.ApprovedBy > 0 {
			managerIDsSet[*calc.ApprovedBy] = true
		}
	}
	managerIDs := make([]int64, 0, len(managerIDsSet))
	for id := range managerIDsSet {
		managerIDs = append(managerIDs, id)
	}

	managerNameMap := make(map[int64]string)
	if len(managerIDs) > 0 {
		var err error
		managerNameMap, err = c.GetManagerNamesBatch(ctx, managerIDs)
		if err != nil {
			log.Printf("Warning: GetManagerNamesBatch failed: %v", err)
		}
	}

	var protoCalculations []*proto.CommissionCalculation
	for _, calc := range calculations {
		protoCalc := c.commissionCalculationToProto(calc)
		protoCalc.Employee = &proto.EmployeeSummary{
			Id:           calc.EmployeeID,
			EmployeeName: employeeNameMap[calc.EmployeeID],
		}

		if calc.CalculatedBy > 0 {
			if name, exists := managerNameMap[calc.CalculatedBy]; exists {
				protoCalc.CalculatedByName = name
			}
		}

		if calc.ApprovedBy != nil && *calc.ApprovedBy > 0 {
			if name, exists := managerNameMap[*calc.ApprovedBy]; exists {
				protoCalc.ApprovedByName = &name
			}
		}

		protoCalculations = append(protoCalculations, protoCalc)
	}

	nextPageToken := ""
	if limit != -1 && int64(offset+limit) < totalCount {
		nextPageToken = strconv.Itoa(page + 1)
	}

	return &proto.ListCommissionCalculationsResponse{
		Success:                true,
		CommissionCalculations: protoCalculations,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(totalCount),
		},
	}, nil
}

func (c *CommissionHandler) ApproveCommission(ctx context.Context, req *proto.ApproveCommissionRequest) (*proto.ApproveCommissionResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}
	if req.GetApprovedBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Approved By (user ID) is required")
	}

	var calculation CommissionCalculation

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&calculation, req.GetCommissionCalculationId()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return status.Errorf(codes.NotFound, "Commission calculation with ID %d not found", req.GetCommissionCalculationId())
			}
			return status.Errorf(codes.Internal, "Failed to retrieve calculation: %v", err)
		}

		if calculation.Status != int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED) {
			return status.Errorf(codes.FailedPrecondition, "Commission can only be approved from CALCULATED status. Current status: %s", proto.CommissionStatus_name[calculation.Status])
		}

		approvedByID := req.GetApprovedBy()
		calculation.Status = int32(proto.CommissionStatus_COMMISSION_STATUS_APPROVED)
		calculation.ApprovedBy = &approvedByID
		if req.GetApprovalNotes() != "" {
			calculation.Notes = lib.StrPtr(req.GetApprovalNotes())
		}

		if err := tx.Save(&calculation).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to save approval: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	c.InvalidateCommissionCaches(ctx, req.GetCommissionCalculationId())

	if err := c.db.WithContext(ctx).Preload("CommissionDetails").Preload("CommissionPayment").First(&calculation, req.GetCommissionCalculationId()).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to retrieve updated data for response: %v", err)
	}

	event := CommissionFinalizedEvent{
		EventType:      "commission.finalized",
		Timestamp:      time.Now(),
		CalculationID:  calculation.ID, // Asumsi 'calculation' adalah model GORM Anda
		EmployeeID:     calculation.EmployeeID,
		PeriodStart:    calculation.CalculationPeriodStart,
		PeriodEnd:      calculation.CalculationPeriodEnd,
		TotalSales:     calculation.TotalSales,
		TotalCommission: calculation.TotalCommission,
	}

	// 2. Publish
	c.publishCommissionFinalizedEvent(event)

	c.publishCommissionStatusEvent(
			"commission.status.updated",
			calculation.ID,
			calculation.Status, // Kirim status baru (APPROVED)
	)

	return &proto.ApproveCommissionResponse{
		Success:               true,
		Message:               lib.StrPtr("Commission approved successfully"),
		CommissionCalculation: c.commissionCalculationToProto(calculation),
	}, nil
}

func (c *CommissionHandler) RejectCommission(ctx context.Context, req *proto.RejectCommissionRequest) (*proto.RejectCommissionResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}
	if req.GetRejectedBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Approved By (user ID) is required")
	}
	if req.GetRejectionReason() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Rejection Reason is required")
	}

	var calculation CommissionCalculation

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Ambil dan Kunci baris data untuk mencegah race condition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&calculation, req.GetCommissionCalculationId()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return status.Errorf(codes.NotFound, "Commission calculation with ID %d not found", req.GetCommissionCalculationId())
			}
			return status.Errorf(codes.Internal, "Failed to retrieve calculation: %v", err)
		}

		if calculation.Status != int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED) {
			return status.Errorf(codes.FailedPrecondition, "Commission can only be approved from CALCULATED status. Current status: %s", proto.CommissionStatus_name[calculation.Status])
		}

		calculation.Status = int32(proto.CommissionStatus_COMMISSION_STATUS_DRAFT)
		calculation.ApprovedBy = nil

		if err := tx.Save(&calculation).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to save approval: %v", err)
		}

		manager, err := c.GetManagerDetails(ctx, req.GetRejectedBy())
		if err != nil {
			return nil
		}

		rejectionNote := fmt.Sprintf("\n[REJECTED by %s on %s]: %s",
			manager.ManagerName,
			time.Now().In(wib).Format("2006-01-02 15:04:05"),
			req.GetRejectionReason(),
		)

		// currentNotes := ""
		// if calculation.Notes != nil {
		// 	currentNotes = *calculation.Notes
		// }
		// newNotes := currentNotes + rejectionNote
		calculation.Notes = &rejectionNote

		if err := tx.Save(&calculation).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to save rejection: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	c.InvalidateCommissionCaches(ctx, req.GetCommissionCalculationId())

	if err := c.db.WithContext(ctx).Preload("CommissionDetails").First(&calculation, req.GetCommissionCalculationId()).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to retrieve updated data for response: %v", err)
	}

	return &proto.RejectCommissionResponse{
		Success:               true,
		Message:               lib.StrPtr("Commission rejected and moved to draft"),
		CommissionCalculation: c.commissionCalculationToProto(calculation),
	}, nil
}

func (c *CommissionHandler) BulkApproveCommissions(ctx context.Context, req *proto.BulkApproveCommissionsRequest) (*proto.BulkApproveCommissionsResponse, error) {
	if len(req.GetCommissionCalculationIds()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation IDs are required")
	}
	if req.GetApprovedBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Approved By (user ID) is required")
	}

	var (
		approvedCalculations []CommissionCalculation
		errorMessages        []string
		wg                   sync.WaitGroup
		mu                   sync.Mutex
	)

	for _, calcID := range req.GetCommissionCalculationIds() {
		wg.Add(1)

		go func(id int64) {
			defer wg.Done()

			var calculation CommissionCalculation

			err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&calculation, id).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						return fmt.Errorf("not found")
					}
					return fmt.Errorf("DB Error: %v", err)
				}

				if calculation.Status != int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED) {
					return fmt.Errorf("invalid status: %s", proto.CommissionStatus_name[calculation.Status])
				}

				approvedByID := req.GetApprovedBy()
				calculation.Status = int32(proto.CommissionStatus_COMMISSION_STATUS_APPROVED)
				calculation.ApprovedBy = &approvedByID
				if req.GetApprovalNotes() != "" {
					calculation.Notes = lib.StrPtr(req.GetApprovalNotes())
				}

				if err := tx.Save(&calculation).Error; err != nil {
					return fmt.Errorf("failed to save: %w", err)
				}

				return nil
			})

			if err != nil {
				mu.Lock()
				errorMessages = append(errorMessages, fmt.Sprintf("Calculation ID %d: %v", id, err))
				mu.Unlock()
				return
			}

			event := CommissionFinalizedEvent{
        EventType:      "commission.finalized",
        Timestamp:      time.Now(),
        CalculationID:  calculation.ID,
        EmployeeID:     calculation.EmployeeID,
        PeriodStart:    calculation.CalculationPeriodStart,
        PeriodEnd:      calculation.CalculationPeriodEnd,
        TotalSales:     calculation.TotalSales,
        TotalCommission: calculation.TotalCommission,
      }
      c.publishCommissionFinalizedEvent(event)

			c.publishCommissionStatusEvent(
          "commission.status.updated",
          calculation.ID,
          calculation.Status, // Kirim status baru (APPROVED)
      )

			c.InvalidateCommissionCaches(ctx, id)

			mu.Lock()
			approvedCalculations = append(approvedCalculations, calculation)
			mu.Unlock()
		}(calcID)
	}

	wg.Wait()

	var protoCalculations []*proto.CommissionCalculation
	for _, calc := range approvedCalculations {
		c.db.WithContext(ctx).Preload("CommissionDetails").Preload("CommissionPayment").First(&calc)
		protoCalculations = append(protoCalculations, c.commissionCalculationToProto(calc))
	}

	msg := fmt.Sprintf("Bulk approval completed. Success: %d, Failed: %d.", len(approvedCalculations), len(errorMessages))
	if len(errorMessages) > 0 {
		msg += " Check 'errors' field for details."
	}

	return &proto.BulkApproveCommissionsResponse{
		Success:              true,
		Message:              &msg,
		ApprovedCalculations: protoCalculations,
		Errors:               errorMessages,
		SuccessCount:         int32(len(approvedCalculations)),
		ErrorCount:           int32(len(errorMessages)),
	}, nil
}
