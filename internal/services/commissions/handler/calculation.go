package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	proto "syntra-system/proto/protogen/commissions"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func (c *CommissionHandler) CalculateCommission(ctx context.Context, req *proto.CalculateCommissionRequest) (*proto.CalculateCommissionResponse, error) {
	if req.GetEmployeeId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Employee ID is required")
	}
	if req.GetPeriodStart() == "" || req.GetPeriodEnd() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Calculation period start and end dates are required")
	}
	if req.CalculatedBy <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Calculated By (user ID) is required")
	}

	result, err := c.CalculateCommissionLogic(ctx, req.GetEmployeeId(), req.GetPeriodStart(), req.GetPeriodEnd())
	if err != nil {
		return nil, err
	}

	calculationModel := CommissionCalculation{
		EmployeeID:             req.GetEmployeeId(),
		CalculationPeriodStart: req.GetPeriodStart(),
		CalculationPeriodEnd:   req.GetPeriodEnd(),
		TotalSales:             result.TotalSales.StringFixed(2),
		BaseCommission:         result.BaseCommission.StringFixed(2),
		BonusCommission:        result.BonusCommission.StringFixed(2),
		TotalCommission:        result.TotalCommission.StringFixed(2),
		Status:                 int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
		CalculatedBy:           req.GetCalculatedBy(),
		CommissionDetails:      result.Details,
	}

	if req.GetSaveCalculation() {
		err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&calculationModel).Error; err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to save commission calculation: %v", err)
		}
	}

	return &proto.CalculateCommissionResponse{
		CommissionCalculation: c.commissionCalculationToProto(calculationModel),
		Breakdown:             result.Breakdown,
		IsPreview:             !req.GetSaveCalculation(),
	}, nil
}

func (c *CommissionHandler) RecalculateCommission(ctx context.Context, req *proto.RecalculateCommissionRequest) (*proto.RecalculateCommissionResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}
	if req.GetRecalculatedBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Recalculated By (user ID) is required")
	}

	var existingCalc CommissionCalculation
	if err := c.db.WithContext(ctx).First(&existingCalc, req.GetCommissionCalculationId()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Commission calculation with ID %d not found", req.GetCommissionCalculationId())
		}
		return nil, status.Errorf(codes.Internal, "Failed to get existing calculation: %v", err)
	}

	result, err := c.CalculateCommissionLogic(ctx, existingCalc.EmployeeID, existingCalc.CalculationPeriodStart, existingCalc.CalculationPeriodEnd)
	if err != nil {
		return nil, err
	}

	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// a. Hapus Detail Lama
		if err := tx.Where("commission_calculation_id = ?", existingCalc.ID).Delete(&CommissionDetail{}).Error; err != nil {
			return fmt.Errorf("failed to delete old details: %w", err)
		}

		// b. Update Data Induk
		updates := map[string]interface{}{
			"TotalSales":      result.TotalSales.StringFixed(2),
			"TotalCommission": result.TotalCommission.StringFixed(2),
			"BaseCommission":  result.BaseCommission.StringFixed(2),
			"BonusCommission": result.BonusCommission.StringFixed(2),
			"Status":          int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
			"CalculatedBy":    req.GetRecalculatedBy(),
			"Notes":           req.Notes,
			"ApprovedBy":      nil, // Reset approval status
		}
		if err := tx.Model(&CommissionCalculation{}).Where("id = ?", existingCalc.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update calculation header: %w", err)
		}

		// c. Simpan Detail Baru
		for i := range result.Details {
			result.Details[i].CommissionCalculationID = existingCalc.ID
		}
		if len(result.Details) > 0 {
			if err := tx.Create(&result.Details).Error; err != nil {
				return fmt.Errorf("failed to create new details: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to save recalculated commission: %v", err)
	}

	// Ambil kembali data yang sudah diupdate untuk respons yang akurat
	if err := c.db.WithContext(ctx).Preload("CommissionDetails").First(&existingCalc, req.GetCommissionCalculationId()).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to retrieve updated calculation for response: %v", err)
	}

	// 5. Hapus Cache
	c.InvalidateCommissionCaches(ctx, existingCalc.ID)

	// 6. Kirim Respons
	return &proto.RecalculateCommissionResponse{
		CommissionCalculation: c.commissionCalculationToProto(existingCalc),
		Breakdown:             result.Breakdown,
	}, nil
}

func (c *CommissionHandler) BulkCalculateCommissions(ctx context.Context, req *proto.BulkCalculateCommissionsRequest) (*proto.BulkCalculateCommissionsResponse, error) {
	if len(req.GetEmployeeIds()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Employee IDs are required")
	}
	if req.GetPeriodStart() == "" || req.GetPeriodEnd() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Calculation period start and end dates are required")
	}
	if req.GetCalculatedBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Calculated By (user ID) is required")
	}

	var (
		successfulCalculations []CommissionCalculation
		errorMessages          []string
		wg                     sync.WaitGroup
		mu                     sync.Mutex
	)

	for _, employeeID := range req.GetEmployeeIds() {
		wg.Add(1)
		go func(eID int64) {
			defer wg.Done()

			calcResult, err := c.CalculateCommissionLogic(ctx, eID, req.GetPeriodStart(), req.GetPeriodEnd())
			if err != nil {
				mu.Lock()
				errorMessages = append(errorMessages, fmt.Sprintf("Employee ID %d: %v", eID, err))
				mu.Unlock()
				return
			}

			calculationModel := CommissionCalculation{
				EmployeeID:             eID,
				CalculationPeriodStart: req.GetPeriodStart(),
				CalculationPeriodEnd:   req.GetPeriodEnd(),
				TotalSales:             calcResult.TotalSales.StringFixed(2),
				BaseCommission:         calcResult.BaseCommission.StringFixed(2),
				BonusCommission:        calcResult.BonusCommission.StringFixed(2),
				TotalCommission:        calcResult.TotalCommission.StringFixed(2),
				Status:                 int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
				CalculatedBy:           req.GetCalculatedBy(),
				CommissionDetails:      calcResult.Details,
			}

			if err := c.db.WithContext(ctx).Create(&calculationModel).Error; err != nil {
				mu.Lock()
				errorMessages = append(errorMessages, fmt.Sprintf("Employee ID %d: failed to save - %v", eID, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			successfulCalculations = append(successfulCalculations, calculationModel)
			mu.Unlock()
		}(employeeID)
	}

	wg.Wait()

	var protoCalculations []*proto.CommissionCalculation
	for _, calc := range successfulCalculations {
		protoCalculations = append(protoCalculations, c.commissionCalculationToProto(calc))
	}

	return &proto.BulkCalculateCommissionsResponse{
		Calculations: protoCalculations,
		Errors:       errorMessages,
		SuccessCount: int32(len(successfulCalculations)),
		ErrorCount:   int32(len(errorMessages)),
	}, nil
}

func (c *CommissionHandler) CalculateCommissionLogic(ctx context.Context, employeeID int64, periodStart, periodEnd string) (*calculationResult, error) {
	// 1. Ambil Data Karyawan & Tiers (Sama seperti sebelumnya)
	// employeeId, err := json.Marshal(map[string]interface{}{
	// 	"employee_id": employeeID,
	// })

	// if err != nil {
	// 	return nil, status.Errorf(codes.Internal, "Failed to marshal request data: %v", err)
	// }

	// msg, err := c.nats.RequestWithContext(ctx, "employee.get", employeeId)
	// if err != nil {
	// 	return nil, status.Errorf(codes.Internal, "Failed to request employee data via NATS: %v", err)
	// }

	// var employee struct {
	// 	Found bool `json:"found"`
	// 	// Error          string `json:"error,omitempty"`
	// 	// EmployeeName   string `json:"employee_name,omitempty"`
	// 	// Email          string `json:"email,omitempty"`
	// 	// Position       string `json:"position,omitempty"`
	// 	CommissionRate string `json:"commission_rate,omitempty"`
	// 	CommissionType int32  `json:"commission_type,omitempty"`
	// }

	// // =======================================================
	// // == DEBUGGING ==
	// // =======================================================
	// employeeJSON, err := json.MarshalIndent(employee, "", "  ")
	// if err != nil {
	// 	log.Printf("Error marshaling 'employee' for debug: %v", err)
	// } else {
	// 	log.Println("--- DEBUG: HASIL DARI NATS (employee.get) ---")
	// 	log.Println(string(employeeJSON)) // Cetak sebagai string JSON
	// 	log.Println("--- DEBUG: AKHIR DARI NATS (employee) ---")
	// }
	// // =======================================================

	// if err := json.Unmarshal(msg.Data, &employee); err != nil {
	// 	return nil, status.Errorf(codes.Internal, "Failed to parse employee response: %v", err)
	// }

	// if !employee.Found {
	// 	return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
	// }

	reqData, err := json.Marshal(map[string]interface{}{
		"employee_id": employeeID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal request data: %v", err)
	}

	commissionTierMsg, err := c.nats.RequestWithContext(ctx, "employee.get-commission-tier", reqData)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to request employee commission tiers via NATS: %v", err)
	}

	var resp struct {
		Found bool `json:"found"`
		// Error          string               `json:"error,omitempty"`
		CommissionType int32                `json:"commission_type,omitempty"`
		CommissionRate string               `json:"commission_rate,omitempty"` // <-- TAMBAHKAN BARIS INI
		Tiers          []CommissionTierInfo `json:"tiers,omitempty"`
	}

	if err := json.Unmarshal(commissionTierMsg.Data, &resp); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to parse NATS response: %v", err)
	}

	if !resp.Found {
		return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
	}

	// =======================================================
	// == DEBUGGING ==
	// =======================================================
	respJSON, err := json.MarshalIndent(resp, "", "  ") // Indent dengan 2 spasi
	if err != nil {
		log.Printf("Error marshaling 'resp' for debug: %v", err)
	} else {
		log.Println("--- DEBUG: HASIL DARI NATS (employee.get-commission-tier) ---")
		log.Println(string(respJSON)) // Cetak sebagai string JSON
		log.Println("--- DEBUG: AKHIR DARI NATS (resp) ---")
	}
	// =======================================================

	// Use the tiers from the NATS response directly
	var tiers []CommissionTierInfo
	if resp.CommissionType == 3 {
		tiers = resp.Tiers
	}

	salesItems, err := c.getSalesData(ctx, employeeID, periodStart, periodEnd)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Service error: %v", err)
	}

	// =======================================================
	// == DEBUGGING ==
	// =======================================================
	salesItemsJSON, err := json.MarshalIndent(salesItems, "", "  ") // Indent dengan 2 spasi
	if err != nil {
		log.Printf("Error marshaling salesItems for debug: %v", err)
	} else {
		log.Println("--- DEBUG: HASIL DARI getSalesData (salesItems) ---")
		log.Println(string(salesItemsJSON))
		log.Println("--- DEBUG: AKHIR DARI salesItems ---")
	}
	// =======================================================

	// 3. Lakukan Kalkulasi (Sama seperti sebelumnya)
	totalSales := decimal.Zero
	totalCommission := decimal.Zero
	baseCommission := decimal.Zero
	bonusCommission := decimal.Zero
	var commissionDetails []CommissionDetail
	var breakdownDetails []*proto.TierCommission

	for _, item := range salesItems { // <--- Tipe data sekarang adalah models.SalesDataItem
		itemSales, err := decimal.NewFromString(item.SalesAmount)
		if err != nil {
			log.Printf("Skipping item %d, invalid sales amount: %s", item.ID, item.SalesAmount)
			continue
		}
		totalSales = totalSales.Add(itemSales)
	}

	switch resp.CommissionType {
	case 1:
		rate, _ := decimal.NewFromString(resp.CommissionRate)
		totalCommission = totalSales.Mul(rate).Div(decimal.NewFromInt(100))
		baseCommission = totalCommission
	case 3:
		// remainingSales := totalSales
		for _, tier := range tiers {
			tierMin, _ := decimal.NewFromString(tier.MinSalesAmount)
			tierRate, _ := decimal.NewFromString(tier.CommissionRate)

			salesInTier := decimal.Zero

			if totalSales.GreaterThan(tierMin) {
				tierMaxStr := tier.MaxSalesAmount
				if tierMaxStr != "" {
					tierMax, _ := decimal.NewFromString(tierMaxStr)
					if totalSales.LessThanOrEqual(tierMax) {
						salesInTier = totalSales.Sub(tierMin)
					} else {
						salesInTier = tierMax.Sub(tierMin)
					}
				} else {
					salesInTier = totalSales.Sub(tierMin)
				}
			}

			if salesInTier.GreaterThan(decimal.Zero) {
				tierComm := salesInTier.Mul(tierRate).Div(decimal.NewFromInt(100))
				totalCommission = totalCommission.Add(tierComm)

				breakdownDetails = append(breakdownDetails, &proto.TierCommission{
					TierMinAmount:   tier.MinSalesAmount,
					TierMaxAmount:   tier.MaxSalesAmount,
					TierRate:        tier.CommissionRate,
					TierSalesAmount: salesInTier.StringFixed(2),
					TierCommission:  tierComm.StringFixed(2),
				})
			}
		}
		baseCommission = totalCommission
	case 2:
		fixedAmount, _ := decimal.NewFromString(resp.CommissionRate)
		itemCount := decimal.NewFromInt(int64(len(salesItems)))
		totalCommission = fixedAmount.Mul(itemCount)
		baseCommission = totalCommission
	default:
		return nil, status.Errorf(codes.FailedPrecondition, "Unknown commission type: %d", resp.CommissionType)
	}

	employeeRate, _ := decimal.NewFromString(resp.CommissionRate)
	for _, item := range salesItems {
		salesAmount, _ := decimal.NewFromString(item.SalesAmount)
		itemCommission := salesAmount.Mul(totalCommission).Div(totalSales)
		if totalSales.IsZero() {
			itemCommission = decimal.Zero
		}

		commissionDetails = append(commissionDetails, CommissionDetail{
			OrderItemID:      item.ID,
			ProductCode:      item.ProductCode,
			SalesAmount:      item.SalesAmount,
			CommissionRate:   employeeRate.StringFixed(4),
			CommissionAmount: itemCommission.StringFixed(2),
		})
	}

	// 4. Buat Breakdown (Sama seperti sebelumnya)
	effectiveRate := "0.00"
	if totalSales.GreaterThan(decimal.Zero) {
		effectiveRate = totalCommission.Div(totalSales).Mul(decimal.NewFromInt(100)).StringFixed(2)
	}

	breakdown := &proto.CommissionBreakdown{
		TotalSales:              totalSales.StringFixed(2),
		BaseCommissionRate:      resp.CommissionRate,
		BaseCommissionAmount:    baseCommission.StringFixed(2),
		TierCommissions:         breakdownDetails,
		BonusCommission:         bonusCommission.StringFixed(2),
		TotalCommission:         totalCommission.StringFixed(2),
		EffectiveCommissionRate: effectiveRate,
	}

	// 5. Kembalikan hasilnya dalam struct
	return &calculationResult{
		TotalSales:      totalSales,
		TotalCommission: totalCommission,
		BaseCommission:  baseCommission,
		BonusCommission: bonusCommission,
		Details:         commissionDetails,
		Breakdown:       breakdown,
	}, nil
}

func (c *CommissionHandler) getSalesData(ctx context.Context, employeeID int64, periodStart, periodEnd string) ([]SalesDataItem, error) {
	var salesItems []SalesDataItem

	// Kueri ke tabel LOKAL 'sales_data_items'
	err := c.db.WithContext(ctx).Model(&SalesDataItem{}).
		Where("employee_id = ?", employeeID).
		Where("order_date BETWEEN ? AND ?", periodStart, periodEnd).
		Where("is_returned = ?", false). // <-- Hanya hitung yang tidak diretur
		Find(&salesItems).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get local sales data: %w", err)
	}

	return salesItems, nil
}
