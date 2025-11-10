package handler

import (
	"context"
	"encoding/json"
	"fmt"
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
		CommissionCalculation: c.commissionCalculationToProto(dbCalc),
	}, nil
}

func (c *CommissionHandler) ListCommissionCalculations(ctx context.Context, req *proto.ListCommissionCalculationsRequest) (*proto.ListCommissionCalculationsResponse, error) {
	var (
		page   = 1
		// 1. Ubah default 'limit' menjadi -1. Ini adalah "no limit" untuk GORM.
		limit  = -1 
		offset = 0
	)

	// 2. Logika ini sekarang hanya berjalan JIKA pagination block ada
	if p := req.GetPagination(); p != nil {
		
        // 3. Hanya hitung limit/offset JIKA PageSize > 0
		if p.GetPageSize() > 0 {
			limit = int(p.GetPageSize())
			if pagenum, err := strconv.Atoi(p.GetPageToken()); err == nil && pagenum > 0 {
				page = pagenum
			}
			offset = (page - 1) * limit
		}
        // Jika PageSize adalah 0, 'limit' akan tetap -1, yang berarti "ambil semua".
	}

	query := c.db.WithContext(ctx).Model(&CommissionCalculation{})

	if req.GetEmployeeId() > 0 {
		query = query.Where("employee_id = ?", req.GetEmployeeId())
	}
	if req.GetStatus() != proto.CommissionStatus_COMMISSION_STATUS_UNSPECIFIED {
		query = query.Where("status = ?", req.GetStatus())
	}
	if period := req.GetCalculationPeriod(); period != nil && period.GetStartDate() != "" && period.GetEndDate() != "" {
		query = query.Where("calculation_period_start >= ? AND calculation_period_end <= ?", period.GetStartDate(), period.GetEndDate())
	}

	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to count calculations: %v", err)
	}

	var calculations []CommissionCalculation

    // 4. Ubah cara Anda membuat kueri
    // Terapkan Order dan Preload terlebih dahulu
	query = query.
		Order("created_at desc").
		Preload("CommissionPayment")

    // 5. Terapkan Offset dan Limit HANYA JIKA limit BUKAN -1
	if limit != -1 {
		query = query.Offset(offset).Limit(limit)
	}

    // 6. Jalankan Find
	err := query.Find(&calculations).Error

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to retrieve calculations: %v", err)
	}

	var protoCalculations []*proto.CommissionCalculation
	for _, calc := range calculations {
		protoCalculations = append(protoCalculations, c.commissionCalculationToProto(calc))
	}

	nextPageToken := ""
    // 7. Hanya buat NextPageToken JIKA pagination diterapkan
	if limit != -1 && int64(offset+limit) < totalCount {
		nextPageToken = strconv.Itoa(page + 1)
	}

	return &proto.ListCommissionCalculationsResponse{
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

	return &proto.ApproveCommissionResponse{
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

		rejectionNote := fmt.Sprintf("\n[REJECTED by User ID %d on %s]: %s",
			req.GetRejectedBy(),
			time.Now().Format("2006-01-02 15:04:05"),
			req.GetRejectionReason(),
		)

		currentNotes := ""
		if calculation.Notes != nil {
			currentNotes = *calculation.Notes
		}
		newNotes := currentNotes + rejectionNote
		calculation.Notes = &newNotes

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

	return &proto.BulkApproveCommissionsResponse{
		ApprovedCalculations: protoCalculations,
		Errors:               errorMessages,
		SuccessCount:         int32(len(approvedCalculations)),
		ErrorCount:           int32(len(errorMessages)),
	}, nil
}
