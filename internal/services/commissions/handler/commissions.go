package handler

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	proto "syntra-system/proto/protogen/commissions"
)

const (
	COMMISSION_CALCULATION_CACHE_PREFIX = "commission_calculation:" 
	COMMISSION_REPORT_CACHE_PREFIX      = "commission_report:"      
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
type CommissionCalculation struct {
	ID                     int64      `gorm:"primaryKey;autoIncrement"`
	EmployeeID             int64      `gorm:"index;not null"` //
	CalculationPeriodStart string     `gorm:"not null"`
	CalculationPeriodEnd   string     `gorm:"not null"`
	TotalSales             string     `gorm:"type:decimal(18,2);not null"`
	BaseCommission         string     `gorm:"type:decimal(18,2);not null"`
	BonusCommission        string     `gorm:"type:decimal(18,2);not null"` //
	TotalCommission        string     `gorm:"type:decimal(18,2);not null"`
	// Status merepresentasikan enum CommissionStatus dari proto (e.g., 2 untuk CALCULATED, 3 untuk APPROVED)
	Status       int32      `gorm:"index;not null"` //
	CalculatedBy int64      `gorm:"not null"`
	ApprovedBy   *int64     //
	Notes        *string    `gorm:"type:text"`
	CreatedAt    *time.Time `gorm:"autoCreateTime"`
	UpdatedAt    *time.Time `gorm:"autoUpdateTime"`

	// Relasi
	CommissionDetails []CommissionDetail `gorm:"foreignKey:CommissionCalculationID"`
	CommissionPayment *CommissionPayment `gorm:"foreignKey:CommissionCalculationID"`
}

type CommissionDetail struct {
	ID                      int64      `gorm:"primaryKey;autoIncrement"`
	CommissionCalculationID int64      `gorm:"index;not null"`
	OrderItemID             int64      `gorm:"not null"`
	ProductID               int32      `gorm:"not null"`
	SalesAmount             string     `gorm:"type:decimal(18,2);not null"` //
	CommissionRate          string     `gorm:"type:decimal(5,4);not null"`
	CommissionAmount        string     `gorm:"type:decimal(18,2);not null"`
	ProductName             *string    //
	OrderDocumentNumber     *string
	CreatedAt               *time.Time `gorm:"autoCreateTime"`
	UpdatedAt               *time.Time `gorm:"autoUpdateTime"`
}

type CommissionPayment struct {
	ID                      int64      `gorm:"primaryKey;autoIncrement"`
	CommissionCalculationID int64      `gorm:"uniqueIndex;not null"` //
	EmployeeID              int64      `gorm:"not null"`
	PaymentAmount           string     `gorm:"type:decimal(18,2);not null"`
	PaymentDate             string     `gorm:"not null"`
	PaymentTypeID           int32      `gorm:"not null"`
	ReferenceNumber         *string    //
	PaidBy                  int64      `gorm:"not null"` //
	Notes                   *string    `gorm:"type:text"`
	CreatedAt               *time.Time `gorm:"autoCreateTime"`
	UpdatedAt               *time.Time `gorm:"autoUpdateTime"`
}

// --- Struct Helper ---
type EmployeeCommissionInfo struct {
	ID             int64
	CommissionType string `gorm:"column:commission_type"`
	CommissionRate string `gorm:"column:commission_rate"`
}

type CommissionTierInfo struct {
	MinSalesAmount string `gorm:"column:min_sales_amount"`
	MaxSalesAmount string `gorm:"column:max_sales_amount"`
	CommissionRate string `gorm:"column:commission_rate"`
}

type OrderItemData struct {
	ID                  int64  `gorm:"column:id"`
	ProductID           int32  `gorm:"column:product_id"`
	LineTotal           string `gorm:"column:line_total"`
	OrderDocumentNumber string `gorm:"column:document_number"`
	ProductName         string `gorm:"column:product_name"`
}

type calculationResult struct {
	totalSales      decimal.Decimal
	totalCommission decimal.Decimal
	baseCommission  decimal.Decimal
	bonusCommission decimal.Decimal
	details         []CommissionDetail
	breakdown       *proto.CommissionBreakdown
}

// --- Func Helper ---
func (c *CommissionHandler) calculateCommissionLogic(ctx context.Context, employeeID int64, periodStart, periodEnd string) (*calculationResult, error) {
	// 1. Ambil Data Karyawan & Tiers (Sama seperti sebelumnya)
	var employee EmployeeCommissionInfo
	if err := c.db.WithContext(ctx).Table("user.employees").Where("id = ? AND is_active = ?", employeeID, true).First(&employee).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
		}
		return nil, status.Errorf(codes.Internal, "Failed to get employee data: %v", err)
	}

	var tiers []CommissionTierInfo
	if employee.CommissionType == "tiered" {
		if err := c.db.WithContext(ctx).Table("user.commission_tiers").Where("employee_id = ?", employeeID).Order("min_sales_amount asc").Find(&tiers).Error; err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to get commission tiers: %v", err)
		}
	}

	// 2. Ambil Data Penjualan (Sama seperti sebelumnya)
	var salesData []OrderItemData
	err := c.db.WithContext(ctx).Table("pos.order_items as oi").
		Select("oi.id, oi.product_id, oi.line_total, od.document_number, p.product_name").
		Joins("join pos.orders_documents as od on od.id = oi.document_id").
		Joins("join pos.products as p on p.id = oi.product_id").
		Where("oi.serving_employee_id = ?", employeeID).
		Where("p.commission_eligible = ?", true).
		Where("od.orders_date BETWEEN ? AND ?", periodStart, periodEnd).
		Find(&salesData).Error
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get sales data: %v", err)
	}

	// 3. Lakukan Kalkulasi (Sama seperti sebelumnya)
	totalSales := decimal.Zero
	for _, item := range salesData {
		lineTotal, _ := decimal.NewFromString(item.LineTotal)
		totalSales = totalSales.Add(lineTotal)
	}

	totalCommission := decimal.Zero
	baseCommission := decimal.Zero
	bonusCommission := decimal.Zero
	var commissionDetails []CommissionDetail
	var breakdownDetails []*proto.TierCommission
	
	switch employee.CommissionType {
	case "percentage":
		rate, _ := decimal.NewFromString(employee.CommissionRate)
		totalCommission = totalSales.Mul(rate).Div(decimal.NewFromInt(100))
		baseCommission = totalCommission
	case "tiered":
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
					TierMinAmount: tier.MinSalesAmount,
					TierMaxAmount: tier.MaxSalesAmount,
					TierRate: tier.CommissionRate,
					TierSalesAmount: salesInTier.StringFixed(2),
					TierCommission: tierComm.StringFixed(2),
				})
			}
		}
		baseCommission = totalCommission
	case "fixed_amount":
		fixedAmount, _ := decimal.NewFromString(employee.CommissionRate)
		itemCount := decimal.NewFromInt(int64(len(salesData)))
		totalCommission = fixedAmount.Mul(itemCount)
		baseCommission = totalCommission
	default:
		return nil, status.Errorf(codes.FailedPrecondition, "Unknown commission type: %s", employee.CommissionType)
	}

	employeeRate, _ := decimal.NewFromString(employee.CommissionRate)
	for _, item := range salesData {
		salesAmount, _ := decimal.NewFromString(item.LineTotal)
		itemCommission := salesAmount.Mul(totalCommission).Div(totalSales)
		if totalSales.IsZero() {
			itemCommission = decimal.Zero
		}

		commissionDetails = append(commissionDetails, CommissionDetail{
			OrderItemID: item.ID,
			ProductID: item.ProductID,
			SalesAmount: item.LineTotal,
			CommissionRate: employeeRate.StringFixed(4),
			CommissionAmount: itemCommission.StringFixed(2),
			ProductName: strPtr(item.ProductName),
			OrderDocumentNumber: strPtr(item.OrderDocumentNumber),
		})
	}

	// 4. Buat Breakdown (Sama seperti sebelumnya)
	effectiveRate := "0.00"
	if totalSales.GreaterThan(decimal.Zero) {
		effectiveRate = totalCommission.Div(totalSales).Mul(decimal.NewFromInt(100)).StringFixed(2)
	}

	breakdown := &proto.CommissionBreakdown{
		TotalSales:              totalSales.StringFixed(2),
		BaseCommissionRate:      employee.CommissionRate,
		BaseCommissionAmount:    baseCommission.StringFixed(2),
		TierCommissions:         breakdownDetails,
		BonusCommission:         bonusCommission.StringFixed(2),
		TotalCommission:         totalCommission.StringFixed(2),
		EffectiveCommissionRate: effectiveRate,
	}

	// 5. Kembalikan hasilnya dalam struct
	return &calculationResult{
		totalSales:      totalSales,
		totalCommission: totalCommission,
		baseCommission:  baseCommission,
		bonusCommission: bonusCommission,
		details:         commissionDetails,
		breakdown:       breakdown,
	}, nil
}

// --- Handler ---
type CommissionHandler struct {
	proto.UnimplementedCommissionServiceServer
	db    *gorm.DB
	redis *redis.Client
}

func NewCommissionHandler(db *gorm.DB, redisClient *redis.Client) *CommissionHandler {
	return &CommissionHandler{
		db:    db,
		redis: redisClient,
	}
}

func (c *CommissionHandler) InvalidateCommissionCaches(ctx context.Context, calcIDs ...int64) {
	// Hapus cache yang bersifat umum atau agregat
	// _ = c.redis.Del(ctx, "some_general_commission_report_key")

	// Hapus cache untuk setiap kalkulasi yang spesifik
	for _, id := range calcIDs {
		cacheKey := fmt.Sprintf("%s%d", COMMISSION_CALCULATION_CACHE_PREFIX, id)
		_ = c.redis.Del(ctx, cacheKey)
		
		// Anda juga bisa menghapus cache laporan yang terkait, jika ada
		// reportCacheKey := fmt.Sprintf("%s%d", COMMISSION_REPORT_CACHE_PREFIX, employeeID)
		// _ = c.redis.Del(ctx, reportCacheKey)
	}
}

// --- Conversion Helpers ---
func (c *CommissionHandler) commissionCalculationToProto(commissionCalculation CommissionCalculation) *proto.CommissionCalculation {
	var detailsProto []*proto.CommissionDetail
	// Konversi setiap item dalam slice CommissionDetails
	for _, detail := range commissionCalculation.CommissionDetails {
		detailsProto = append(detailsProto, c.commissionDetailToProto(detail))
	}

	var paymentProto *proto.CommissionPayment
	// Konversi relasi CommissionPayment jika ada (tidak nil)
	if commissionCalculation.CommissionPayment != nil {
		paymentProto = c.commissionPaymentToProto(*commissionCalculation.CommissionPayment)
	}

	return &proto.CommissionCalculation{
		Id:                      commissionCalculation.ID,
		EmployeeId:              commissionCalculation.EmployeeID,
		CalculationPeriodStart:  commissionCalculation.CalculationPeriodStart,
		CalculationPeriodEnd:    commissionCalculation.CalculationPeriodEnd,
		TotalSales:              commissionCalculation.TotalSales,
		BaseCommission:          commissionCalculation.BaseCommission,
		BonusCommission:         commissionCalculation.BonusCommission,
		TotalCommission:         commissionCalculation.TotalCommission,
		Status:                  proto.CommissionStatus(commissionCalculation.Status), // Konversi int32 ke enum proto
		CalculatedBy:            commissionCalculation.CalculatedBy,
		ApprovedBy:              commissionCalculation.ApprovedBy,
		Notes:                   commissionCalculation.Notes,
		CreatedAt:               timestamppb.New(timeNowOrZero(commissionCalculation.CreatedAt)),
		UpdatedAt:               timestamppb.New(timeNowOrZero(commissionCalculation.UpdatedAt)),
		CommissionDetails:       detailsProto,
		CommissionPayment:       paymentProto,
		// Note: Employee (summary) tidak diisi di sini, mirip seperti PaymentType.
	}
}

func (h *CommissionHandler) commissionDetailToProto(commissionDetail CommissionDetail) *proto.CommissionDetail {
	return &proto.CommissionDetail{
		Id:                    commissionDetail.ID,
		CommissionCalculationId: commissionDetail.CommissionCalculationID,
		OrderItemId:           commissionDetail.OrderItemID,
		ProductId:             commissionDetail.ProductID,
		SalesAmount:           commissionDetail.SalesAmount,
		CommissionRate:        commissionDetail.CommissionRate,
		CommissionAmount:      commissionDetail.CommissionAmount,
		ProductName:           commissionDetail.ProductName,
		OrderDocumentNumber:   commissionDetail.OrderDocumentNumber,
		CreatedAt:             timestamppb.New(timeNowOrZero(commissionDetail.CreatedAt)),
	}
}

func (h *CommissionHandler) commissionPaymentToProto(commissionPayment CommissionPayment) *proto.CommissionPayment {
	return &proto.CommissionPayment{
		Id:                      commissionPayment.ID,
		CommissionCalculationId: commissionPayment.CommissionCalculationID,
		EmployeeId:              commissionPayment.EmployeeID,
		PaymentAmount:           commissionPayment.PaymentAmount,
		PaymentDate:             commissionPayment.PaymentDate,
		PaymentTypeId:           commissionPayment.PaymentTypeID,
		ReferenceNumber:         commissionPayment.ReferenceNumber, // Langsung assign karena GORM model & proto sama-sama pointer
		PaidBy:                  commissionPayment.PaidBy,
		Notes:                   commissionPayment.Notes,
		CreatedAt:               timestamppb.New(timeNowOrZero(commissionPayment.CreatedAt)),
		// Note: PaymentType (summary) tidak diisi di sini karena datanya dari service lain.
		// Data ini bisa di-populate di level atas jika diperlukan (misal, dengan gRPC call lain).
	}
}

// Commission Calculation
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

	result, err := c.calculateCommissionLogic(ctx, req.GetEmployeeId(), req.GetPeriodStart(), req.GetPeriodEnd())
	if err != nil {
		return nil, err
	}

	calculationModel := CommissionCalculation{
		EmployeeID: req.GetEmployeeId(),
		CalculationPeriodStart: req.GetPeriodStart(),
		CalculationPeriodEnd: req.GetPeriodEnd(),
		TotalSales: result.totalSales.StringFixed(2),
		BaseCommission: result.baseCommission.StringFixed(2),
		TotalCommission:        result.totalCommission.StringFixed(2),
		Status:                 int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
		CalculatedBy:           req.GetCalculatedBy(),
		CommissionDetails:      result.details,
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
		Breakdown: result.breakdown,
		IsPreview: !req.GetSaveCalculation(),
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

	result, err := c.calculateCommissionLogic(ctx, existingCalc.EmployeeID, existingCalc.CalculationPeriodStart, existingCalc.CalculationPeriodEnd)
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
			"TotalSales":      result.totalSales.StringFixed(2),
			"TotalCommission": result.totalCommission.StringFixed(2),
			"BaseCommission":  result.baseCommission.StringFixed(2),
			"BonusCommission": result.bonusCommission.StringFixed(2),
			"Status":          int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
			"CalculatedBy":    req.GetRecalculatedBy(),
			"Notes":           req.Notes,
			"ApprovedBy":      nil, // Reset approval status
		}
		if err := tx.Model(&CommissionCalculation{}).Where("id = ?", existingCalc.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update calculation header: %w", err)
		}

		// c. Simpan Detail Baru
		for i := range result.details {
			result.details[i].CommissionCalculationID = existingCalc.ID
		}
		if len(result.details) > 0 {
			if err := tx.Create(&result.details).Error; err != nil {
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
		Breakdown:             result.breakdown,
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

			calcResult, err := c.calculateCommissionLogic(ctx, eID, req.GetPeriodStart(), req.GetPeriodEnd())
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
				TotalSales:             calcResult.totalSales.StringFixed(2),
				BaseCommission:         calcResult.baseCommission.StringFixed(2),
				BonusCommission:        calcResult.bonusCommission.StringFixed(2),
				TotalCommission:        calcResult.totalCommission.StringFixed(2),
				Status:                 int32(proto.CommissionStatus_COMMISSION_STATUS_CALCULATED),
				CalculatedBy:           req.GetCalculatedBy(),
				CommissionDetails:      calcResult.details,
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
		Errors: errorMessages,
		SuccessCount: int32(len(successfulCalculations)),
		ErrorCount: int32(len(errorMessages)),
	}, nil
}

// Commission Management

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
		if err := c.redis.Set(ctx, cacheKey, jsonData, 24*time.Hour).Err();err != nil {
			fmt.Printf("Failed to set cache for key %s: %v\n", cacheKey, err)
		}
	}

	return &proto.GetCommissionCalculationResponse{
		CommissionCalculation: c.commissionCalculationToProto(dbCalc),
	}, nil
}

func (c * CommissionHandler) ListCommissionCalculations(ctx context.Context, req *proto.ListCommissionCalculationsRequest) (*proto.ListCommissionCalculationsResponse, error) {
	var (
		page = 1
		limit = 20
	)
	if p := req.GetPagination(); p != nil {
		if p.GetPageSize() > 0 {
			limit = int(p.GetPageSize())
		}
		if pagenum, err := strconv.Atoi(p.GetPageToken()); err == nil && pagenum > 0 {
			page = pagenum
		}
	}	
	offset := (page - 1) * limit

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
	err := query.
		Order("created_at desc").
		Offset(offset). 
		Limit(limit). 
		Preload("CommissionPayment"). 
		Find(&calculations).Error
	
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to retrieve calculations: %v", err)
	} 

	var protoCalculations []*proto.CommissionCalculation
	for _, calc := range calculations {
		protoCalculations = append(protoCalculations, c.commissionCalculationToProto(calc))
	}

	nextPageToken := ""
	if int64(offset+limit) < totalCount {
		nextPageToken = strconv.Itoa(page + 1)
	}

	return &proto.ListCommissionCalculationsResponse{
		CommissionCalculations: protoCalculations,
		Pagination: &proto.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount: int32(totalCount),
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&calculation, req. GetCommissionCalculationId()).Error; err != nil {
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
			calculation.Notes = strPtr(req.GetApprovalNotes())
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
		errorMessages				 []string
		wg									 sync.WaitGroup
		mu									 sync.Mutex
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
					calculation.Notes = strPtr(req.GetApprovalNotes())
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
		Errors: errorMessages,
		SuccessCount: int32(len(approvedCalculations)),
		ErrorCount: int32(len(errorMessages)),
	}, nil
}

// Commission Payment
func (c *CommissionHandler) PayCommission(ctx context.Context, req *proto.PayCommissionRequest) (*proto.PayCommissionResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}
	if req.GetPaymentTypeId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Payment Type ID is required")
	}
	if req.GetPaidBy() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Paid By (user ID) is required")
	}

	paymentDate := time.Now().Format("2006-01-02")
	if req.GetPaymentDate() != "" {
		paymentDate = req.GetPaymentDate()
	}

	var calculation CommissionCalculation
	var payment CommissionPayment

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&calculation, req.GetCommissionCalculationId()).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return status.Errorf(codes.NotFound, "Commission calculation with ID %d Not Found", req.GetCommissionCalculationId())
			}
			return status.Errorf(codes.Internal, "Failed to retrieve calculation: %v", err)
		}

		if calculation.Status != int32(proto.CommissionStatus_COMMISSION_STATUS_APPROVED) {
			return status.Errorf(codes.FailedPrecondition, "Commission can only be paid from APPROVED status. Current status: %s", proto.CommissionStatus_name[calculation.Status])
		}

		payment = CommissionPayment{
			CommissionCalculationID: calculation.ID,
			EmployeeID:              calculation.EmployeeID,
			PaymentAmount:           calculation.TotalCommission, // Jumlah pembayaran = total komisi
			PaymentDate:             paymentDate,
			PaymentTypeID:           req.GetPaymentTypeId(),
			ReferenceNumber:         req.ReferenceNumber,
			PaidBy:                  req.GetPaidBy(),
			Notes:                   req.Notes,
		}
		if err := tx.Create(&payment).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to create payment record: %v", err)
		}

		calculation.Status = int32(proto.CommissionStatus_COMMISSION_STATUS_PAID)
		if err := tx.Save(&calculation).Error; err != nil {
			return status.Errorf(codes.Internal, "Failed to update calculation status: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	c.InvalidateCommissionCaches(ctx, req.GetCommissionCalculationId())

	c.db.WithContext(ctx).Preload("CommissionDetails").First(&calculation, calculation.ID)

	return &proto.PayCommissionResponse{
		CommissionPayment: c.commissionPaymentToProto(payment),
		UpdatedCalculation: c.commissionCalculationToProto(calculation),
	}, nil
}

func (c *CommissionHandler) GetCommissionPayment(ctx context.Context, req *proto.GetCommissionPaymentRequest) (*proto.GetCommissionPaymentResponse, error) {
	if req.GetCommissionCalculationId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Commission Calculation ID is required")
	}

	var payment CommissionPayment
	err := c.db.WithContext(ctx).Where("commission_calculation_id = ?", req.GetCommissionCalculationId()).First(&payment).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Commission calculation with ID %d Not Found", req.GetCommissionCalculationId())
		}
		return nil, status.Errorf(codes.Internal, "Failed to retrieve commission payment: %v", err)
	}

	return &proto.GetCommissionPaymentResponse{
		CommissionPayment: c.commissionPaymentToProto(payment),
	}, nil
}

// Commission Reporting
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
	if err != nil {
		var summary proto.CommissionSummary
		if err := json.Unmarshal([]byte(val), &summary); err == nil {
			return &proto.GetCommissionSummaryResponse{
				Summary: &summary,
			}, nil
		}
	}

	var employee struct {
		EmployeeName string
	}
	if err := c.db.WithContext(ctx).Table("user.employees").Select("employee_name").Where("id = ?", employeeID).First(&employee).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
		}
		return nil, status.Errorf(codes.Internal, "Failed to get employee name: %v", err)
	}

	var aggResult struct {
		TotalSales string
		TotalEarned string
		TotalPaid string
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
		EmployeeId:              employeeID,
		EmployeeName:            employee.EmployeeName,
		Period:                  req.GetDateRange(),
		TotalSales:              totalSales.StringFixed(2),
		TotalCommissionEarned:   totalEarned.StringFixed(2),
		TotalCommissionPaid:     totalPaid.StringFixed(2),
		CommissionPending:       pending.StringFixed(2),
		AverageCommissionRate:   avgRate.StringFixed(2),
		CalculationCount:        aggResult.CalculationCount,
		RecentCalculations:      recentCalcsProto,
	}

	jsonData, err := json.Marshal(summary)
	if err == nil {
		c.redis.Set(ctx, cacheKey, jsonData, 2*time.Hour)
	}

	return &proto.GetCommissionSummaryResponse{
		Summary: summary,
	}, nil
}

func ( c * CommissionHandler) GetCommissionReport(ctx context.Context, req *proto.GetCommissionReportRequest) (*proto.GetCommissionReportResponse, error) {
	if req.GetDateRange() == nil || req.GetDateRange().GetStartDate() == "" || req.GetDateRange().GetEndDate() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Date range is required")
	}
	startDate := req.GetDateRange().GetStartDate()
	endDate := req.GetDateRange().GetEndDate()

	var (
		page = 1
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
		Paid string
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
		EmployeeID           int64
		TotalSales           string
		TotalCommissionEarned string
		TotalCommissionPaid  string
		CalculationCount     int32
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

	employeeNameMap := make(map[int64]string)
	if len(employeeIDs) > 0 {
		var employees []struct {
			ID int64
			EmployeeName string
		}
		if err := c.db.WithContext(ctx).Table("user.employees").Select("id, employee_name").Where("id IN ?", employeeIDs).Find(&employees).Error; err != nil {
			for _, e := range employees {
				employeeNameMap[e.ID] = e.EmployeeName
			}
	  }
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
			EmployeeId:              data.EmployeeID,
			EmployeeName:            employeeNameMap[data.EmployeeID],
			Period:                  req.GetDateRange(),
			TotalSales:              data.TotalSales,
			TotalCommissionEarned:   data.TotalCommissionEarned,
			TotalCommissionPaid:     data.TotalCommissionPaid,
			CommissionPending:       pending.StringFixed(2),
			AverageCommissionRate:   avgRate.StringFixed(2),
			CalculationCount:        data.CalculationCount,
		}
		summariesProto = append(summariesProto, summary)
	}

	nextPageToken := ""
	if int64(offset+limit) < totalEmployees {
		nextPageToken = strconv.Itoa(page + 1)
	}

	return &proto.GetCommissionReportResponse{
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

// Commission Settings
func (c *CommissionHandler) GetCommissionSettings(ctx context.Context, req *proto.GetCommissionSettingsRequest) (*proto.GetCommissionSettingsResponse, error) {
	if req.GetEmployeeId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Employee ID is required")
	}
	employeeID := req.GetEmployeeId()

	var employeeData struct {
		ID             int64
		EmployeeName   string
		Position       string
		CommissionRate string
		CommissionType string
	}
	if err := c.db.WithContext(ctx).Table("user.employees").Select("id, employee_name, position, commission_rate, commission_type").Where("ide = ?", employeeID).First(&employeeData).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "Employee with ID %d not found", employeeID)
		}
		return nil, status.Errorf(codes.Internal, "Failed to get employee settings: %v", err)
	}

	var tierSettingsGorm []CommissionTierInfo
	if employeeData.CommissionType == "tiered" {
		if err := c.db.WithContext(ctx).Where("employee_id = ?", employeeID).Order("min_sales_amount asc").Find(&tierSettingsGorm).Error; err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to get commission tiers: %v", err)
		}
	}

	var commissionTypeProto proto.CommissionType
	switch employeeData.CommissionType {
	case "percentage":
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_PERCENTAGE
	case "tiered":
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_TIERED
	case "fixed_amount":
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_FIXED_AMOUNT
	default:
		commissionTypeProto = proto.CommissionType_COMMISSION_TYPE_UNSPECIFIED
	}

	employeeSummaryProto := &proto.EmployeeSummary{
		Id:            employeeData.ID,
		EmployeeName:  employeeData.EmployeeName,
		Position:      &employeeData.Position,
		CommissionRate: employeeData.CommissionRate,
		CommissionType: commissionTypeProto,
	}

	var tierSettingsProto []*proto.CommissionTierSetting
	for _, tier := range tierSettingsGorm {
		tierSettingsProto = append(tierSettingsProto, &proto.CommissionTierSetting{
			MinSalesAmount:  tier.MinSalesAmount,
			MaxSalesAmount:  &tier.MaxSalesAmount,
			CommissionRate:  tier.CommissionRate,
		})
	}

	return &proto.GetCommissionSettingsResponse{
		Employee: employeeSummaryProto,
		TierSettings: tierSettingsProto,
	}, nil
}